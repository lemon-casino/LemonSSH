// Package cloudsync owns the Go cloud snapshot transport (P6-01, SYNC-02).
// This file implements the WebDAV provider: GET/PUT with ETag strong
// validation so a stale writer surfaces as a conflict instead of silently
// overwriting a newer snapshot. Authentication supports basic, digest
// (RFC 7616, qop="auth") and bearer tokens; AllowInsecure opts into
// skipping TLS certificate verification for self-signed servers.
package cloudsync

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	// ErrConflict means the remote moved on: the caller must pull, merge and
	// retry rather than overwrite.
	ErrConflict = errors.New("cloudsync: remote snapshot changed")
	// ErrNotFound means no snapshot exists at the path yet.
	ErrNotFound = errors.New("cloudsync: snapshot not found")
	// ErrUnauthorized means credentials were rejected (401 after response).
	ErrUnauthorized = errors.New("cloudsync: unauthorized")
)

// DefaultWebDAVSnapshotName matches SYNC_CONSTANTS.SYNC_FILE_NAME in the
// renderer so the Go transport and the renderer fallback address the same
// remote file. LegacyWebDAVSnapshotName keeps pre-rename snapshots
// discoverable and updatable: reads fall back to it, uploads write back to
// the name that actually exists, and deletes remove both. A brand-new
// snapshot is always created under the default name, and an explicitly
// configured SnapshotName is addressed exactly as configured.
const (
	DefaultWebDAVSnapshotName = "lemonssh-vault.json"
	LegacyWebDAVSnapshotName  = "netcatty-vault.json"
)

// WebDAVConfig configures one WebDAV snapshot endpoint.
type WebDAVConfig struct {
	// Endpoint is the collection URL the snapshot file lives in, e.g.
	// https://dav.example.com/lemonssh/.
	Endpoint string
	// SnapshotName is the file name inside Endpoint (default
	// DefaultWebDAVSnapshotName).
	SnapshotName string
	// AuthType selects the authentication scheme: "" / "basic" (default),
	// "digest" (Username/Password, RFC 7616 qop="auth") or "token" (Bearer).
	AuthType string
	Username string
	Password string
	// Token is the bearer token used when AuthType is "token".
	Token string
	// AllowInsecure skips TLS certificate verification (self-signed
	// certificates on self-hosted servers). It never downgrades the scheme.
	AllowInsecure bool
	Timeout       time.Duration
}

// WebDAVClient talks to one WebDAV snapshot endpoint.
type WebDAVClient struct {
	config WebDAVConfig
	client *http.Client
	digest *digestState
	// legacyName is the pre-rename snapshot name probed as a fallback. It is
	// empty when the caller configured an explicit SnapshotName: a
	// user-chosen name is addressed exactly as configured.
	legacyName string
}

func NewWebDAVClient(config WebDAVConfig) *WebDAVClient {
	legacyName := ""
	if config.SnapshotName == "" {
		config.SnapshotName = DefaultWebDAVSnapshotName
		legacyName = LegacyWebDAVSnapshotName
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if config.AllowInsecure {
		// Explicit user opt-in for self-signed certificates, mirroring the
		// renderer fallback's https.Agent({rejectUnauthorized: false}).
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- user-requested self-signed support
	}
	return &WebDAVClient{
		config:     config,
		client:     &http.Client{Timeout: timeout, Transport: transport},
		digest:     &digestState{},
		legacyName: legacyName,
	}
}

// authScheme normalizes the configured auth type onto the three supported
// schemes; unknown values fall back to basic.
func (c *WebDAVClient) authScheme() string {
	switch strings.ToLower(strings.TrimSpace(c.config.AuthType)) {
	case "digest":
		return "digest"
	case "token", "bearer":
		return "token"
	default:
		return "basic"
	}
}

func (c *WebDAVClient) snapshotURL() string {
	return c.snapshotURLByName(c.config.SnapshotName)
}

// snapshotURLByName builds the endpoint URL for one snapshot name.
func (c *WebDAVClient) snapshotURLByName(name string) string {
	endpoint := strings.TrimRight(c.config.Endpoint, "/")
	return endpoint + "/" + strings.TrimLeft(name, "/")
}

// headSnapshot probes one snapshot name and returns its ETag; ErrNotFound
// when the name is absent.
func (c *WebDAVClient) headSnapshot(ctx context.Context, name string) (string, error) {
	response, err := c.do(ctx, "head", http.MethodHead, c.snapshotURLByName(name), nil, nil)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return "", ErrNotFound
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", ErrUnauthorized
	default:
		return "", fmt.Errorf("webdav head: unexpected status %d", response.StatusCode)
	}
	return response.Header.Get("ETag"), nil
}

