// Package netpolicy is the Go authority for provider network access
// (W08, P7-03, AI-03): endpoint allowlists, SSRF guards, redirect
// revalidation and body limits. Provider SDK transports must route through
// this package's Transport so the policy that decides "may we call this
// URL" is the same code that performs the dial.
package netpolicy

import (
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// BuiltinFetchHosts are the provider hosts allowed without configuration,
// matching BUILTIN_FETCH_HOSTS in providerHandlers.cjs.
var BuiltinFetchHosts = map[string]bool{
	"api.openai.com":                    true,
	"api.anthropic.com":                 true,
	"generativelanguage.googleapis.com": true,
	"openrouter.ai":                     true,
	"api.tavily.com":                    true,
	"api.exa.ai":                        true,
	"api.bochaai.com":                   true,
	"open.bigmodel.cn":                  true,
}

// BuiltinLocalhostPorts are the loopback ports allowed without
// configuration (Ollama, LM Studio, common dev ports) — Issue #9 SSRF guard.
var BuiltinLocalhostPorts = map[int]bool{
	11434: true,
	1234:  true,
	3000:  true,
	3001:  true,
	5000:  true,
	5001:  true,
	8000:  true,
	8080:  true,
	8888:  true,
}

// MetadataHostnames are cloud instance-metadata endpoints, always blocked.
var MetadataHostnames = map[string]bool{
	"metadata.google.internal": true,
}

// FetchOptions mirrors the two fetch modes of providerHandlers.cjs.
type FetchOptions struct {
	// AllowCustomEndpoints corresponds to skipHostCheck: the user
	// explicitly configured this endpoint, so arbitrary public hosts are
	// allowed — private/internal hosts still never are, and HTTPS is
	// mandatory.
	AllowCustomEndpoints bool
}

// Policy holds the dynamic endpoint sets rebuilt from provider configs.
type Policy struct {
	providerHosts     map[string]bool
	providerHTTPHosts map[string]bool
	localPorts        map[int]bool
	webSearchHost     string
}

// New returns a policy seeded with the builtin hosts and ports.
func New() *Policy {
	return &Policy{
		providerHosts:     map[string]bool{},
		providerHTTPHosts: map[string]bool{},
		localPorts:        map[int]bool{},
	}
}

// AddProviderEndpoint registers a configured provider baseURL: localhost
// endpoints extend the allowed local ports (restricted authorization);
// remote hosts join the allowlist, tracking explicit http:// separately.
func (p *Policy) AddProviderEndpoint(rawURL string) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	host := parsed.Hostname()
	if host == "" {
		return
	}
	port := 0
	if parsed.Port() != "" {
		port, _ = strconv.Atoi(parsed.Port())
	} else if parsed.Scheme == "https" {
		port = 443
	} else {
		port = 80
	}

	if host == "localhost" || host == "127.0.0.1" {
		if port > 0 {
			p.localPorts[port] = true
		}
		return
	}
	p.providerHosts[host] = true
	if parsed.Scheme == "http" {
		p.providerHTTPHosts[host] = true
	}
}

// SetWebSearchHost records the configured web search API host, which may
// use plain http like explicit provider http endpoints.
func (p *Policy) SetWebSearchHost(rawURL string) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	if parsed.Hostname() != "" {
		p.webSearchHost = parsed.Hostname()
	}
}

// IsPrivateIP reports whether the address is loopback, private, link-local,
// CGNAT or unspecified — the SSRF guard set. IPv4-mapped IPv6 is resolved
// before the check.
func IsPrivateIP(ip netip.Addr) bool {
	if !ip.IsValid() {
		return true // unparsable addresses fail closed
	}
	if ip.Is4In6() {
		ip = ip.Unmap()
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	// CGNAT 100.64.0.0/10 (Tailscale etc.), which Go's IsPrivate misses.
	if ip.Is4() {
		b := ip.As4()
		return b[0] == 100 && b[1] >= 64 && b[1] <= 127
	}
	return false
}

// isPrivateHost extends IsPrivateIP with the literal hostnames the CJS
// guard blocks.
func isPrivateHost(hostname string) bool {
	if hostname == "localhost" {
		return true
	}
	if MetadataHostnames[hostname] {
		return true
	}
	ip, err := netip.ParseAddr(strings.Trim(hostname, "[]"))
	if err != nil {
		return false // a hostname, not an IP: allowlist decides
	}
	return IsPrivateIP(ip)
}

// IsAllowedURL decides whether one URL may be fetched under the policy.
// The decision sequence mirrors isAllowedFetchUrl: custom-endpoint mode
// still refuses private hosts and mandates HTTPS; the normal path allows
// loopback only on allowlisted ports, and remote hosts only on the
// builtin/configured allowlist with scheme restrictions for plain http.
func (p *Policy) IsAllowedURL(rawURL string, opts FetchOptions) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return p.isAllowedParsedURL(parsed, opts)
}

func (p *Policy) isAllowedParsedURL(parsed *url.URL, opts FetchOptions) bool {
	hostname := parsed.Hostname()
	if hostname == "" {
		return false
	}

	if opts.AllowCustomEndpoints {
		if isPrivateHost(hostname) {
			return false
		}
		return parsed.Scheme == "https"
	}

	if hostname == "localhost" || hostname == "127.0.0.1" {
		port := 80
		if parsed.Port() != "" {
			port, _ = strconv.Atoi(parsed.Port())
		} else if parsed.Scheme == "https" {
			port = 443
		}
		return p.isLocalPortAllowed(port)
	}

	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return false
	}
	if parsed.Scheme == "http" {
		if !p.providerHTTPHosts[hostname] && p.webSearchHost != hostname {
			return false
		}
	}
	return BuiltinFetchHosts[hostname] || p.providerHosts[hostname]
}

func (p *Policy) isLocalPortAllowed(port int) bool {
	if BuiltinLocalhostPorts[port] {
		return true
	}
	return p.localPorts[port]
}
