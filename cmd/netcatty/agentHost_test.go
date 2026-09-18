package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/binaricat/netcatty/internal/rpc"
)

func newTestAgentHost(t *testing.T) (*AgentHost, string) {
	t.Helper()
	host := newAgentHost(AgentHostConfig{
		Version: appVersion{Name: "LemonSSH", Version: "0.0.1", GOOS: "windows", GOARCH: "amd64"},
		Sessions: func() []SessionEntry {
			return []SessionEntry{
				{ID: "sess-1", Label: "prod shell", Kind: "ssh"},
				{ID: "sess-2", Label: "local", Kind: "local"},
			}
		},
	})
	path := filepath.Join(t.TempDir(), "agent-rpc-discovery.json")
	if err := host.Start(path); err != nil {
		t.Fatalf("host start: %v", err)
	}
	t.Cleanup(func() {
		host.Stop()
		_ = rpc.RemoveDiscovery(path)
	})
	return host, path
}

// TestAgentHostStatusRoundTrip drives the full stack a native binary uses:
// discovery file -> loopback dial -> authenticated call -> JSON result.
func TestAgentHostStatusRoundTrip(t *testing.T) {
	_, discoveryPath := newTestAgentHost(t)
	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	result, err := client.Call(context.Background(), "netcatty/getStatus", map[string]any{})
	if err != nil {
		t.Fatalf("getStatus: %v", err)
	}
	var status map[string]any
	if err := json.Unmarshal(result, &status); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if status["name"] != "LemonSSH" || status["permissionMode"] != "confirm" {
		t.Errorf("status payload = %v", status)
	}
}

func TestAgentHostContextScopedToPrincipal(t *testing.T) {
	_, discoveryPath := newTestAgentHost(t)
	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	result, err := client.Call(context.Background(), "netcatty/getContext", map[string]any{})
	if err != nil {
		t.Fatalf("getContext: %v", err)
	}
	var context map[string]any
	if err := json.Unmarshal(result, &context); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	sessions, ok := context["sessions"].([]any)
	if !ok || len(sessions) != 2 {
		t.Errorf("sessions = %v", context["sessions"])
	}
}

func TestAgentHostUnknownMethodTyped(t *testing.T) {
	_, discoveryPath := newTestAgentHost(t)
	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	_, err = client.Call(context.Background(), "netcatty/nonexistent", map[string]any{})
	if err == nil {
		t.Fatal("unknown method must fail")
	}
	var rpcErr *rpc.RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != "UNKNOWN_METHOD" {
		t.Fatalf("expected UNKNOWN_METHOD RPCError, got %v", err)
	}
}

// TestAgentHostDiscoveryRemoved pins the shutdown contract: Stop leaves no
// discovery file behind, so stale launchers fail with the typed
// unavailable message instead of dialing a dead port.
func TestAgentHostDiscoveryRemoved(t *testing.T) {
	host, discoveryPath := newTestAgentHost(t)
	host.Stop()
	if _, err := rpc.LoadDiscovery(discoveryPath); err == nil {
		t.Fatal("discovery must be removed on stop")
	}
}
