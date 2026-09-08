// Package sshpool owns the shared SSH transport pool (P3-04). Every business
// domain (shell, SFTP, transfer, forwarding) must lease from this pool; private
// per-domain pools are forbidden. One authenticated transport legitimately
// serves many concurrent channels, so leases reference-count the transport;
// typed leases decide healthy-return versus discard semantics.
package sshpool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	netcattyssh "github.com/binaricat/netcatty/internal/terminal/ssh"
	"golang.org/x/crypto/ssh"
)

var ErrPoolClosed = errors.New("ssh pool is closed")

// LeaseKind is the typed usage of a transport, recorded for diagnostics and
// forward/agent asymmetry policy.
type LeaseKind string

const (
	KindShell    LeaseKind = "shell"
	KindSFTP     LeaseKind = "sftp"
	KindTransfer LeaseKind = "transfer"
	KindForward  LeaseKind = "forward"
)

// CompatibilityKey derives the immutable pool key from endpoint, auth
// fingerprint and jump chain. Equal keys mean "safe to reuse one
// authenticated transport".
func CompatibilityKey(config netcattyssh.DialConfig) (string, error) {
	authDigest, err := authFingerprint(config)
	if err != nil {
		return "", err
	}
	jumpHash := ""
	if len(config.JumpHosts) > 0 {
		jumpParts := make([]string, 0, len(config.JumpHosts))
		for _, hop := range config.JumpHosts {
			hopHash, err := CompatibilityKey(hop)
			if err != nil {
				return "", err
			}
			jumpParts = append(jumpParts, hopHash)
		}
		joined, _ := json.Marshal(jumpParts)
		sum := sha256.Sum256(joined)
		jumpHash = hex.EncodeToString(sum[:])
	}
	input := struct {
		Hostname     string `json:"hostname"`
		Port         uint16 `json:"port"`
		Username     string `json:"username"`
		JumpHash     string `json:"jumpHash"`
		AuthDigest   string `json:"authDigest"`
		ForwardAgent bool   `json:"forwardAgent"`
	}{
		Hostname:     config.Hostname,
		Port:         config.Port,
		Username:     config.Username,
		JumpHash:     jumpHash,
		AuthDigest:   authDigest,
		ForwardAgent: config.ForwardAgent,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func authFingerprint(config netcattyssh.DialConfig) (string, error) {
	material := struct {
		Password    string `json:"password,omitempty"`
		PrivateKey  string `json:"privateKey,omitempty"`
		Passphrase  string `json:"passphrase,omitempty"`
		Interactive bool   `json:"interactive,omitempty"`
	}{
		Password:    config.Auth.Password,
		PrivateKey:  string(config.Auth.PrivateKeyPEM),
		Passphrase:  config.Auth.Passphrase,
		Interactive: config.Auth.Interactive != nil,
	}
	encoded, err := json.Marshal(material)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

type pooledTransport struct {
	key         string
	transport   *netcattyssh.Transport
	lastUsed    time.Time
	outstanding int
	singleUse   bool
}

// Lease is one checkout. Return or Discard must be called exactly once.
type Lease struct {
	pool     *Pool
	entry    *pooledTransport
	Kind     LeaseKind
	returned bool
}

// Client exposes the authenticated SSH client for the lease lifetime.
func (l *Lease) Client() *ssh.Client { return l.entry.transport.Client }

// Return gives a healthy transport back to the pool.
func (l *Lease) Return() { l.pool.release(l, true) }

// Discard drops an unhealthy transport instead of reusing it.
func (l *Lease) Discard() { l.pool.release(l, false) }

// Pool manages authenticated transports.
type Pool struct {
	mu         sync.Mutex
	closed     bool
	idleTTL    time.Duration
	maxIdle    int
	transports map[string]*pooledTransport
	waiters    map[string]*dialGroup
	dialFunc   func(ctx context.Context, config netcattyssh.DialConfig) (*netcattyssh.Transport, error)
}

type dialGroup struct {
	done chan struct{}
}

// Option configures the pool.
type Option func(*Pool)

// WithIdleTTL sets how long an idle transport stays pooled.
func WithIdleTTL(ttl time.Duration) Option { return func(p *Pool) { p.idleTTL = ttl } }

// WithMaxIdle bounds idle transports (LRU eviction).
func WithMaxIdle(max int) Option { return func(p *Pool) { p.maxIdle = max } }

// New constructs the shared pool. The dial function is injectable for tests.
func New(dial func(ctx context.Context, config netcattyssh.DialConfig) (*netcattyssh.Transport, error), options ...Option) *Pool {
	pool := &Pool{
		idleTTL:    5 * time.Minute,
		maxIdle:    8,
		transports: make(map[string]*pooledTransport),
		waiters:    make(map[string]*dialGroup),
		dialFunc:   dial,
	}
	for _, option := range options {
		option(pool)
	}
	return pool
}

// Get returns a lease, dialing via single-flight when no live compatible
// transport exists.
func (p *Pool) Get(ctx context.Context, config netcattyssh.DialConfig, kind LeaseKind) (*Lease, error) {
	key, err := CompatibilityKey(config)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, ErrPoolClosed
	}
	if config.ForwardAgent {
		// Asymmetric reuse: agent-forwarding transports are single-use.
		transport, err := p.dialFunc(ctx, config)
		if err != nil {
			p.mu.Unlock()
			return nil, err
		}
		entry := &pooledTransport{key: key, transport: transport, lastUsed: time.Now(), outstanding: 1, singleUse: true}
		lease := &Lease{pool: p, entry: entry, Kind: kind}
		p.mu.Unlock()
		return lease, nil
	}
	if entry, ok := p.transports[key]; ok {
		if entry.outstanding == 0 && time.Since(entry.lastUsed) > p.idleTTL {
			_ = entry.transport.Close()
			delete(p.transports, key)
		} else {
			entry.outstanding++
			entry.lastUsed = time.Now()
			p.mu.Unlock()
			return &Lease{pool: p, entry: entry, Kind: kind}, nil
		}
	}
	group, waiting := p.waiters[key]
	if !waiting {
		group = &dialGroup{done: make(chan struct{})}
		p.waiters[key] = group
		go p.dial(ctx, config, key, group)
	}
	p.mu.Unlock()

	select {
	case <-group.done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.transports[key]
	if !ok {
		return nil, ErrPoolClosed
	}
	entry.outstanding++
	return &Lease{pool: p, entry: entry, Kind: kind}, nil
}

func (p *Pool) dial(ctx context.Context, config netcattyssh.DialConfig, key string, group *dialGroup) {
	transport, err := p.dialFunc(ctx, config)
	p.mu.Lock()
	if err == nil {
		p.transports[key] = &pooledTransport{key: key, transport: transport, lastUsed: time.Now()}
	}
	delete(p.waiters, key)
	p.mu.Unlock()
	close(group.done)
}

func (p *Pool) release(lease *Lease, healthy bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if lease.returned {
		return
	}
	lease.returned = true
	entry := lease.entry
	if entry.outstanding > 0 {
		entry.outstanding--
	}
	if entry.singleUse {
		// Agent-forwarding transports never re-enter the pool.
		_ = entry.transport.Close()
		delete(p.transports, entry.key)
		return
	}
	if !healthy || p.closed {
		_ = entry.transport.Close()
		delete(p.transports, entry.key)
		return
	}
	entry.lastUsed = time.Now()
	p.evictLocked()
}

func (p *Pool) evictLocked() {
	now := time.Now()
	for key, entry := range p.transports {
		if entry.outstanding == 0 && now.Sub(entry.lastUsed) > p.idleTTL {
			_ = entry.transport.Close()
			delete(p.transports, key)
		}
	}
	idle := make([]string, 0, len(p.transports))
	for key, entry := range p.transports {
		if entry.outstanding == 0 {
			idle = append(idle, key)
		}
	}
	if len(idle) <= p.maxIdle {
		return
	}
	for len(idle) > p.maxIdle {
		oldestKey := ""
		var oldest time.Time
		for _, key := range idle {
			entry := p.transports[key]
			if oldestKey == "" || entry.lastUsed.Before(oldest) {
				oldestKey, oldest = key, entry.lastUsed
			}
		}
		if oldestKey == "" {
			return
		}
		_ = p.transports[oldestKey].transport.Close()
		delete(p.transports, oldestKey)
		kept := idle[:0]
		for _, item := range idle {
			if item != oldestKey {
				kept = append(kept, item)
			}
		}
		idle = kept
	}
}

// Shutdown closes every transport and rejects further Get calls.
func (p *Pool) Shutdown() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	var firstErr error
	for key, entry := range p.transports {
		if err := entry.transport.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(p.transports, key)
	}
	return firstErr
}

// Size reports the number of pooled transports (tests/observability).
func (p *Pool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.transports)
}
