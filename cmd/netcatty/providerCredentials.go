package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const providerKeyPlaceholder = "__IPC_SECURED__"
const webSearchKeyPlaceholder = "__WEB_SEARCH_KEY__"

func (s *ProviderFetchService) authorizeProviderRequest(request ProviderFetchRequest) (ProviderFetchRequest, error) {
	headers := make(map[string]string, len(request.Headers))
	for name, value := range request.Headers {
		headers[http.CanonicalHeaderKey(name)] = value
	}
	request.Headers = headers
	var webSearchErr error
	request, webSearchErr = s.authorizeWebSearchRequest(request)
	if webSearchErr != nil {
		return request, webSearchErr
	}
	s.mu.Lock()
	provider, configured := s.providers[request.ProviderID]
	s.mu.Unlock()
	if configured {
		target, targetErr := url.Parse(request.URL)
		base, baseErr := url.Parse(provider.BaseURL)
		if targetErr != nil || baseErr != nil || base.Host == "" || !strings.EqualFold(target.Scheme, base.Scheme) || !strings.EqualFold(target.Host, base.Host) {
			return request, fmt.Errorf("AI provider request does not match its configured endpoint")
		}
		request.credentialOrigin = strings.ToLower(base.Scheme + "://" + base.Host)
		for name, value := range provider.CustomHeaders {
			request.Headers[http.CanonicalHeaderKey(name)] = value
		}
		request.SkipTLSVerify = request.SkipTLSVerify || provider.SkipTLSVerify
	}
	needsKey := strings.Contains(request.URL, providerKeyPlaceholder)
	for _, value := range request.Headers {
		needsKey = needsKey || strings.Contains(value, providerKeyPlaceholder)
	}
	if !needsKey {
		return request, nil
	}
	if !configured || provider.APIKey == "" {
		return request, fmt.Errorf("AI provider key is unavailable; check the provider configuration in Settings")
	}
	target, err := url.Parse(request.URL)
	if err != nil {
		return request, fmt.Errorf("Invalid provider URL")
	}
	base, err := url.Parse(provider.BaseURL)
	if err != nil || base.Host == "" || !strings.EqualFold(target.Scheme, base.Scheme) || !strings.EqualFold(target.Host, base.Host) {
		return request, fmt.Errorf("AI provider key can only be used with its configured endpoint")
	}
	key, err := s.openStoredAPIKey(provider.APIKey, "AI provider")
	if err != nil {
		return request, err
	}
	for name, value := range request.Headers {
		request.Headers[name] = strings.ReplaceAll(value, providerKeyPlaceholder, key)
	}
	query := target.Query()
	for name, values := range query {
		for i, value := range values {
			values[i] = strings.ReplaceAll(value, providerKeyPlaceholder, key)
		}
		query[name] = values
	}
	target.RawQuery = query.Encode()
	request.URL = target.String()
	return request, nil
}

func (s *ProviderFetchService) authorizeWebSearchRequest(request ProviderFetchRequest) (ProviderFetchRequest, error) {
	needsKey := strings.Contains(request.URL, webSearchKeyPlaceholder)
	for _, value := range request.Headers {
		needsKey = needsKey || strings.Contains(value, webSearchKeyPlaceholder)
	}
	if !needsKey {
		return request, nil
	}

	s.mu.Lock()
	config := s.webSearch
	s.mu.Unlock()
	if config.BaseURL == "" || config.APIKey == "" {
		return request, fmt.Errorf("Web search key is unavailable; check the web search configuration in Settings")
	}
	target, targetErr := url.Parse(request.URL)
	base, baseErr := url.Parse(config.BaseURL)
	if targetErr != nil || baseErr != nil || base.Host == "" || !strings.EqualFold(target.Scheme, base.Scheme) || !strings.EqualFold(target.Host, base.Host) {
		return request, fmt.Errorf("Web search key can only be used with its configured endpoint")
	}
	key, err := s.openStoredAPIKey(config.APIKey, "Web search")
	if err != nil {
		return request, err
	}
	request.credentialOrigin = strings.ToLower(base.Scheme + "://" + base.Host)
	for name, value := range request.Headers {
		request.Headers[name] = strings.ReplaceAll(value, webSearchKeyPlaceholder, key)
	}
	query := target.Query()
	for name, values := range query {
		for i, value := range values {
			values[i] = strings.ReplaceAll(value, webSearchKeyPlaceholder, key)
		}
		query[name] = values
	}
	target.RawQuery = query.Encode()
	request.URL = target.String()
	return request, nil
}

func (s *ProviderFetchService) openStoredAPIKey(value, label string) (string, error) {
	if !strings.HasPrefix(value, "enc:v1:") {
		return value, nil
	}
	if s.credentials == nil {
		return "", fmt.Errorf("Secure storage is unavailable for the %s key", label)
	}
	envelope, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "enc:v1:"))
	if err != nil {
		return "", fmt.Errorf("%s key could not be decrypted; save the key again in Settings", label)
	}
	plain, err := s.credentials.Open(envelope, "cloud-sync-credentials")
	if err != nil {
		return "", fmt.Errorf("%s key could not be decrypted; save the key again in Settings", label)
	}
	key := string(plain)
	for i := range plain {
		plain[i] = 0
	}
	return key, nil
}
