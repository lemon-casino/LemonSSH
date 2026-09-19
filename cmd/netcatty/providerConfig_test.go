package main

import (
	"testing"

	"github.com/binaricat/netcatty/internal/platform/netpolicy"
)

func newTestNetPolicy() *netpolicy.Policy {
	policy := netpolicy.New()
	return policy
}

func TestLoadProviderConfig(t *testing.T) {
	t.Setenv("NETCATTY_AI_PROVIDER_JSON", "")
	if _, ok, err := loadProviderConfig(); ok || err != nil {
		t.Fatalf("unset config must be ok=false, got ok=%v err=%v", ok, err)
	}

	t.Setenv("NETCATTY_AI_PROVIDER_JSON", "{bad")
	if _, ok, err := loadProviderConfig(); !ok || err == nil {
		t.Fatalf("malformed JSON must fail with ok=true")
	}

	t.Setenv("NETCATTY_AI_PROVIDER_JSON", `{"endpoint":"https://api.openai.com/v1","model":"gpt-test"}`)
	if _, _, err := loadProviderConfig(); err == nil {
		t.Fatalf("missing apiKeyValue must fail")
	}

	t.Setenv("NETCATTY_AI_PROVIDER_JSON", `{"endpoint":"https://api.openai.com/v1","model":"gpt-test","apiKeyValue":"sk-1"}`)
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
