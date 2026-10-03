// Package forwarduse owns the shell-neutral port-forwarding use cases:
// local, remote and dynamic (SOCKS5) tunnel lifecycle over the shared SSH
// transport pool (start/stop/list/snapshot), rule-ID parsing, port binding
// and connection relaying. The SOCKS5 handshake itself lives in
// internal/terminal/forward.
//
// Per the Wails v3 migration (W04), this package is the canonical owner of
// the forward tunnel table and its rules. Shell facades (Wails today,
// capability dispatch later) adapt it to their transport and must not copy or
// re-implement its behavior. It must never import a shell (Wails, Electron,
// UI).
package forwarduse

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/binaricat/lemonssh/internal/app/terminaluse"
	"github.com/binaricat/lemonssh/internal/terminal/forward"
	"github.com/binaricat/lemonssh/internal/terminal/ssh"
	"github.com/binaricat/lemonssh/internal/terminal/sshpool"
)

// Result is one tunnel operation outcome.
type Result struct {
	TunnelID         string `json:"tunnelId"`
	Success          bool   `json:"success"`
	Status           string `json:"status,omitempty"`
	Error            string `json:"error,omitempty"`
	Cancelled        bool   `json:"cancelled,omitempty"`
	BlockedByCleanup bool   `json:"blockedByCleanup,omitempty"`
	Reused           bool   `json:"reused,omitempty"`
}

