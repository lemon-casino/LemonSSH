package netpolicy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHTTPProxySettingsValidateAndNormalize(t *testing.T) {
	policy := New()
	if err := policy.SetHTTPProxy("custom", "ftp://proxy.example", ""); err == nil {
		t.Fatal("unsupported proxy scheme accepted")
	}
	if err := policy.SetHTTPProxy("custom", "http://user:secret@proxy.example", ""); err == nil {
		t.Fatal("proxy URL credentials accepted")
	}
	if err := policy.SetHTTPProxy("custom", "socks5://proxy.example", "<LOCAL>, *.example.com"); err != nil {
		t.Fatal(err)
	}
	mode, rawURL, bypass := policy.HTTPProxy()
	if mode != "custom" || rawURL != "socks5://proxy.example" || bypass != "<local>,*.example.com" {
		t.Fatalf("unexpected normalized proxy settings: %q %q %q", mode, rawURL, bypass)
	}
}

func TestCustomProxyBypassPatterns(t *testing.T) {
	patterns := []string{"<local>", "exact.example", "*.internal.example", ".suffix.example"}
	for _, host := range []string{"printer", "exact.example", "api.internal.example", "api.suffix.example"} {
		if !shouldBypassProxy(host, patterns) {
			t.Fatalf("expected bypass for %q", host)
		}
	}
	if shouldBypassProxy("api.openai.com", patterns) {
		t.Fatal("public provider host was unexpectedly bypassed")
	}
}

func TestCanonicalProxyAddressAddsDefaultPorts(t *testing.T) {
	cases := map[string]string{
		"http://proxy.example":   "proxy.example:80",
		"https://proxy.example":  "proxy.example:443",
		"socks5://proxy.example": "proxy.example:1080",
	}
	for raw, expected := range cases {
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if actual := canonicalProxyAddress(parsed); actual != expected {
			t.Fatalf("canonicalProxyAddress(%q) = %q, want %q", raw, actual, expected)
		}
	}
}

func TestCustomLoopbackProxyCanReachAllowlistedProvider(t *testing.T) {
	var requestedURL string
	proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestedURL = request.URL.String()
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "proxied")
	}))
	defer proxy.Close()

	policy := New()
	policy.AddProviderEndpoint("http://provider.example/v1")
	if err := policy.SetHTTPProxy("custom", proxy.URL, ""); err != nil {
		t.Fatal(err)
	}
	response, err := policy.NewClient(ClientOptions{}).Get("http://provider.example/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if !strings.Contains(requestedURL, "provider.example") || string(body) != "proxied" {
		t.Fatalf("request did not traverse the configured proxy: url=%q body=%q", requestedURL, body)
	}
}