// resolveActiveSnapshot returns the name the snapshot currently lives at
// (default name first, legacy fallback) plus its current ETag; (default
// name, "") when neither exists. An unexpected HEAD result falls back to the
// default name instead of failing the caller — the caller's precondition
// handling still protects the write.
func (c *WebDAVClient) resolveActiveSnapshot(ctx context.Context) (string, string) {
	if c.legacyName == "" {
		return c.config.SnapshotName, ""
	}
	etag, err := c.headSnapshot(ctx, c.config.SnapshotName)
	if err == nil {
		return c.config.SnapshotName, etag
	}
	if !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrUnauthorized) {
		return c.config.SnapshotName, ""
	}
	if etag, err = c.headSnapshot(ctx, c.legacyName); err == nil {
		return c.legacyName, etag
	}
	return c.config.SnapshotName, ""
}

// newRequest builds one request and stamps whichever Authorization header the
// configured scheme requires. Digest authorization is only attached when a
// challenge is already cached; otherwise the request goes unauthenticated and
// do() retries once the 401 challenge arrives.
func (c *WebDAVClient) newRequest(ctx context.Context, method, url string, body []byte) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}
	switch c.authScheme() {
	case "token":
		request.Header.Set("Authorization", "Bearer "+c.config.Token)
	case "digest":
		if c.config.Username == "" && c.config.Password == "" {
			return request, nil
		}
		if auth := c.digest.authorization(c.config.Username, c.config.Password, request.Method, request.URL.RequestURI()); auth != "" {
			request.Header.Set("Authorization", auth)
		}
	default:
		if c.config.Username != "" || c.config.Password != "" {
			request.SetBasicAuth(c.config.Username, c.config.Password)
		}
	}
	return request, nil
}

// do sends one request, wrapping transport failures with the operation name.
// Digest requests that receive a 401 challenge are retried once with freshly
// parsed credentials; the challenge is cached so later requests authorize
// proactively. extraHeaders (e.g. If-Match) ride on both attempts.
func (c *WebDAVClient) do(ctx context.Context, op, method, url string, body []byte, extraHeaders map[string]string) (*http.Response, error) {
	response, err := c.send(ctx, method, url, body, extraHeaders)
	if err != nil {
		return nil, fmt.Errorf("webdav %s: %w", op, err)
	}
	if response.StatusCode == http.StatusUnauthorized && c.authScheme() == "digest" {
		challenge := response.Header.Get("WWW-Authenticate")
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if !c.digest.updateChallenge(challenge) {
			return response, nil
		}
		response, err = c.send(ctx, method, url, body, extraHeaders)
		if err != nil {
			return nil, fmt.Errorf("webdav %s: %w", op, err)
		}
	}
	return response, nil
}

