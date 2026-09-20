package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/binaricat/netcatty/internal/platform/netpolicy"
)

func newTestProviderFetchService() *ProviderFetchService {
	return newProviderFetchService(netpolicy.New())
}

func TestProviderFetchDeniesHostOutsideAllowlist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	service := newTestProviderFetchService()
	// A loopback port outside the builtin allowlist stays refused until the
	// renderer registers the provider baseURL (Issue #9 SSRF guard).
	result := service.Fetch(ProviderFetchRequest{URL: server.URL + "/v1/models"})
	if result.OK {
		t.Fatalf("unregistered loopback port must be refused, got %+v", result)
	}
	if result.Error != "URL host is not in the allowed list" {
		t.Fatalf("unexpected error message: %q", result.Error)
	}
}

func TestProviderFetchAllowlistAddHostUnlocksRegisteredLoopback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer ollama" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"qwen2.5:7b"}]}`))
	}))
	defer server.Close()

	service := newTestProviderFetchService()
	if !service.AllowlistAddHost(server.URL).OK {
		t.Fatal("allowlist registration must succeed")
	}
	result := service.Fetch(ProviderFetchRequest{
		URL:     server.URL + "/v1/models",
		Method:  http.MethodGet,
		Headers: map[string]string{"Authorization": "Bearer ollama"},
	})
	if !result.OK || result.Status != http.StatusOK {
		t.Fatalf("registered loopback provider must be reachable, got %+v", result)
	}
	if !strings.Contains(result.Data, "qwen2.5:7b") {
		t.Fatalf("response body must round-trip, got %q", result.Data)
	}
}

func TestProviderFetchSyncProvidersRegistersConfiguredHosts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"model-a"}]}`))
	}))
	defer server.Close()

	service := newTestProviderFetchService()
	if denied := service.Fetch(ProviderFetchRequest{URL: server.URL}); denied.OK {
		t.Fatal("provider host must start denied")
	}
	// Keys never cross this boundary: only endpoint metadata is read.
	synced := service.SyncProviders([]ProviderEndpointConfig{{
		ID:         "prov-1",
		ProviderID: "openai",
		BaseURL:    server.URL,
		Enabled:    true,
	}})
	if !synced.OK {
		t.Fatalf("sync must succeed, got %+v", synced)
	}
	if allowed := service.Fetch(ProviderFetchRequest{URL: server.URL}); !allowed.OK {
		t.Fatalf("synced provider host must be allowed, got %+v", allowed)
	}
}

func TestProviderFetchSyncWebSearchInjectsCredentialForConfiguredOrigin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Fatalf("web search credential was not injected: %q", request.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()

	service := newTestProviderFetchService()
	service.credentials = streamCredentialFixture{}
	sealed := "enc:v1:" + base64.StdEncoding.EncodeToString([]byte("encrypted-fixture"))
	if result := service.SyncWebSearch(server.URL, sealed); !result.OK {
		t.Fatalf("web search sync failed: %+v", result)
	}
	result := service.Fetch(ProviderFetchRequest{
		URL:     server.URL + "/search",
		Method:  http.MethodPost,
		Headers: map[string]string{"Authorization": "Bearer " + webSearchKeyPlaceholder},
	})
	if !result.OK {
		t.Fatalf("web search fetch failed: %+v", result)
	}

	if _, err := service.authorizeProviderRequest(ProviderFetchRequest{
		URL:     "https://api.tavily.com/search",
		Headers: map[string]string{"Authorization": "Bearer " + webSearchKeyPlaceholder},
	}); err == nil || !strings.Contains(err.Error(), "configured endpoint") {
		t.Fatalf("web search credential must stay bound to its configured origin, got %v", err)
	}
}

func TestProviderFetchRejectsPrivateHostsEvenWhenSkippingHostCheck(t *testing.T) {
	service := newTestProviderFetchService()
	for _, url := range []string{
		"https://10.0.0.5/v1/models",                 // RFC1918
		"https://169.254.169.254/latest/meta-data",   // link-local
		"https://metadata.google.internal/v1/models", // metadata host
		"http://public.example.com/v1/models",        // custom mode requires https
		"https://100.100.100.200/v1/models",          // CGNAT range
	} {
		result := service.Fetch(ProviderFetchRequest{URL: url, SkipHostCheck: true})
		if result.OK {
			t.Fatalf("skipHostCheck must never reach %s, got %+v", url, result)
		}
	}
}

func TestProviderFetchDisabledRedirectsReturnTheRedirectResponse(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"final"}]}`))
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.Redirect(w, request, target.URL+"/v1/models", http.StatusFound)
	}))
	defer redirector.Close()

	service := newTestProviderFetchService()
	service.AllowlistAddHost(redirector.URL)
	service.AllowlistAddHost(target.URL)

	withoutFollow := service.Fetch(ProviderFetchRequest{URL: redirector.URL + "/v1/models"})
	if withoutFollow.Status != http.StatusFound || withoutFollow.OK {
		t.Fatalf("disabled redirects must return the 3xx response, got %+v", withoutFollow)
	}
	withFollow := service.Fetch(ProviderFetchRequest{URL: redirector.URL + "/v1/models", FollowRedirects: true})
	if !withFollow.OK || !strings.Contains(withFollow.Data, "final") {
		t.Fatalf("followRedirects must reach the target, got %+v", withFollow)
	}
}

func TestProviderFetchRejectsEmptyURL(t *testing.T) {
	service := newTestProviderFetchService()
	if result := service.Fetch(ProviderFetchRequest{}); result.Error != "Invalid URL" {
		t.Fatalf("empty URL must be reported as invalid, got %+v", result)
	}
	if result := service.AllowlistAddHost("  "); result.OK || result.Error == "" {
		t.Fatalf("blank baseURL must be rejected, got %+v", result)
	}
}
