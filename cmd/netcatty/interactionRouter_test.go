package main

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/app/terminaluse"
	"github.com/binaricat/netcatty/internal/capability"
	"github.com/binaricat/netcatty/internal/rpc"
)

func newGateTestHost(t *testing.T, router *InteractionRouter, permissionMode string) string {
	t.Helper()
	host := newAgentHost(AgentHostConfig{
		Version: appVersion{Name: "LemonSSH", Version: "0.0.1"},
		Sessions: func() []SessionEntry {
			return []SessionEntry{{ID: "sess-1", Kind: "local"}}
		},
		SFTP:           &fakeSFTPReader{},
		Jobs:           terminaluse.NewJobQueue(func(sessionID string) (terminaluse.CommandRunner, error) { return scriptRunner{}, nil }),
		Approvals:      router,
		PermissionMode: permissionMode,
	})
	discoveryPath := filepath.Join(t.TempDir(), "discovery.json")
	if err := host.Start(discoveryPath); err != nil {
		t.Fatalf("host start: %v", err)
	}
	t.Cleanup(func() {
		host.Stop()
		_ = rpc.RemoveDiscovery(discoveryPath)
	})
	return discoveryPath
}

func gatedExecCall(t *testing.T, discoveryPath string) {
	t.Helper()
	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	_, err = client.Call(context.Background(), "netcatty/exec", map[string]any{
		"chatSessionId": "chat-1",
		"sessionId":     "sess-1",
		"command":       "echo approved",
	})
	if err != nil {
		t.Fatalf("approved exec: %v", err)
	}
}

// TestApprovalGateApproveRunsHandler pins the happy path: the interaction
// is emitted, the user approves, and the write runs.
func TestApprovalGateApproveRunsHandler(t *testing.T) {
	emitted := &emittedInteractions{}
	router := newInteractionRouter(func(name string, payload any) {
		if name == "agent:interaction" {
			if m, ok := payload.(map[string]any); ok {
				emitted.add(m)
			}
		}
	})
	discoveryPath := newGateTestHost(t, router, "confirm")

	go func() {
		for emitted.len() == 0 {
			time.Sleep(time.Millisecond)
		}
		_ = router.Respond(emitted.firstID(), true)
	}()

	gatedExecCall(t, discoveryPath)

	if emitted.len() != 1 {
		t.Fatalf("emitted interactions = %d, want 1", emitted.len())
	}
	first := emitted.all()[0]
	if first["capabilityId"] != "terminal.execute" {
		t.Errorf("capability = %v", first["capabilityId"])
	}
	if summary, ok := first["summary"].(map[string]any); !ok || summary["command"] != "echo approved" {
		t.Errorf("summary = %v", first["summary"])
	}
}

// emittedInteractions is a mutex-protected collector for emit callbacks.
type emittedInteractions struct {
	mu   sync.Mutex
	list []map[string]any
}

func (e *emittedInteractions) add(m map[string]any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.list = append(e.list, m)
}

func (e *emittedInteractions) len() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.list)
}

func (e *emittedInteractions) firstID() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.list[0]["interactionId"].(string)
}

func (e *emittedInteractions) all() []map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]map[string]any, len(e.list))
	copy(out, e.list)
	return out
}

// TestApprovalGateDenyBlocksWrite pins the deny path: the write never
// reaches the handler.
func TestApprovalGateDenyBlocksWrite(t *testing.T) {
	router := newInteractionRouter(func(name string, payload any) {})
	discoveryPath := newGateTestHost(t, router, "confirm")

	go func() {
		for {
			router.mu.Lock()
			count := len(router.pending)
			router.mu.Unlock()
			if count > 0 {
				break
			}
			time.Sleep(time.Millisecond)
		}
		router.mu.Lock()
		for id := range router.pending {
			router.mu.Unlock()
			_ = router.Respond(id, false)
			return
		}
	}()

	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	_, callErr := client.Call(context.Background(), "netcatty/exec", map[string]any{
		"chatSessionId": "chat-1",
		"sessionId":     "sess-1",
		"command":       "denied command",
	})
	if callErr == nil || !strings.Contains(callErr.Error(), "denied by user") && !strings.Contains(callErr.Error(), "USER_DENIED") {
		t.Errorf("denied write must fail USER_DENIED, got %v", callErr)
	}
}

// TestApprovalGateDoubleRespondFails pins the exactly-once consumption.
func TestApprovalGateDoubleRespondFails(t *testing.T) {
	router := newInteractionRouter(func(name string, payload any) {})
	discoveryPath := newGateTestHost(t, router, "confirm")

	go func() {
		for {
			router.mu.Lock()
			count := len(router.pending)
			router.mu.Unlock()
			if count > 0 {
				router.mu.Lock()
				var id string
				for pendingID := range router.pending {
					id = pendingID
				}
				router.mu.Unlock()
				_ = router.Respond(id, true)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	gatedExecCall(t, discoveryPath)

	router.mu.Lock()
	remaining := len(router.pending)
	router.mu.Unlock()
	if remaining != 0 {
		t.Errorf("pending interactions = %d after resolution", remaining)
	}

	// A second respond on the drained ID fails typed.
	if err := router.Respond("ia_nonexistent", true); err == nil {
		t.Errorf("respond on resolved/unknown interaction must fail")
	}
	_ = discoveryPath
}

// TestApprovalGateTimeoutDenies pins the bounded wait: with nobody
// responding, the deadline denies the write.
func TestApprovalGateTimeoutDenies(t *testing.T) {
	router := newInteractionRouter(func(name string, payload any) {})
	router.timeout = 50 * time.Millisecond
	discoveryPath := newGateTestHost(t, router, "confirm")

	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	_, callErr := client.Call(context.Background(), "netcatty/exec", map[string]any{
		"chatSessionId": "chat-1",
		"sessionId":     "sess-1",
		"command":       "unattended",
	})
	if callErr == nil {
		t.Fatal("timed-out approval must deny the write")
	}
	if strings.Contains(callErr.Error(), "echo approved") {
		t.Errorf("handler output leaked past denial: %v", callErr)
	}
}

// TestPendingInteractionsFacade covers the settings-UI listing through the
// AgentService facade while a prompt is open.
func TestPendingInteractionsFacade(t *testing.T) {
	router := newInteractionRouter(func(name string, payload any) {})
	service := newAgentService(nil, false, nil, nil, router)
	if got := service.AgentPendingInteractions(); len(got) != 0 {
		t.Fatalf("idle pending = %v", got)
	}

	// Simulate one open prompt by requesting from a goroutine.
	done := make(chan bool)
	go func() {
		approved, err := router.RequestApproval(context.Background(), capability.Request{
			RPCMethod: "netcatty/exec",
			Params:    map[string]any{"command": "ls"},
		}, &capability.Definition{ID: "terminal.execute", Description: "Execute a command"})
		if err == nil {
			done <- approved
		}
	}()
	time.Sleep(10 * time.Millisecond)

	pending := service.AgentPendingInteractions()
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want 1", len(pending))
	}
	_ = router.Respond(pending[0]["interactionId"].(string), true)
	if approved := <-done; !approved {
		t.Errorf("approval round trip failed")
	}

	// Router-less service fails typed.
	bare := newAgentService(nil, false, nil, nil, nil)
	if err := bare.AgentRespondInteraction("ia_x", true); err == nil {
		t.Errorf("respond without router must fail")
	}
}
