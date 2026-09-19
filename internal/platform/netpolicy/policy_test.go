package netpolicy

import (
	"net/netip"
	"testing"
)

func TestIsPrivateIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"127.9.9.9", true},
		{"10.1.2.3", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"172.32.0.1", false},
		{"192.168.1.1", true},
		{"169.254.169.254", true},
		{"100.64.0.1", true},
		{"100.127.255.255", true},
		{"100.128.0.1", false},
		{"0.0.0.0", true},
		{"8.8.8.8", false},
		{"::1", true},
		{"::", true},
		{"fd12:3456::1", true},
		{"fe80::1", true},
		{"::ffff:127.0.0.1", true},
		{"::ffff:8.8.8.8", false},
		{"2606:4700::1", false},
		{"not-an-ip", true}, // unparsable fails closed
	}
	for _, tc := range cases {
		got := IsPrivateIP(parseOrInvalid(tc.ip))
		if got != tc.want {
			t.Errorf("IsPrivateIP(%s) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func parseOrInvalid(s string) netip.Addr {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}
	}
	return addr
}

func newTestPolicy() *Policy {
	policy := New()
	policy.AddProviderEndpoint("https://custom-provider.example.com/v1")
	policy.AddProviderEndpoint("http://insecure-provider.example.com/v1")
	policy.AddProviderEndpoint("http://127.0.0.1:9999") // extends local ports
	policy.AddProviderEndpoint("https://localhost:4321")
	policy.SetWebSearchHost("http://search.internal.example:8080/v1")
	return policy
}

func TestIsAllowedURLNormalMode(t *testing.T) {
	policy := newTestPolicy()

	allowed := []string{
		"https://api.openai.com/v1/chat/completions",
		"https://api.anthropic.com/v1/messages",
		"https://generativelanguage.googleapis.com/v1/models",
		"https://openrouter.ai/api/v1",
		"https://custom-provider.example.com/v1/chat",
		"http://insecure-provider.example.com/v1/chat", // explicit http baseURL
		"http://127.0.0.1:11434/api/tags",              // Ollama builtin port
		"http://127.0.0.1:9999/v1",                     // provider-registered port
		"https://localhost:4321/v1",                    // provider-registered port
		"ftp://127.0.0.1:11434",                        // loopback port decision precedes scheme check (CJS parity); transport rejects the dial
		"http://127.0.0.1:4321/v1",                     // registered localhost ports are scheme-agnostic (CJS parity)
	}
	for _, raw := range allowed {
		if !policy.IsAllowedURL(raw, FetchOptions{}) {
			t.Errorf("expected allowed: %s", raw)
		}
	}

	denied := []string{
		"https://evil.example.com/v1",                      // not allowlisted
		"http://api.openai.com/v1",                         // plain http to builtin host
		"http://127.0.0.1:22",                              // loopback port not allowed
		"https://metadata.google.internal/computeMetadata", // metadata host
		// http web search host passes the scheme gate but still needs provider
		// registration to clear the final allowlist (CJS parity).
		"http://search.internal.example:8080/v1/search",
	}

	for _, raw := range denied {
		if policy.IsAllowedURL(raw, FetchOptions{}) {
			t.Errorf("expected denied: %s", raw)
		}
	}
}

func TestIsAllowedURLCustomEndpointMode(t *testing.T) {
	policy := newTestPolicy()
	opts := FetchOptions{AllowCustomEndpoints: true}

	if !policy.IsAllowedURL("https://my-own-server.example.com/api", opts) {
		t.Errorf("custom endpoints allow arbitrary public https")
	}
	denied := []string{
		"http://my-own-server.example.com/api",             // https mandatory
		"https://127.0.0.1:11434/v1",                       // loopback still blocked
		"https://metadata.google.internal/computeMetadata", // metadata still blocked
		"https://192.168.1.10/v1",                          // private ranges still blocked
		"https://[fd12::1]/v1",                             // IPv6 ULA still blocked
	}
	for _, raw := range denied {
		if policy.IsAllowedURL(raw, opts) {
			t.Errorf("expected denied in custom mode: %s", raw)
		}
	}
}

func TestProviderLoopbackRegistration(t *testing.T) {
	policy := New()
	policy.AddProviderEndpoint("http://127.0.0.1:9876/v1")
	if !policy.IsAllowedURL("http://127.0.0.1:9876/v1/chat", FetchOptions{}) {
		t.Errorf("registered loopback endpoint must be allowed")
	}
	if !policy.IsAllowedURL("http://localhost:9876/v1/chat", FetchOptions{}) {
		t.Errorf("localhost alias must also pass")
	}
	if policy.IsAllowedURL("http://127.0.0.1:9877/v1", FetchOptions{}) {
		t.Errorf("unregistered loopback port must stay gated")
	}
}
