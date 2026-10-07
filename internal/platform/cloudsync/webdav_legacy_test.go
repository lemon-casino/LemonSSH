package cloudsync

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// davFile is one stored snapshot with a strong ETag.
type davFile struct {
	body []byte
	etag string
}

// fakeDAVTree is a path-addressed WebDAV-ish server: GET/HEAD/PUT/DELETE per
// file name, strong ETags and If-Match enforcement (a precondition against a
// missing file fails with 412 like many servers do).
type fakeDAVTree struct {
	mu      sync.Mutex
	files   map[string]davFile
	nextID  int
	deleted []string
	puts    []string
}

func newFakeDAVTree() *fakeDAVTree {
	return &fakeDAVTree{files: make(map[string]davFile)}
}

func (f *fakeDAVTree) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		name := strings.TrimPrefix(r.URL.Path, "/")
		existing, ok := f.files[name]
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("ETag", existing.etag)
			w.WriteHeader(http.StatusOK)
			if r.Method == http.MethodGet {
				_, _ = w.Write(existing.body)
			}
		case http.MethodPut:
			if r.Header.Get("If-Match") != "" && (!ok || r.Header.Get("If-Match") != existing.etag) {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			body, _ := io.ReadAll(r.Body)
			f.nextID++
			etag := strconv.Quote("e" + strconv.Itoa(f.nextID))
			f.files[name] = davFile{body: body, etag: etag}
			f.puts = append(f.puts, name)
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			f.deleted = append(f.deleted, name)
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			delete(f.files, name)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func (f *fakeDAVTree) put(name string, body string, etag string) {
	f.files[name] = davFile{body: []byte(body), etag: etag}
}

// TestWebDAVLegacySnapshotFallback proves a pre-rename snapshot stays
// discoverable and keeps being updated in place: reads fall back to the
// legacy name and uploads write back to the name that actually exists, so no
// fork under the new name is created.
func TestWebDAVLegacySnapshotFallback(t *testing.T) {
	fake := newFakeDAVTree()
	fake.put(LegacyWebDAVSnapshotName, "legacy-body", `"legacy-etag"`)
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	client := NewWebDAVClient(WebDAVConfig{Endpoint: server.URL})
	ctx := context.Background()

	body, etag, err := client.Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if string(body) != "legacy-body" || etag != `"legacy-etag"` {
		t.Fatalf("legacy snapshot must be readable, got %q/%q", body, etag)
	}

	// An upload with the observed ETag must target the legacy name and
	// succeed (If-Match stays paired with that object).
	updated, err := client.PutSnapshot(ctx, []byte("updated-body"), `"legacy-etag"`)
	if err != nil {
		t.Fatalf("put onto legacy snapshot must not conflict: %v", err)
	}
	if _, ok := fake.files[DefaultWebDAVSnapshotName]; ok {
		t.Fatal("upload must not fork the snapshot under the new name")
	}
	if got := fake.files[LegacyWebDAVSnapshotName].body; string(got) != "updated-body" {
		t.Fatalf("legacy snapshot must be updated in place, got %q", got)
	}
	if updated == "" {
		t.Fatal("put must return an ETag")
	}

	// A brand-new snapshot (neither name exists) is created under the new
	// name without a precondition.
	empty := newFakeDAVTree()
	emptyServer := httptest.NewServer(empty.handler())
	defer emptyServer.Close()
	emptyClient := NewWebDAVClient(WebDAVConfig{Endpoint: emptyServer.URL})
	if _, err := emptyClient.PutSnapshot(ctx, []byte("fresh"), ""); err != nil {
		t.Fatalf("fresh put: %v", err)
	}
	if _, ok := empty.files[DefaultWebDAVSnapshotName]; !ok {
		t.Fatal("fresh snapshot must be created under the new name")
	}
	if _, ok := empty.files[LegacyWebDAVSnapshotName]; ok {
		t.Fatal("fresh snapshot must not be created under the legacy name")
	}
}

// TestWebDAVDeleteRemovesBothNames: deleting the remote snapshot is an
// explicit user intent, so both names are removed and a fallback cannot
// resurrect the snapshot.
func TestWebDAVDeleteRemovesBothNames(t *testing.T) {
	fake := newFakeDAVTree()
	fake.put(DefaultWebDAVSnapshotName, "new", `"e-new"`)
	fake.put(LegacyWebDAVSnapshotName, "legacy", `"e-legacy"`)
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	client := NewWebDAVClient(WebDAVConfig{Endpoint: server.URL})

	if err := client.DeleteSnapshot(context.Background()); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(fake.files) != 0 {
		t.Fatalf("both names must be removed, left: %v", fake.files)
	}
	if _, _, err := client.Snapshot(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("snapshot must be gone after delete, got %v", err)
	}
}

// TestWebDAVExplicitSnapshotNameNotDual: a user-configured snapshot name is
// addressed exactly as configured — no legacy-name fallback.
func TestWebDAVExplicitSnapshotNameNotDual(t *testing.T) {
	fake := newFakeDAVTree()
	fake.put(LegacyWebDAVSnapshotName, "legacy", `"e-legacy"`)
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	client := NewWebDAVClient(WebDAVConfig{Endpoint: server.URL, SnapshotName: "custom.json"})

	if _, _, err := client.Snapshot(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("explicit name must not fall back to the legacy name, got %v", err)
	}
	if err := client.DeleteSnapshot(context.Background()); err != nil {
		t.Fatalf("delete: %v", err)
	}
	for _, deleted := range fake.deleted {
		if deleted == LegacyWebDAVSnapshotName {
			t.Fatal("explicit-name client must not touch the legacy name")
		}
	}
}
