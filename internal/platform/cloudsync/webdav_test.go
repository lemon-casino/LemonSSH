package cloudsync

import (
	"context"
	"errors"
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
	mu       sync.Mutex
	body     []byte
	etag     string
	lastAuth string
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
