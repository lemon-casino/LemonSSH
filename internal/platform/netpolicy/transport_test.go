package netpolicy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

// newLocalProvider registers one httptest server as a local provider
// endpoint, mirroring a configured local inference server.
func newLocalProvider(t *testing.T, policy *Policy, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	policy.AddProviderEndpoint(server.URL + "/")
	return server
}

func TestTransportAllowsRegisteredLocalEndpoint(t *testing.T) {
	policy := New()
	server := newLocalProvider(t, policy, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("model says hi"))
	}))
	client := policy.NewClient(ClientOptions{})

	resp, err := client.Get(server.URL + "/v1/chat")
	if err != nil {
		t.Fatalf("call local provider: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "model says hi" {
		t.Errorf("body = %q", body)
	}
}

func TestTransportDeniesUnregisteredHost(t *testing.T) {
	policy := New()
	client := policy.NewClient(ClientOptions{})
	_, err := client.Get("https://evil.example.com/v1")
	if err == nil || !errors.Is(err, ErrURLDenied) {
		t.Fatalf("unregistered host must fail ErrURLDenied, got %v", err)
	}
}

func TestTransportRedirectRevalidation(t *testing.T) {
	policy := New()
	target := newLocalProvider(t, policy, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("after redirect"))
	}))
	// httptest servers share one port set; a redirect to another local
	// provider endpoint exercises allowlist checks, so register both.
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/final", http.StatusFound)
	}))
	t.Cleanup(redirector.Close)
	policy.AddProviderEndpoint(redirector.URL + "/")
	// Redirect to the allowlisted local endpoint must succeed.
	client := policy.NewClient(ClientOptions{})
	resp, err := client.Get(redirector.URL + "/start")
	if err != nil {
		t.Fatalf("redirect within allowlist: %v", err)
	}
	_ = resp.Body.Close()

	// Redirect to a private address must be refused (T48).
	metadataRedirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data", http.StatusFound)
	}))
	t.Cleanup(metadataRedirector.Close)
	policy.AddProviderEndpoint(metadataRedirector.URL + "/")

	_, err = client.Get(metadataRedirector.URL + "/start")
	if err == nil || !errors.Is(err, ErrRedirectDenied) {
		t.Fatalf("redirect to metadata must fail ErrRedirectDenied, got %v", err)
	}
}

func TestTransportResponseBodyLimit(t *testing.T) {
	policy := New()
	server := newLocalProvider(t, policy, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 4096)))
	}))
	client := policy.NewClient(ClientOptions{MaxResponseBodyBytes: 1024})

	resp, err := client.Get(server.URL + "/big")
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	defer resp.Body.Close()
	_, readErr := io.ReadAll(resp.Body)
	if readErr == nil || !errors.Is(readErr, ErrBodyTooLarge) {
		t.Fatalf("oversized body must fail ErrBodyTooLarge, got %v", readErr)
	}
}

// TestDialGuardRefusesRebinding pins the DNS-to-dial guarantee: a public
// hostname resolving to a metadata address is refused before connecting.
func TestDialGuardRefusesRebinding(t *testing.T) {
	policy := New()
	policy.AddProviderEndpoint("https://rebind.example/v1")
	client := policy.NewClient(ClientOptions{
		Resolver: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("169.254.169.254")}, nil
		},
	})
	_, err := client.Get("https://rebind.example/v1/chat")
	if err == nil || !errors.Is(err, ErrDialDenied) {
		t.Fatalf("rebind to metadata must fail ErrDialDenied, got %v", err)
	}
}

// TestResolveAndValidateGuard pins the dial guard decisions directly:
// public resolutions pass, private resolutions on non-local targets are
// refused, and local-allowed targets may keep loopback addresses.
func TestResolveAndValidateGuard(t *testing.T) {
	policy := New()

	public := netip.MustParseAddr("93.184.216.34")
	metadata := netip.MustParseAddr("169.254.169.254")

	// Use the resolver path for determinism.
	withResolver := ClientOptions{Resolver: func(ctx context.Context, host string) ([]netip.Addr, error) {
		return []netip.Addr{public}, nil
	}}
	if _, err := policy.resolveAndValidate(context.Background(), withResolver, "clean.example", false); err != nil {
		t.Errorf("public resolution must pass, got %v", err)
	}

	withMetadata := ClientOptions{Resolver: func(ctx context.Context, host string) ([]netip.Addr, error) {
		return []netip.Addr{metadata}, nil
	}}
	if _, err := policy.resolveAndValidate(context.Background(), withMetadata, "rebind.example", false); !errors.Is(err, ErrDialDenied) {
		t.Errorf("metadata resolution must be refused, got %v", err)
	}

	withLoopback := ClientOptions{Resolver: func(ctx context.Context, host string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}}
	if _, err := policy.resolveAndValidate(context.Background(), withLoopback, "localhost", true); err != nil {
		t.Errorf("local-allowed target may resolve loopback, got %v", err)
	}
	if _, err := policy.resolveAndValidate(context.Background(), withLoopback, "rebind.example", false); !errors.Is(err, ErrDialDenied) {
		t.Errorf("loopback resolution on remote target must be refused, got %v", err)
	}
}

func TestRedirectLimit(t *testing.T) {
	policy := New()
	loop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/again", http.StatusFound)
	}))
	t.Cleanup(loop.Close)
	policy.AddProviderEndpoint(loop.URL + "/")

	client := policy.NewClient(ClientOptions{MaxRedirects: 2})
	_, err := client.Get(loop.URL + "/start")
	if err == nil || !strings.Contains(err.Error(), "stopped after") {
		t.Fatalf("redirect loop must hit the limit, got %v", err)
	}
}
