package main

import (
	"testing"

	"github.com/lemon-casino/lemonssh/internal/platform/netpolicy"
)

func newTestNetPolicy() *netpolicy.Policy {
	policy := netpolicy.New()
	return policy
}

func TestLoadProviderConfig(t *testing.T) {
	t.Setenv("LEMONSSH_AI_PROVIDER_JSON", "")
	if _, ok, err := loadProviderConfig(); ok || err != nil {
		t.Fatalf("unset config must be ok=false, got ok=%v err=%v", ok, err)
	}

	t.Setenv("LEMONSSH_AI_PROVIDER_JSON", "{bad")
	if _, ok, err := loadProviderConfig(); !ok || err == nil {
		t.Fatalf("malformed JSON must fail with ok=true")
	}

	t.Setenv("LEMONSSH_AI_PROVIDER_JSON", `{"endpoint":"https://api.openai.com/v1","model":"gpt-test"}`)
	if _, _, err := loadProviderConfig(); err == nil {
		t.Fatalf("missing apiKeyValue must fail")
	}

	t.Setenv("LEMONSSH_AI_PROVIDER_JSON", `{"endpoint":"https://api.openai.com/v1","model":"gpt-test","apiKeyValue":"sk-1"}`)
	config, ok, err := loadProviderConfig()
	if !ok || err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if config.APIKeyHeader != "Authorization" || config.MaxIterations != 8 {
		t.Errorf("defaults: header=%q iterations=%d", config.APIKeyHeader, config.MaxIterations)
	}
}

func TestBuildProviderDriverPolicyGuard(t *testing.T) {
	policy := newTestNetPolicy()
	// Explicit provider configs allow arbitrary public https endpoints
	// (custom-endpoint mode) — but private/metadata hosts stay refused.
	_, err := buildProviderDriver(ProviderConfig{
		Endpoint: "http://169.254.169.254/latest", Model: "m", APIKeyValue: "k",
	}, policy, nil, nil, nil)
	if err == nil {
		t.Fatal("metadata endpoint must refuse driver creation")
	}
	_, err = buildProviderDriver(ProviderConfig{
		Endpoint: "https://192.168.1.10/v1", Model: "m", APIKeyValue: "k",
	}, policy, nil, nil, nil)
	if err == nil {
		t.Fatal("private-range endpoint must refuse driver creation")
	}
}

func TestBuildProviderDriverAcceptsBuiltinHost(t *testing.T) {
	policy := newTestNetPolicy()
	driver, err := buildProviderDriver(ProviderConfig{
		Endpoint: "https://api.openai.com/v1/chat/completions", Model: "gpt-test", APIKeyValue: "sk-1",
	}, policy, nil, nil, nil)
	if err != nil {
		t.Fatalf("builtin host must be accepted: %v", err)
	}
	if driver == nil {
		t.Fatal("driver must be built")
	}
}

// TestActivePortForwardLines pins the F12 system-prompt context: the live
// forwardService tunnel table joined with the vault rule metadata, error
// tunnels dropped, metadata-missing tunnels still rendered from RuleID.
func TestActivePortForwardLines(t *testing.T) {
	rules := []any{
		map[string]any{
			"id":          "rule-local",
			"label":       "Web",
			"type":        "local",
			"bindAddress": "127.0.0.1",
			"localPort":   float64(8080),
			"remoteHost":  "db.internal",
			"remotePort":  float64(5432),
		},
		map[string]any{
			"id":          "rule-dynamic",
			"label":       "SOCKS",
			"type":        "dynamic",
			"bindAddress": "0.0.0.0",
			"localPort":   float64(1080),
		},
		map[string]any{
			"id":          "rule-remote",
			"type":        "remote",
			"localPort":   float64(3000),
			"remotePort":  float64(8080),
		},
	}
	tunnels := []PortForwardListItem{
		{RuleID: "rule-local", TunnelID: "t1", Type: "local", Status: "active"},
		{RuleID: "rule-dynamic", TunnelID: "t2", Type: "dynamic", Status: "active"},
		{RuleID: "rule-remote", TunnelID: "t3", Type: "remote", Status: ""},
		{RuleID: "rule-error", TunnelID: "t4", Type: "local", Status: "error"},
		{RuleID: "rule-unknown", TunnelID: "t5", Type: "local", Status: "active"},
	}

	lines := activePortForwardLines(rules, tunnels)
	want := []string{
		"Web (local): 127.0.0.1:8080 -> db.internal:5432 [active]",
		"SOCKS (dynamic): SOCKS5 0.0.0.0:1080 [active]",
		"rule-remote (remote): remote localhost:8080 -> local 127.0.0.1:3000 [active]",
		"rule-unknown (local): 127.0.0.1:0 -> localhost:0 [active]",
	}
	if len(lines) != len(want) {
		t.Fatalf("expected %d lines, got %d: %v", len(want), len(lines), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}

	if got := activePortForwardLines(nil, nil); len(got) != 0 {
		t.Fatalf("empty context must render nothing, got %v", got)
	}
}
