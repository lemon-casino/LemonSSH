package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/binaricat/netcatty/internal/capability"
	"github.com/binaricat/netcatty/internal/platform/netpolicy"
)

// ProviderConfig is one explicitly-configured live provider. It arrives
// from the environment (NETCATTY_AI_PROVIDER_JSON) — never hardcoded
// keys. Endpoint must pass the netpolicy URL guard before any traffic.
type ProviderConfig struct {
	Family        string `json:"family"` // openai | anthropic | google
	Endpoint      string `json:"endpoint"`
	APIKeyHeader  string `json:"apiKeyHeader"`
	APIKeyValue   string `json:"apiKeyValue"`
	Model         string `json:"model"`
	SystemBase    string `json:"systemBase,omitempty"`
	MaxIterations int    `json:"maxIterations,omitempty"`
}

// loadProviderConfig reads the explicit provider configuration; ok=false
// when unset (the fixture/dev flag path stays authoritative).
func loadProviderConfig() (ProviderConfig, bool, error) {
	raw := os.Getenv("NETCATTY_AI_PROVIDER_JSON")
	if raw == "" {
		return ProviderConfig{}, false, nil
	}
	var config ProviderConfig
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return ProviderConfig{}, true, fmt.Errorf("provider config is not valid JSON: %w", err)
	}
	if config.Endpoint == "" || config.Model == "" || strings.TrimSpace(config.APIKeyValue) == "" {
		return ProviderConfig{}, true, fmt.Errorf("provider config requires endpoint, model and apiKeyValue")
	}
	if config.APIKeyHeader == "" {
		config.APIKeyHeader = "Authorization"
	}
	if config.MaxIterations <= 0 {
		config.MaxIterations = 8
	}
	return config, true, nil
}

// buildProviderDriver turns an explicit provider config into a live turn
// driver behind the netpolicy-enforced HTTP client. The URL must be
// allowlisted by policy before any dial happens.
func buildProviderDriver(config ProviderConfig, policy *netpolicy.Policy, sessions func() []SessionEntry, portForwards func() []string, dispatcher *capability.Dispatcher) (*ProviderDriver, error) {
	if !policy.IsAllowedURL(config.Endpoint, netpolicy.FetchOptions{AllowCustomEndpoints: true}) {
		return nil, fmt.Errorf("provider endpoint %q is not allowed by network policy", config.Endpoint)
	}
	client := policy.NewClient(netpolicy.ClientOptions{
		FetchOptions: netpolicy.FetchOptions{AllowCustomEndpoints: true},
		MaxRedirects: -1, // providers follow redirects only through policy re-check; disable stdlib auto-follow
	})
	apiKeyValue := config.APIKeyValue
	if config.Family == "" || config.Family == "openai" {
		apiKeyValue = "Bearer " + config.APIKeyValue
	}
	return NewProviderDriver(
		client, config.Endpoint, config.APIKeyHeader, apiKeyValue, config.Model,
		config.SystemBase, config.MaxIterations, sessions, portForwards, dispatcher,
	), nil
}