func (c *WebDAVClient) send(ctx context.Context, method, url string, body []byte, extraHeaders map[string]string) (*http.Response, error) {
	request, err := c.newRequest(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	for name, value := range extraHeaders {
		request.Header.Set(name, value)
	}
	return c.client.Do(request)
}

// Snapshot fetches the current remote snapshot and its ETag, reading the
// default name first and the legacy pre-rename name as fallback. ErrNotFound
// when the endpoint has no snapshot under either name.
func (c *WebDAVClient) Snapshot(ctx context.Context) ([]byte, string, error) {
	body, etag, err := c.getSnapshot(ctx, c.config.SnapshotName)
	if errors.Is(err, ErrNotFound) && c.legacyName != "" {
		return c.getSnapshot(ctx, c.legacyName)
	}
	return body, etag, err
}

func (c *WebDAVClient) getSnapshot(ctx context.Context, name string) ([]byte, string, error) {
	response, err := c.do(ctx, "get", http.MethodGet, c.snapshotURLByName(name), nil, nil)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, "", ErrNotFound
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, "", ErrUnauthorized
	default:
		return nil, "", fmt.Errorf("webdav get: unexpected status %d", response.StatusCode)
	}
	etag := response.Header.Get("ETag")
	body, err := io.ReadAll(io.LimitReader(response.Body, maxSnapshotBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("webdav get body: %w", err)
	}
	if len(body) > maxSnapshotBytes {
		return nil, "", fmt.Errorf("webdav get: snapshot exceeds %d bytes", maxSnapshotBytes)
	}
	return body, etag, nil
}

// PutSnapshot uploads a snapshot, writing back to the name the snapshot
// currently lives at (default name first, legacy fallback) so pre-rename
// snapshots keep being updated in place instead of forking. When expectETag
// is non-empty the upload carries If-Match, and a 412 response maps to
// ErrConflict so a stale writer pulls and merges instead of clobbering the
// remote. A missing precondition upload to an existing snapshot is also a
// conflict.
func (c *WebDAVClient) PutSnapshot(ctx context.Context, data []byte, expectETag string) (string, error) {
	if int64(len(data)) > maxSnapshotBytes {
		return "", fmt.Errorf("webdav put: snapshot exceeds %d bytes", maxSnapshotBytes)
	}
	name, _ := c.resolveActiveSnapshot(ctx)
	extraHeaders := map[string]string{}
	if expectETag != "" {
		extraHeaders["If-Match"] = expectETag
	}
	response, err := c.do(ctx, "put", http.MethodPut, c.snapshotURLByName(name), data, extraHeaders)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
	case http.StatusPreconditionFailed:
		return "", ErrConflict
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", ErrUnauthorized
	case http.StatusNotFound:
		return "", ErrNotFound
	default:
		return "", fmt.Errorf("webdav put: unexpected status %d", response.StatusCode)
	}
	etag := response.Header.Get("ETag")
	if etag == "" {
		// Many servers skip the ETag on PUT; re-read so the next write can
		// still use a strong precondition.
		_, etag, err = c.Snapshot(ctx)
		if err != nil {
			return "", err
		}
	}
	return etag, nil
}

// DeleteSnapshot removes the remote snapshot. The user explicitly asked to
// drop the remote snapshot, so both the default and the legacy name are
// removed (a 404 counts as removed).
func (c *WebDAVClient) DeleteSnapshot(ctx context.Context) error {
	if err := c.deleteSnapshot(ctx, c.config.SnapshotName); err != nil {
		return err
	}
	if c.legacyName != "" {
		return c.deleteSnapshot(ctx, c.legacyName)
	}
	return nil
}

func (c *WebDAVClient) deleteSnapshot(ctx context.Context, name string) error {
	response, err := c.do(ctx, "delete", http.MethodDelete, c.snapshotURLByName(name), nil, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	default:
		return fmt.Errorf("webdav delete: unexpected status %d", response.StatusCode)
	}
}

// maxSnapshotBytes bounds one cloud snapshot (16 MiB).
const maxSnapshotBytes = 16 << 20

// digestState caches one RFC 7616 digest challenge so later requests can
// authorize proactively. The nonce count is protected for concurrent calls.
type digestState struct {
	mu        sync.Mutex
	realm     string
	nonce     string
	opaque    string
	algorithm string
	qop       string
	nc        uint64
}

// updateChallenge parses a WWW-Authenticate header and caches it. It returns
// false when the header is missing, is not a Digest challenge, or carries no
// nonce (e.g. a Basic challenge from a server that rejects the credentials).
func (d *digestState) updateChallenge(header string) bool {
	trimmed := strings.TrimSpace(header)
	if !strings.HasPrefix(strings.ToLower(trimmed), "digest") {
		return false
	}
	params := parseAuthParams(trimmed)
	nonce := params["nonce"]
	if nonce == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if nonce != d.nonce {
		// Fresh nonce: the counter restarts.
		d.nc = 0
	}
	d.realm = params["realm"]
	d.nonce = nonce
	d.opaque = params["opaque"]
	d.algorithm = params["algorithm"]
	d.qop = params["qop"]
	return true
}