// ListItem is one active tunnel row.
type ListItem struct {
	RuleID   string `json:"ruleId"`
	TunnelID string `json:"tunnelId"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

// RuntimeRecord is one row of a RuntimeSnapshot.
type RuntimeRecord struct {
	RuleID    string `json:"ruleId"`
	TunnelID  string `json:"tunnelId"`
	Phase     string `json:"phase"`
	Error     string `json:"error,omitempty"`
	Revision  uint64 `json:"revision"`
	UpdatedAt int64  `json:"updatedAt"`
}

// RuntimeSnapshot is the renderer-facing forward table state. Revision is the
// tunnel-table revision as of the snapshot; the next RuntimeEvent carries
// Revision+1, so the renderer can detect missed events by revision gaps.
type RuntimeSnapshot struct {
	Epoch    string          `json:"epoch"`
	Revision uint64          `json:"revision"`
	Records  []RuntimeRecord `json:"records"`
}

// RuntimeEvent kinds.
const (
	RuntimeEventKindUpsert = "upsert"
	RuntimeEventKindRemove = "remove"
)

// RuntimeEvent is one ordered tunnel-table change (tunnel started / stopped).
// Revisions are contiguous per epoch: revision N is followed by N+1. The
// renderer reconciles from RuntimeSnapshot whenever it observes a gap.
type RuntimeEvent struct {
	Epoch    string         `json:"epoch"`
	Revision uint64         `json:"revision"`
	Kind     string         `json:"kind"`
	Record   *RuntimeRecord `json:"record,omitempty"`
	TunnelID string         `json:"tunnelId,omitempty"`
	RuleID   string         `json:"ruleId,omitempty"`
}

// Service owns forward tunnels end-to-end: pool lease (KindForward) →
// per-tunnel forward.Manager → bind/relay → teardown back to the pool. The
// pool instance is shared with the terminal and SFTP use cases; forwarding
// never dials around it.
type Service struct {
	mu         sync.Mutex
	pool       *sshpool.Pool
	knownHosts *ssh.KnownHosts
	tunnels    map[string]*tunnel
	// emitMu serializes revision claiming with sink delivery so runtime
	// events arrive strictly in revision order even when Start/Stop race.
	emitMu   sync.Mutex
	revision uint64
	epoch    string
	sink     func(RuntimeEvent)
}

type tunnel struct {
	ruleID  string
	kind    string
	lease   *sshpool.Lease
	manager *forward.Manager
}

// New wires the shared SSH pool and known-hosts store.
func New(pool *sshpool.Pool, knownHosts *ssh.KnownHosts) *Service {
	return &Service{pool: pool, knownHosts: knownHosts, tunnels: make(map[string]*tunnel)}
}

// SetRuntimeEpoch stamps the owning shell identity onto emitted runtime
// events, mirroring the epoch passed to RuntimeSnapshot.
func (s *Service) SetRuntimeEpoch(epoch string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.epoch = epoch
}

// OnRuntimeEvent installs the single sink invoked for every ordered
// tunnel-table change (tunnel started → upsert, tunnel stopped → remove).
// Passing nil disables delivery. The sink is invoked synchronously in
// revision order; it must not re-enter publish (Start/Stop) or it deadlocks.
func (s *Service) OnRuntimeEvent(sink func(RuntimeEvent)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sink = sink
}

// publish mutates the tunnel table and emits one ordered runtime event. fn
// runs with s.mu held and returns the change to emit (ok=false for read-only
// operations, which claim no revision). Revision claiming and sink delivery
// both happen under s.emitMu, so sink calls arrive strictly in revision order
// even when Start/Stop race; delivery never holds s.mu, so the sink may call
// List/Snapshot freely.
func (s *Service) publish(fn func() (RuntimeEvent, bool)) {
	s.emitMu.Lock()
	defer s.emitMu.Unlock()
	s.mu.Lock()
	event, changed := fn()
	var sink func(RuntimeEvent)
	if changed {
		s.revision++
		event.Revision = s.revision
		event.Epoch = s.epoch
		if event.Record != nil {
			event.Record.Revision = s.revision
		}
		sink = s.sink
	}
	s.mu.Unlock()
	if changed && sink != nil {
		sink(event)
	}
}

// Start starts (or reuses) one tunnel for the rule encoded in id.
func (s *Service) Start(id, kind, bindHost string, bindPort uint16, targetHost string, targetPort uint16, request terminaluse.SSHConnectRequest) Result {
	ruleID := parseForwardRuleID(id)
	s.mu.Lock()
	for tunnelID, existing := range s.tunnels {
		if existing.ruleID == ruleID {
			s.mu.Unlock()
			return Result{TunnelID: tunnelID, Success: true, Status: "active", Reused: true}
		}
	}
	s.mu.Unlock()

	if s.pool == nil {
		return Result{TunnelID: id, Error: "ssh pool unavailable"}
	}
	if request.Port == 0 {
		request.Port = 22
	}
	config, err := terminaluse.SSHDialConfig(request, s.knownHosts, ssh.DialInteractive{})
	if err != nil {
		return Result{TunnelID: id, Error: err.Error()}
	}
	lease, err := s.pool.Get(context.Background(), config, sshpool.KindForward)
	if err != nil {
		return Result{TunnelID: id, Error: err.Error()}
	}
	client := lease.Client()
	if client == nil {
		lease.Discard()
		return Result{TunnelID: id, Error: "ssh client unavailable"}
	}
	tunnelFn := func(ctx context.Context, host string, port uint16, local net.Conn) error {
		remote, dialErr := client.Dial("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
		if dialErr != nil {
			return dialErr
		}
		proxyConn(local, remote)
		return nil
	}
	listenRemote := func(ctx context.Context, spec forward.Spec) (net.Listener, error) {
		addr := net.JoinHostPort(spec.BindHost, fmt.Sprintf("%d", spec.BindPort))
		return client.Listen("tcp", addr)
	}
	manager, err := forward.NewManagerForRemote(listenRemote, tunnelFn)
	if err != nil {
		lease.Discard()
		return Result{TunnelID: id, Error: err.Error()}
	}
	_, err = manager.Start(context.Background(), forward.Spec{
		ID:         id,
		Kind:       forward.Kind(kind),
		BindHost:   bindHost,
		BindPort:   bindPort,
		TargetHost: targetHost,
		TargetPort: targetPort,
		RuleID:     ruleID,
	})
	if err != nil {
		lease.Discard()
		return Result{TunnelID: id, Error: err.Error()}
	}
	s.mu.Lock()
	if _, exists := s.tunnels[id]; exists {
		s.mu.Unlock()
		return Result{TunnelID: id, Success: true, Status: "active", Reused: true}
	}
	s.tunnels[id] = &tunnel{ruleID: ruleID, kind: kind, lease: lease, manager: manager}
	s.mu.Unlock()
	s.publish(func() (RuntimeEvent, bool) {
		return RuntimeEvent{
			Kind: RuntimeEventKindUpsert,
			Record: &RuntimeRecord{
				RuleID:    ruleID,
				TunnelID:  id,
				Phase:     "active",
				UpdatedAt: time.Now().UnixMilli(),
			},
		}, true
	})
	return Result{TunnelID: id, Success: true, Status: "active"}
}

// Stop stops one tunnel and returns its lease to the pool.
func (s *Service) Stop(id string) Result {
	var removed *tunnel
	s.publish(func() (RuntimeEvent, bool) {
		tunnel := s.tunnels[id]
		if tunnel == nil {
			return RuntimeEvent{}, false
		}
		delete(s.tunnels, id)
		removed = tunnel
		return RuntimeEvent{
			Kind:     RuntimeEventKindRemove,
			TunnelID: id,
			RuleID:   tunnel.ruleID,
		}, true
	})
	if removed == nil {
		return Result{TunnelID: id, Success: true, Status: "inactive"}
	}
	_, _ = removed.manager.Stop(id)
	removed.lease.Return()
	return Result{TunnelID: id, Success: true, Status: "inactive"}
}

// StopByRuleId stops every tunnel started for one rule.
func (s *Service) StopByRuleId(ruleID string) map[string]any {
	s.mu.Lock()
	ids := make([]string, 0)
	for id, tunnel := range s.tunnels {
		if tunnel.ruleID == ruleID {
			ids = append(ids, id)
		}
	}
	s.mu.Unlock()
	for _, id := range ids {
		_ = s.Stop(id)
	}
	return map[string]any{"stopped": len(ids)}
}

// recordsLocked renders the live tunnel table as runtime records. s.mu held.
func (s *Service) recordsLocked(now int64, revision uint64) []RuntimeRecord {
	records := make([]RuntimeRecord, 0, len(s.tunnels))
	for id, tunnel := range s.tunnels {
		phase := "active"
		errText := ""
		if state, err := tunnel.manager.Snapshot(id); err == nil && state.Err != "" {
			phase = "error"
			errText = state.Err
		}
		records = append(records, RuntimeRecord{
			RuleID:    tunnel.ruleID,
			TunnelID:  id,
			Phase:     phase,
			Error:     errText,
			Revision:  revision,
			UpdatedAt: now,
		})
	}
	return records
}

// List reports all active tunnels with their live status.
func (s *Service) List() []ListItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	records := s.recordsLocked(time.Now().UnixMilli(), s.revision)
	items := make([]ListItem, 0, len(records))
	for _, record := range records {
		items = append(items, ListItem{
			RuleID:   record.RuleID,
			TunnelID: record.TunnelID,
			Type:     s.tunnels[record.TunnelID].kind,
			Status:   record.Phase,
			Error:    record.Error,
		})
	}
	return items
}

// Snapshot reports one tunnel's status.
func (s *Service) Snapshot(id string) Result {
	s.mu.Lock()
	tunnel := s.tunnels[id]
	s.mu.Unlock()
	if tunnel == nil {
		return Result{TunnelID: id, Success: true, Status: "inactive"}
	}
	state, err := tunnel.manager.Snapshot(id)
	if err != nil {
		return Result{TunnelID: id, Success: true, Status: "inactive"}
	}
	status := "active"
	if state.Err != "" {
		status = "error"
	}
	return Result{TunnelID: id, Success: true, Status: status, Error: state.Err}
}

// RuntimeSnapshot renders the tunnel table for the renderer poll; epoch
// identifies the owning shell so stale snapshots from a previous process are
// detectable. Revision is the current tunnel-table revision; the next
// RuntimeEvent carries Revision+1.
func (s *Service) RuntimeSnapshot(epoch string) RuntimeSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixMilli()
	return RuntimeSnapshot{
		Epoch:    epoch,
		Revision: s.revision,
		Records:  s.recordsLocked(now, s.revision),
	}
}

func parseForwardRuleID(tunnelID string) string {
	trimmed := strings.TrimPrefix(tunnelID, "pf-")
	if index := strings.LastIndex(trimmed, "-"); index > 0 {
		return trimmed[:index]
	}
	return tunnelID
}

func proxyConn(left, right net.Conn) {
	defer left.Close()
	defer right.Close()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(left, right); done <- struct{}{} }()
	go func() { _, _ = io.Copy(right, left); done <- struct{}{} }()
	<-done
}
