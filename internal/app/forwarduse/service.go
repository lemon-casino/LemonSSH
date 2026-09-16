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

	"github.com/binaricat/netcatty/internal/app/terminaluse"
	"github.com/binaricat/netcatty/internal/terminal/forward"
	"github.com/binaricat/netcatty/internal/terminal/ssh"
	"github.com/binaricat/netcatty/internal/terminal/sshpool"
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
	RuleID   string `json:"ruleId"`
	TunnelID string `json:"tunnelId"`
	Phase    string `json:"phase"`
	Error    string `json:"error,omitempty"`
	Revision uint64 `json:"revision"`
}

// RuntimeSnapshot is the renderer-facing forward table state.
type RuntimeSnapshot struct {
	Epoch    string          `json:"epoch"`
	Revision uint64          `json:"revision"`
	Records  []RuntimeRecord `json:"records"`
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
	config, err := terminaluse.SSHDialConfig(request, s.knownHosts, nil)
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
	s.tunnels[id] = &tunnel{ruleID: ruleID, kind: kind, lease: lease, manager: manager}
	s.mu.Unlock()
	return Result{TunnelID: id, Success: true, Status: "active"}
}

// Stop stops one tunnel and returns its lease to the pool.
func (s *Service) Stop(id string) Result {
	s.mu.Lock()
	tunnel := s.tunnels[id]
	delete(s.tunnels, id)
	s.mu.Unlock()
	if tunnel == nil {
		return Result{TunnelID: id, Success: true, Status: "inactive"}
	}
	_, _ = tunnel.manager.Stop(id)
	tunnel.lease.Return()
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

// List reports all active tunnels with their live status.
func (s *Service) List() []ListItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]ListItem, 0, len(s.tunnels))
	for id, tunnel := range s.tunnels {
		status := "active"
		errText := ""
		if state, err := tunnel.manager.Snapshot(id); err == nil && state.Err != "" {
			status = "error"
			errText = state.Err
		}
		items = append(items, ListItem{
			RuleID:   tunnel.ruleID,
			TunnelID: id,
			Type:     tunnel.kind,
			Status:   status,
			Error:    errText,
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
// detectable.
func (s *Service) RuntimeSnapshot(epoch string) RuntimeSnapshot {
	items := s.List()
	records := make([]RuntimeRecord, 0, len(items))
	for _, item := range items {
		records = append(records, RuntimeRecord{
			RuleID:   item.RuleID,
			TunnelID: item.TunnelID,
			Phase:    item.Status,
			Error:    item.Error,
		})
	}
	return RuntimeSnapshot{Epoch: epoch, Records: records}
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