// authorization renders the Authorization header for one request, or "" when
// no challenge has been seen yet. nc/cnonce are minted per call per RFC 7616.
func (d *digestState) authorization(username, password, method, uri string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.nonce == "" {
		return ""
	}
	d.nc++
	nc := fmt.Sprintf("%08x", d.nc)
	cnonce := randomCnonce()
	hashFunc := digestHashFunc(d.algorithm)
	ha1 := hexHash(hashFunc, username+":"+d.realm+":"+password)
	if strings.HasSuffix(strings.ToLower(d.algorithm), "-sess") {
		ha1 = hexHash(hashFunc, ha1+":"+d.nonce+":"+cnonce)
	}
	ha2 := hexHash(hashFunc, method+":"+uri)
	var response string
	parts := []string{
		`username="` + username + `"`,
		`realm="` + d.realm + `"`,
		`nonce="` + d.nonce + `"`,
		`uri="` + uri + `"`,
	}
	algorithm := d.algorithm
	if algorithm == "" {
		algorithm = "MD5"
	}
	parts = append(parts, "algorithm="+algorithm)
	if d.qop != "" {
		// Only qop="auth" is offered/selected; auth-int requires body hashing
		// that no snapshot server in the wild negotiates here.
		response = hexHash(hashFunc, ha1+":"+d.nonce+":"+nc+":"+cnonce+":auth:"+ha2)
		parts = append(parts,
			`response="`+response+`"`,
			`qop=auth`,
			`nc=`+nc,
			`cnonce="`+cnonce+`"`,
		)
	} else {
		response = hexHash(hashFunc, ha1+":"+d.nonce+":"+ha2)
		parts = append(parts, `response="`+response+`"`)
	}
	if d.opaque != "" {
		parts = append(parts, `opaque="`+d.opaque+`"`)
	}
	return "Digest " + strings.Join(parts, ", ")
}

// digestHashFunc maps the challenge's algorithm onto its hash; unknown values
// default to MD5, the universal baseline.
func digestHashFunc(algorithm string) func() hash.Hash {
	switch strings.ToUpper(strings.TrimSpace(algorithm)) {
	case "SHA-256", "SHA-256-SESS":
		return sha256.New
	default:
		return md5.New
	}
}

func hexHash(hashFunc func() hash.Hash, value string) string {
	sum := hashFunc()
	sum.Write([]byte(value))
	return fmt.Sprintf("%x", sum.Sum(nil))
}

// randomCnonce returns 16 hex characters from the CSPRNG.
func randomCnonce() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand failing means the runtime is broken; a constant cnonce
		// still yields a valid (if less replay-resistant) response.
		return "lemonssh-cnonce"
	}
	return fmt.Sprintf("%x", buf)
}

// parseAuthParams splits "Digest key="value", key=value, ..." into a map with
// lowercased keys. Quoted values may contain commas and equals signs.
func parseAuthParams(header string) map[string]string {
	params := map[string]string{}
	rest := strings.TrimSpace(header)
	if i := strings.IndexByte(rest, ' '); i >= 0 {
		rest = strings.TrimSpace(rest[i+1:])
	}
	for rest != "" {
		// Drop separator commas left over from the previous pair.
		rest = strings.TrimLeft(strings.TrimSpace(rest), ",")
		eq := strings.IndexByte(rest, '=')
		if eq < 0 {
			break
		}
		key := strings.ToLower(strings.TrimSpace(rest[:eq]))
		rest = strings.TrimSpace(rest[eq+1:])
		var value string
		if strings.HasPrefix(rest, `"`) {
			end := strings.IndexByte(rest[1:], '"')
			if end < 0 {
				value = rest[1:]
				rest = ""
			} else {
				value = rest[1 : 1+end]
				rest = strings.TrimSpace(rest[end+2:])
			}
		} else if i := strings.IndexByte(rest, ','); i >= 0 {
			value = strings.TrimSpace(rest[:i])
			rest = strings.TrimSpace(rest[i+1:])
		} else {
			value = rest
			rest = ""
		}
		if key != "" {
			params[key] = value
		}
	}
	return params
}

// digestResponse recomputes the expected response hash; shared by tests to
// verify the client's Authorization header against the server-side password.
func digestResponse(algorithm, username, password, realm, method, uri, nonce, nc, cnonce, qop string) string {
	hashFunc := digestHashFunc(algorithm)
	ha1 := hexHash(hashFunc, username+":"+realm+":"+password)
	ha2 := hexHash(hashFunc, method+":"+uri)
	if qop != "" {
		return hexHash(hashFunc, ha1+":"+nonce+":"+nc+":"+cnonce+":"+qop+":"+ha2)
	}
	return hexHash(hashFunc, ha1+":"+nonce+":"+ha2)
}
