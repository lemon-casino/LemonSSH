package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/binaricat/lemonssh/internal/capability"
	"github.com/binaricat/lemonssh/internal/platform/netpolicy"
)

// ProviderConfig is one explicitly-configured live provider. It arrives
// from the environment (LEMONSSH_AI_PROVIDER_JSON) — never hardcoded
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

// isLoopbackEndpoint reports whether the URL targets 127.0.0.1/localhost
// or ::1 (any port) — the restricted local authorization case of W08.
func isLoopbackEndpoint(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

// loadProviderConfig reads the explicit provider configuration; ok=false
// when unset (the fixture/dev flag path stays authoritative).
func loadProviderConfig() (ProviderConfig, bool, error) {
	raw := os.Getenv("LEMONSSH_AI_PROVIDER_JSON")
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

// activePortForwardLines renders the live port-forward context section of
// the provider system prompt: one line per active tunnel, joining the
// forwardService tunnel table (authoritative lifecycle) with the vault rule
// metadata (label/hosts/ports, redacted at the vault reader). Tunnels in
// error are omitted — a dead forward is not usable context.
func activePortForwardLines(rules []any, tunnels []PortForwardListItem) []string {
	byID := make(map[string]map[string]any, len(rules))
	for _, rule := range rules {
		if typed, ok := rule.(map[string]any); ok {
			if id := stringValue(typed["id"]); id != "" {
				byID[id] = typed
			}
		}
	}
	lines := make([]string, 0, len(tunnels))
	for _, tunnel := range tunnels {
		if tunnel.Status == "error" {
			continue
		}
		rule := byID[tunnel.RuleID]
		label := stringValue(rule["label"])
		kind := tunnel.Type
		if kind == "" {
			kind = stringValue(rule["type"])
		}
		bindAddress := stringValue(rule["bindAddress"])
		if bindAddress == "" {
			bindAddress = "127.0.0.1"
		}
		localPort := numberValue(rule["localPort"])
		remoteHost := stringValue(rule["remoteHost"])
		remotePort := numberValue(rule["remotePort"])
		var detail string
		switch kind {
		case "dynamic":
			kind = "dynamic"
			detail = fmt.Sprintf("SOCKS5 %s:%d", bindAddress, localPort)
		case "remote":
			kind = "remote"
			if remoteHost == "" {
				remoteHost = "localhost"
			}
			detail = fmt.Sprintf("remote %s:%d -> local %s:%d", remoteHost, remotePort, bindAddress, localPort)
		default:
			kind = "local"
			if remoteHost == "" {
				remoteHost = "localhost"
			}
			detail = fmt.Sprintf("%s:%d -> %s:%d", bindAddress, localPort, remoteHost, remotePort)
		}
		if label == "" {
			label = tunnel.RuleID
		}
		status := tunnel.Status
		if status == "" {
			status = "active"
		}
		lines = append(lines, fmt.Sprintf("%s (%s): %s [%s]", label, kind, detail, status))
	}
	return lines
}

// numberValue widens the JSON number shapes the profile store decodes into
// (float64) plus the plain int shapes tests construct.
func numberValue(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int:
		return int64(typed)
	case int64:
		return typed
	case uint16:
		return int64(typed)
	}
	return 0
}

// buildProviderDriver turns an explicit provider config into a live turn
// driver behind the netpolicy-enforced HTTP client. Local inference
// servers (Ollama/LM Studio on loopback) use the restricted local
// authorization branch; other endpoints must pass the custom-endpoint
// guard (private ranges and metadata hosts stay refused).
func buildProviderDriver(config ProviderConfig, policy *netpolicy.Policy, sessions func() []SessionEntry, portForwards func() []string, dispatcher *capability.Dispatcher) (*ProviderDriver, error) {
	if isLoopbackEndpoint(config.Endpoint) {
		policy.AddProviderEndpoint(config.Endpoint)
	} else if !policy.IsAllowedURL(config.Endpoint, netpolicy.FetchOptions{AllowCustomEndpoints: true}) {
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
