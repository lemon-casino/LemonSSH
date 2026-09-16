package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/binaricat/netcatty/internal/terminal/forward"
	netcattyssh "github.com/binaricat/netcatty/internal/terminal/ssh"
	"github.com/binaricat/netcatty/internal/terminal/sshpool"
)

type ForwardService struct {
	mu         sync.Mutex
	pool       *sshpool.Pool
	knownHosts *netcattyssh.KnownHosts
	tunnels    map[string]*forwardTunnel
}

type forwardTunnel struct {
	ruleID  string
	kind    string
	lease   *sshpool.Lease
	manager *forward.Manager
}

type PortForwardResult struct {
	TunnelID         string `json:"tunnelId"`
	Success          bool   `json:"success"`
	Status           string `json:"status,omitempty"`
	Error            string `json:"error,omitempty"`
	Cancelled        bool   `json:"cancelled,omitempty"`
	BlockedByCleanup bool   `json:"blockedByCleanup,omitempty"`
	Reused           bool   `json:"reused,omitempty"`
}

type PortForwardListItem struct {
	RuleID   string `json:"ruleId"`
	TunnelID string `json:"tunnelId"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

type PortForwardRuntimeRecord struct {
	RuleID   string `json:"ruleId"`
	TunnelID string `json:"tunnelId"`
	Phase    string `json:"phase"`
	Error    string `json:"error,omitempty"`
	Revision uint64 `json:"revision"`
}

type PortForwardRuntimeSnapshot struct {
	Epoch    string                     `json:"epoch"`
	Revision uint64                     `json:"revision"`
	Records  []PortForwardRuntimeRecord `json:"records"`
}

func NewForwardService(pool *sshpool.Pool, knownHosts *netcattyssh.KnownHosts) *ForwardService {
	return &ForwardService{pool: pool, knownHosts: knownHosts, tunnels: make(map[string]*forwardTunnel)}
}

func (s *ForwardService) Start(id, kind, bindHost string, bindPort uint16, targetHost string, targetPort uint16, request SSHConnectRequest) PortForwardResult {
	ruleID := parseForwardRuleID(id)
	s.mu.Lock()
	for tunnelID, existing := range s.tunnels {
		if existing.ruleID == ruleID {
			s.mu.Unlock()
			return PortForwardResult{TunnelID: tunnelID, Success: true, Status: "active", Reused: true}
		}
	}
	s.mu.Unlock()

	if s.pool == nil {
		return PortForwardResult{TunnelID: id, Error: "ssh pool unavailable"}
	}
	if request.Port == 0 {
		request.Port = 22
	}
	config, err := terminalSSHDialConfig(request, s.knownHosts, nil)
	if err != nil {
		return PortForwardResult{TunnelID: id, Error: err.Error()}
	}
	lease, err := s.pool.Get(context.Background(), config, sshpool.KindForward)
	if err != nil {
		return PortForwardResult{TunnelID: id, Error: err.Error()}
	}
	client := lease.Client()
	if client == nil {
		lease.Discard()
		return PortForwardResult{TunnelID: id, Error: "ssh client unavailable"}
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
		return PortForwardResult{TunnelID: id, Error: err.Error()}
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
		return PortForwardResult{TunnelID: id, Error: err.Error()}
	}
	s.mu.Lock()
	s.tunnels[id] = &forwardTunnel{ruleID: ruleID, kind: kind, lease: lease, manager: manager}
	s.mu.Unlock()
	return PortForwardResult{TunnelID: id, Success: true, Status: "active"}
}

func (s *ForwardService) Stop(id string) PortForwardResult {
	s.mu.Lock()
	tunnel := s.tunnels[id]
	delete(s.tunnels, id)
	s.mu.Unlock()
	if tunnel == nil {
		return PortForwardResult{TunnelID: id, Success: true, Status: "inactive"}
	}
	_, _ = tunnel.manager.Stop(id)
	tunnel.lease.Return()
	return PortForwardResult{TunnelID: id, Success: true, Status: "inactive"}
}

func (s *ForwardService) StopByRuleId(ruleID string) map[string]any {
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

func (s *ForwardService) List() []PortForwardListItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]PortForwardListItem, 0, len(s.tunnels))
	for id, tunnel := range s.tunnels {
		status := "active"
		errText := ""
		if state, err := tunnel.manager.Snapshot(id); err == nil && state.Err != "" {
			status = "error"
			errText = state.Err
		}
		items = append(items, PortForwardListItem{
			RuleID:   tunnel.ruleID,
			TunnelID: id,
			Type:     tunnel.kind,
			Status:   status,
			Error:    errText,
		})
	}
	return items
}

func (s *ForwardService) Snapshot(id string) PortForwardResult {
	s.mu.Lock()
	tunnel := s.tunnels[id]
	s.mu.Unlock()
	if tunnel == nil {
		return PortForwardResult{TunnelID: id, Success: true, Status: "inactive"}
	}
	state, err := tunnel.manager.Snapshot(id)
	if err != nil {
		return PortForwardResult{TunnelID: id, Success: true, Status: "inactive"}
	}
	status := "active"
	if state.Err != "" {
		status = "error"
	}
	return PortForwardResult{TunnelID: id, Success: true, Status: status, Error: state.Err}
}

func (s *ForwardService) RuntimeSnapshot() PortForwardRuntimeSnapshot {
	items := s.List()
	records := make([]PortForwardRuntimeRecord, 0, len(items))
	for _, item := range items {
		records = append(records, PortForwardRuntimeRecord{
			RuleID:   item.RuleID,
			TunnelID: item.TunnelID,
			Phase:    item.Status,
			Error:    item.Error,
		})
	}
	return PortForwardRuntimeSnapshot{Epoch: "wails", Records: records}
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
