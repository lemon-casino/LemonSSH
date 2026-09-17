package netpolicy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// Limits matching providerHandlers.cjs.
const (
	MaxResponseBodyBytes = 10 << 20 // 10 MiB safety limit
	MaxRedirects         = 5
)

// Classification is the per-URL decision the transport carries into the
// dial phase so resolved IPs are judged in the same context as the URL.
type Classification int

const (
	ClassificationRemote Classification = iota
	ClassificationLocalAllowed
)

var (
	ErrURLDenied      = errors.New("netpolicy: URL is not allowed")
	ErrRedirectDenied = errors.New("netpolicy: redirect target is not allowed")
	ErrDialDenied     = errors.New("netpolicy: dial address is not allowed")
	ErrBodyTooLarge   = errors.New("netpolicy: response body exceeded maximum size")
)

// ClientOptions configure one enforcing http.Client.
type ClientOptions struct {
	// FetchOptions apply to every request and redirect hop.
	FetchOptions
	// SkipTLSVerify mirrors the explicit skipTLS provider option; it never
	// bypasses the host policy.
	SkipTLSVerify bool
	// MaxRedirects defaults to MaxRedirects; a negative value disables
	// following.
	MaxRedirects int
	// MaxResponseBodyBytes defaults to MaxResponseBodyBytes.
	MaxResponseBodyBytes int64
	// Resolver overrides DNS resolution (tests).
	Resolver func(ctx context.Context, host string) ([]netip.Addr, error)
}

func (o ClientOptions) withDefaults() ClientOptions {
	if o.MaxRedirects == 0 {
		o.MaxRedirects = MaxRedirects
	}
	if o.MaxResponseBodyBytes == 0 {
		o.MaxResponseBodyBytes = MaxResponseBodyBytes
	}
	return o
}

// NewClient builds an http.Client whose URLs, dials, redirects and bodies
// are all constrained by the policy. The DNS-to-dial guarantee holds
// because the dialer validates every resolved address and connects only to
// an address it validated (T48: a public hostname that starts resolving to
// loopback or metadata space is refused).
func (p *Policy) NewClient(opts ClientOptions) *http.Client {
	opts = opts.withDefaults()

	dialer := &net.Dialer{Timeout: 30 * time.Second}
	base := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrDialDenied, err)
			}
			ip, err := p.resolveAndValidate(ctx, opts, host, localAllowedFrom(ctx))
			if err != nil {
				return nil, err
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		},
	}
	if opts.SkipTLSVerify {
		base.TLSClientConfig = skipTLSConfig(base.TLSClientConfig)
	}

	client := &http.Client{
		Transport: &enforcingTransport{policy: p, base: base, opts: opts},
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > opts.MaxRedirects {
			return fmt.Errorf("netpolicy: stopped after %d redirects", opts.MaxRedirects)
		}
		if !p.IsAllowedURL(req.URL.String(), opts.FetchOptions) {
			return ErrRedirectDenied
		}
		return nil
	}
	return client
}

// enforcingTransport validates each request URL and carries the
// classification into the dial phase.
type enforcingTransport struct {
	policy *Policy
	base   http.RoundTripper
	opts   ClientOptions
}

func (e *enforcingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	localAllowed := e.policy.isLocalAllowedTarget(req.URL, e.opts.FetchOptions)
	if _, allowed := e.policy.classify(req.URL, e.opts.FetchOptions); !allowed {
		return nil, ErrURLDenied
	}
	resp, err := e.base.RoundTrip(req.WithContext(withLocalAllowed(req.Context(), localAllowed)))
	if err != nil {
		return nil, err
	}
	resp.Body = &limitedBody{ReadCloser: resp.Body, remaining: e.opts.MaxResponseBodyBytes}
	return resp, nil
}

type limitedBody struct {
	io.ReadCloser
	remaining int64
}

func (b *limitedBody) Read(p []byte) (int, error) {
	if b.remaining < 0 {
		return 0, ErrBodyTooLarge
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	if b.remaining < 0 {
		return n, ErrBodyTooLarge
	}
	return n, err
}

// classify exposes the URL decision plus whether loopback addresses may be
// dialed for it.
func (p *Policy) classify(u *url.URL, opts FetchOptions) (Classification, bool) {
	if !p.isAllowedParsedURL(u, opts) {
		return ClassificationRemote, false
	}
	if u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" {
		return ClassificationLocalAllowed, true
	}
	return ClassificationRemote, true
}

func (p *Policy) isLocalAllowedTarget(u *url.URL, opts FetchOptions) bool {
	classification, allowed := p.classify(u, opts)
	return allowed && classification == ClassificationLocalAllowed
}

// resolveAndValidate resolves the host and returns exactly one validated
// address. Any private resolution on a non-local-allowed target is refused.
func (p *Policy) resolveAndValidate(ctx context.Context, opts ClientOptions, host string, localAllowed bool) (netip.Addr, error) {
	if ip, err := netip.ParseAddr(strings.ToLower(host)); err == nil {
		if IsPrivateIP(ip) && !localAllowed {
			return netip.Addr{}, ErrDialDenied
		}
		return ip, nil
	}

	var (
		ips []netip.Addr
		err error
	)
	if opts.Resolver != nil {
		ips, err = opts.Resolver(ctx, host)
	} else {
		ips, err = net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	}
	if err != nil {
		return netip.Addr{}, fmt.Errorf("%w: lookup %q: %v", ErrDialDenied, host, err)
	}
	for _, ip := range ips {
		unmapped := ip
		if unmapped.Is4In6() {
			unmapped = unmapped.Unmap()
		}
		if IsPrivateIP(unmapped) && !localAllowed {
			return netip.Addr{}, ErrDialDenied
		}
	}
	for _, ip := range ips {
		if !IsPrivateIP(ip) || localAllowed {
			return ip, nil
		}
	}
	return netip.Addr{}, ErrDialDenied
}

type localAllowedKey struct{}

func withLocalAllowed(ctx context.Context, allowed bool) context.Context {
	return context.WithValue(ctx, localAllowedKey{}, allowed)
}

func localAllowedFrom(ctx context.Context) bool {
	allowed, _ := ctx.Value(localAllowedKey{}).(bool)
	return allowed
}

func skipTLSConfig(cfg *tls.Config) *tls.Config {
	clone := cfg.Clone()
	if clone == nil {
		clone = &tls.Config{}
	}
	clone.InsecureSkipVerify = true
	return clone
}
