package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/binaricat/netcatty/internal/app/terminaluse"
	"github.com/binaricat/netcatty/internal/profile/store"
	"github.com/binaricat/netcatty/internal/rpc"
)

// TestForwardReadDomainThroughDispatch drives the forward read surface
// over a seeded vault store, plus the confirm-mode stop fail-closed rule.
func TestForwardReadDomainThroughDispatch(t *testing.T) {
	dir := t.TempDir()
	profileStore, err := store.Open(filepath.Join(dir, "profile.db"), nil)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = profileStore.Close() })
	if _, err := profileStore.Write(store.WriteRequest{Mutations: []store.Mutation{
		{Domain: "vault", Key: "netcatty_port_forwarding_v1", Value: []byte(`[
			{"id":"rule-1","label":"web","type":"local","localPort":8080,"remoteHost":"10.0.0.5","remotePort":80,"hostId":"host-1","status":"idle"}
		]`)},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	queue := terminaluse.NewJobQueue(func(sessionID string) (terminaluse.CommandRunner, error) {
		return scriptRunner{}, nil
	})
	host := newAgentHost(AgentHostConfig{
		Version: appVersion{Name: "LemonSSH", Version: "0.0.1"},
		Sessions: func() []SessionEntry {
			return []SessionEntry{{ID: "sess-1", Kind: "ssh"}}
		},
		Jobs:           queue,
		Vault:          newVaultReader(profileStore),
		Forwards:       NewForwardService(nil, nil),
		PermissionMode: "auto",
	})
	discoveryPath := filepath.Join(t.TempDir(), "discovery.json")
	if err := host.Start(discoveryPath); err != nil {
		t.Fatalf("host start: %v", err)
	}
	t.Cleanup(func() {
		host.Stop()
		_ = rpc.RemoveDiscovery(discoveryPath)
	})
	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	// Rules list: persisted rules read back redacted.
	rules, err := client.Call(context.Background(), "portforward/rules/list", map[string]any{})
	if err != nil {
		t.Fatalf("rules list: %v", err)
	}
	if !strings.Contains(string(rules), `"ruleId":"rule-1"`) && !strings.Contains(string(rules), `"id":"rule-1"`) {
		t.Errorf("rules = %s", rules)
	}

	// Tunnels list: empty but live from forwarduse.
	tunnels, err := client.Call(context.Background(), "portforward/tunnels/list", map[string]any{})
	if err != nil {
		t.Fatalf("tunnels list: %v", err)
	}
	if !strings.Contains(string(tunnels), "ok") {
		t.Errorf("tunnels = %s", tunnels)
	}

	// Stop in auto mode: no tunnel for the rule, forwarded to forwarduse
	// which reports an empty stop result.
	stopped, err := client.Call(context.Background(), "portforward/stop", map[string]any{"ruleId": "rule-1"})
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	_ = stopped
}

// TestForwardStopConfirmModeFailsClosed pins the write-domain rule:
// portforward.stop is a write and confirm mode without an approval gate
// refuses it before any handler runs.
func TestForwardStopConfirmModeFailsClosed(t *testing.T) {
	host := newAgentHost(AgentHostConfig{
		Version:        appVersion{Name: "LemonSSH", Version: "0.0.1"},
		Forwards:       NewForwardService(nil, nil),
		PermissionMode: "confirm",
		Sessions:       func() []SessionEntry { return nil },
	})
	discoveryPath := filepath.Join(t.TempDir(), "discovery.json")
	if err := host.Start(discoveryPath); err != nil {
		t.Fatalf("host start: %v", err)
	}
	t.Cleanup(func() {
		host.Stop()
		_ = rpc.RemoveDiscovery(discoveryPath)
	})
	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	_, err = client.Call(context.Background(), "portforward/stop", map[string]any{"ruleId": "rule-1"})
	var rpcErr *rpc.RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != "APPROVAL_GATE_UNAVAILABLE" {
		t.Fatalf("confirm stop must fail APPROVAL_GATE_UNAVAILABLE, got %v", err)
	}
}

// errorsAs is a local errors.As wrapper for the rpc error type.
func errorsAs(err error, target **rpc.RPCError) bool {
	if e, ok := err.(*rpc.RPCError); ok {
		*target = e
		return true
	}
	return false
}
