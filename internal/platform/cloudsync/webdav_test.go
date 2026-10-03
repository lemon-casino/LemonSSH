package cloudsync

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeDAV is a minimal WebDAV-ish server: strong ETags, If-Match enforcement
// and 404 before the first PUT — enough to prove the client contract.
type fakeDAV struct {
	mu          sync.Mutex
	body        []byte
	etag        string
	lastAuth    string
	lastIfMatch string
}

func (f *fakeDAV) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.lastAuth = r.Header.Get("Authorization")
		f.lastIfMatch = r.Header.Get("If-Match")
		w.Header().Set("ETag", f.etag)
		switch r.Method {
		case http.MethodGet:
			if f.body == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(f.body)
		case http.MethodPut:
			if f.lastIfMatch != "" && f.lastIfMatch != f.etag {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			if f.body != nil && f.lastIfMatch == "" {
				// Overwrite without precondition on an existing snapshot is a
				// conflict by contract: callers must always pass an ETag
				// once a snapshot exists.
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			buf := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(buf)
			f.body = buf
			f.etag = strconv.Quote("e" + strconv.Itoa(len(buf)))
			w.Header().Set("ETag", f.etag)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func TestWebDAVRoundTripAndConflict(t *testing.T) {
	fake := &fakeDAV{etag: "\"0\""}
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	client := NewWebDAVClient(WebDAVConfig{
		Endpoint:      server.URL,
		Username:      "lemon",
		Password:      "secret",
		AllowInsecure: !strings.HasPrefix(server.URL, "https"),
	})

	// First pull: no snapshot yet.
	if _, _, err := client.Snapshot(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// First put: no expectation, snapshot created.
	etag, err := client.PutSnapshot(context.Background(), []byte("v1"), "")
	if err != nil {
		t.Fatal(err)
	}
	if etag == "" {
		t.Fatal("put must return an ETag")
	}

	// Stale writer (wrong ETag) conflicts instead of clobbering.
	if _, err := client.PutSnapshot(context.Background(), []byte("stale"), "\"bogus\""); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}

	// Fresh writer succeeds.
	if _, err := client.PutSnapshot(context.Background(), []byte("v2"), etag); err != nil {
		t.Fatal(err)
	}
	body, etag2, err := client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "v2" || etag2 == "" {
		t.Fatalf("body %q etag %q", body, etag2)
	}
}

func TestWebDAVUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	client := NewWebDAVClient(WebDAVConfig{Endpoint: server.URL})
	if _, _, err := client.Snapshot(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestWebDAVSendsBasicAuth(t *testing.T) {
	fake := &fakeDAV{etag: strconv.Quote("e0")}
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	client := NewWebDAVClient(WebDAVConfig{Endpoint: server.URL, Username: "lemon", Password: "secret"})
	if _, err := client.PutSnapshot(context.Background(), []byte("x"), ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fake.lastAuth, "Basic ") {
		t.Fatal("basic auth header missing")
	}
}

// digestServer implements one digest challenge/response cycle: requests
// without a verifiable Digest Authorization header get a 401 challenge, valid
// ones succeed. challenges counts served 401s so tests can prove the client
// caches the challenge instead of re-paying the round trip.
type digestServer struct {
	mu         sync.Mutex
	challenges int
	algorithm  string
	authHeader []string
}

func (s *digestServer) handler(realm, nonce, opaque string, username, password string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		valid := false
		if strings.HasPrefix(auth, "Digest ") {
			s.mu.Lock()
			s.authHeader = append(s.authHeader, auth)
			s.mu.Unlock()
			params := parseAuthParams(auth)
			algorithm := params["algorithm"]
			if algorithm == "" {
				algorithm = "MD5"
			}
			want := digestResponse(algorithm, username, password, realm, r.Method, r.URL.RequestURI(), nonce, params["nc"], params["cnonce"], params["qop"])
			valid = params["username"] == username && params["response"] == want
		}
		if !valid {
			s.mu.Lock()
			s.challenges++
			s.mu.Unlock()
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(
				`Digest realm=%q, nonce=%q, qop="auth", opaque=%q, algorithm=%s`,
				realm, nonce, opaque, s.algorithm))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

func (s *digestServer) challengeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.challenges
}

// TestWebDAVDigestAuth proves the client answers one 401 challenge with a
// server-verifiable RFC 7616 response and then authorizes proactively: the
// second operation must not trigger another 401.
func TestWebDAVDigestAuth(t *testing.T) {
	const (
		digestUser = "lemon"
		digestPass = "secret"
		realm      = "dav@example.com"
		nonce      = "nonce-abc123"
		opaque     = "opaque-xyz"
	)
	for _, algorithm := range []string{"MD5", "SHA-256"} {
		t.Run(algorithm, func(t *testing.T) {
			serverState := &digestServer{algorithm: algorithm}
			server := httptest.NewServer(serverState.handler(realm, nonce, opaque, digestUser, digestPass))
			defer server.Close()

			client := NewWebDAVClient(WebDAVConfig{
				Endpoint: server.URL,
				AuthType: "digest",
				Username: digestUser,
				Password: digestPass,
			})
			ctx := context.Background()

			// First operation pays the challenge and retries.
			if _, err := client.PutSnapshot(ctx, []byte("v1"), ""); err != nil {
				t.Fatalf("first put: %v", err)
			}
			// Second operation reuses the cached challenge.
			if _, err := client.PutSnapshot(ctx, []byte("v2"), ""); err != nil {
				t.Fatalf("second put: %v", err)
			}
			if _, _, err := client.Snapshot(ctx); err != nil {
				t.Fatalf("get: %v", err)
			}

			if got := serverState.challengeCount(); got != 1 {
				t.Fatalf("challenges = %d, want 1 (challenge must be cached)", got)
			}
			header := serverState.authHeader[0]
			if !strings.Contains(header, `qop=auth`) || !strings.Contains(header, "nc=00000001") {
				t.Fatalf("first authorization = %q", header)
			}
			if !strings.Contains(header, `opaque="`+opaque+`"`) {
				t.Fatalf("first authorization = %q", header)
			}
		})
	}
}

// TestWebDAVSendsBearerToken: token configs authorize with Bearer, not basic.
func TestWebDAVSendsBearerToken(t *testing.T) {
	var mu sync.Mutex
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewWebDAVClient(WebDAVConfig{Endpoint: server.URL, AuthType: "token", Token: "tok-123"})
	if _, err := client.PutSnapshot(context.Background(), []byte("x"), ""); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if got != "Bearer tok-123" {
		t.Fatalf("authorization = %q, want Bearer", got)
	}
}

// TestWebDAVAllowInsecureSkipsTLSVerify: self-signed endpoints fail closed by
// default and succeed with AllowInsecure.
func TestWebDAVAllowInsecureSkipsTLSVerify(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	strict := NewWebDAVClient(WebDAVConfig{Endpoint: server.URL})
	if _, _, err := strict.Snapshot(context.Background()); err == nil {
		t.Fatal("expected certificate verification to fail without allowInsecure")
	}

	lenient := NewWebDAVClient(WebDAVConfig{Endpoint: server.URL, AllowInsecure: true})
	if _, _, err := lenient.Snapshot(context.Background()); err != nil {
		t.Fatalf("expected insecure client to succeed, got %v", err)
	}
}

// TestWebDAVDefaultSnapshotNameMatchesRenderer pins the remote file name to
// the renderer's SYNC_CONSTANTS.SYNC_FILE_NAME so switching between the Go
// transport and the renderer fallback keeps addressing the same snapshot.
func TestWebDAVDefaultSnapshotNameMatchesRenderer(t *testing.T) {
	client := NewWebDAVClient(WebDAVConfig{Endpoint: "https://dav.example.com/dav/"})
	if got := client.snapshotURL(); got != "https://dav.example.com/dav/lemonssh-vault.json" {
		t.Fatalf("snapshot URL = %q, want .../lemonssh-vault.json", got)
	}
	if got := client.snapshotURLByName(LegacyWebDAVSnapshotName); got != "https://dav.example.com/dav/netcatty-vault.json" {
		t.Fatalf("legacy snapshot URL = %q, want .../netcatty-vault.json", got)
	}
}
