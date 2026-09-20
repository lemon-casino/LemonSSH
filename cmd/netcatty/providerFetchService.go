package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/binaricat/netcatty/internal/platform/credentials"
	"github.com/binaricat/netcatty/internal/platform/netpolicy"
)

// ProviderFetchService is the Go owner for renderer provider traffic: the
// settings model-list/probe fetch (aiFetch), the temporary discovery
// allowlist (aiAllowlistAddHost) and the provider-config allowlist rebuild
// (aiSyncProviders). Every request runs through the netpolicy authority, so
// URL, dial, redirect and body limits match providerHandlers.cjs.
type ProviderFetchService struct {
	policy      *netpolicy.Policy
	mu          sync.Mutex
	providers   map[string]ProviderEndpointConfig
	credentials credentials.Provider
	streams     map[string]*providerStream
	cancelled   map[string]time.Time
	emit        func(string, any)
}

func newProviderFetchService(policy *netpolicy.Policy) *ProviderFetchService {
	return &ProviderFetchService{policy: policy, providers: map[string]ProviderEndpointConfig{}, streams: map[string]*providerStream{}, cancelled: map[string]time.Time{}}
}

// ProviderFetchRequest mirrors the aiFetch IPC payload.
type ProviderFetchRequest struct {
	URL              string
	Method           string
	Headers          map[string]string
	Body             string
	ProviderID       string
	SkipHostCheck    bool
	FollowRedirects  bool
	SkipTLSVerify    bool
	credentialOrigin string
}

// ProviderFetchResult mirrors the aiFetch IPC response shape.
type ProviderFetchResult struct {
	OK     bool
	Status int
	Data   string
	Error  string
}

// ProviderAllowlistResult reports whether an allowlist mutation was applied.
type ProviderAllowlistResult struct {
	OK    bool
	Error string
}

// ProviderEndpointConfig is the allowlist-relevant part of one provider
// config synced from the renderer.
type ProviderEndpointConfig struct {
	ID            string
	ProviderID    string
	BaseURL       string
	Enabled       bool
	APIKey        string
	SkipTLSVerify bool
	CustomHeaders map[string]string
}

// providerFetchTimeout matches the 30s request timeout of the Electron path.
const providerFetchTimeout = 30 * time.Second

func (s *ProviderFetchService) client(request ProviderFetchRequest) *http.Client {
	client := s.policy.NewClient(netpolicy.ClientOptions{
		FetchOptions:  netpolicy.FetchOptions{AllowCustomEndpoints: request.SkipHostCheck},
		SkipTLSVerify: request.SkipTLSVerify,
		// The policy client revalidates every redirect hop (netpolicy
		// MaxRedirects bound); without followRedirects Electron's MAX_REDIRECTS
		// is 0, so the 3xx response itself is returned instead of an error.
	})
	if !request.FollowRedirects {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	} else if request.credentialOrigin != "" {
		check := client.CheckRedirect
		client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
			if strings.ToLower(next.URL.Scheme+"://"+next.URL.Host) != request.credentialOrigin {
				return netpolicy.ErrRedirectDenied
			}
			return check(next, via)
		}
	}
	return client
}

// Fetch performs one policy-enforced provider request.
func (s *ProviderFetchService) Fetch(request ProviderFetchRequest) ProviderFetchResult {
	var authErr error
	request, authErr = s.authorizeProviderRequest(request)
	if authErr != nil {
		return ProviderFetchResult{Error: authErr.Error()}
	}
	url := strings.TrimSpace(request.URL)
	if url == "" {
		return ProviderFetchResult{Error: "Invalid URL"}
	}
	method := strings.ToUpper(strings.TrimSpace(request.Method))
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if request.Body != "" {
		body = bytes.NewReader([]byte(request.Body))
	}
	ctx, cancel := context.WithTimeout(context.Background(), providerFetchTimeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return ProviderFetchResult{Error: "Invalid URL"}
	}
	for name, value := range request.Headers {
		httpRequest.Header.Set(name, value)
	}
	response, err := s.client(request).Do(httpRequest)
	if err != nil {
		return ProviderFetchResult{Error: providerFetchError(err)}
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		return ProviderFetchResult{
			Status: response.StatusCode,
			Error:  providerFetchError(readErr),
		}
	}
	return ProviderFetchResult{
		OK:     response.StatusCode >= 200 && response.StatusCode < 300,
		Status: response.StatusCode,
		Data:   string(data),
	}
}

// AllowlistAddHost registers one provider baseURL so discovery can reach it,
// exactly like netcatty:ai:allowlist:add-host. The registration lives for the
// process; the netpolicy guard (private ranges, metadata hosts, scheme rules)
// still decides every request.
func (s *ProviderFetchService) AllowlistAddHost(baseURL string) ProviderAllowlistResult {
	if strings.TrimSpace(baseURL) == "" {
		return ProviderAllowlistResult{Error: "baseURL must be a string"}
	}
	s.policy.AddProviderEndpoint(baseURL)
	return ProviderAllowlistResult{OK: true}
}

// SyncProviders retains configured encrypted keys in host memory and registers
// their endpoints. Plaintext keys are resolved only when sending a request.
func (s *ProviderFetchService) SyncProviders(providers []ProviderEndpointConfig) ProviderAllowlistResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers = map[string]ProviderEndpointConfig{}
	for _, provider := range providers {
		if !provider.Enabled {
			continue
		}
		s.providers[provider.ID] = provider
		if strings.TrimSpace(provider.BaseURL) == "" {
			continue
		}
		s.policy.AddProviderEndpoint(provider.BaseURL)
	}
	return ProviderAllowlistResult{OK: true}
}

// providerFetchError maps the policy and transport sentinels onto the message
// vocabulary the renderer already shows for Electron failures.
func providerFetchError(err error) string {
	var requestError *url.Error
	if errors.As(err, &requestError) {
		return providerFetchError(requestError.Err)
	}
	switch {
	case errors.Is(err, netpolicy.ErrURLDenied):
		return "URL host is not in the allowed list"
	case errors.Is(err, netpolicy.ErrRedirectDenied):
		return "Redirect target is not allowed"
	case errors.Is(err, netpolicy.ErrDialDenied):
		return "URL host is blocked by network policy"
	case errors.Is(err, netpolicy.ErrBodyTooLarge):
		return "Response body exceeded maximum size (10MB)"
	case errors.Is(err, context.DeadlineExceeded):
		return "Request timeout"
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return "Request timeout"
	}
	return fmt.Sprintf("Request failed: %v", err)
}
