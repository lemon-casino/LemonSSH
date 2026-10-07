package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/lemon-casino/lemonssh/internal/platform/cloudsync"
)

// s3LegacyBlob is one stored object with a strong ETag.
type s3LegacyBlob struct {
	body []byte
	etag string
}

// fakeS3Legacy is a minimal S3-compatible endpoint for the active-key
// semantics tests: in-memory objects, strong ETags, If-Match enforcement
// (including 404 for a precondition against a missing key, like real S3).
type fakeS3Legacy struct {
	mu      sync.Mutex
	objects map[string]s3LegacyBlob
	nextID  int
}

func newFakeS3Legacy() *fakeS3Legacy {
	return &fakeS3Legacy{objects: make(map[string]s3LegacyBlob)}
}

func (f *fakeS3Legacy) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		existing, ok := f.objects[key]
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
			body, _ := io.ReadAll(r.Body)
			if r.Header.Get("If-Match") != "" && (!ok || r.Header.Get("If-Match") != existing.etag) {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			f.nextID++
			etag := strconv.Quote("e" + strconv.Itoa(f.nextID))
			f.objects[key] = s3LegacyBlob{body: body, etag: etag}
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			delete(f.objects, key)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func (f *fakeS3Legacy) put(key, body, etag string) {
	f.objects[key] = s3LegacyBlob{body: []byte(body), etag: etag}
}

func (f *fakeS3Legacy) s3Config(endpoint string) json.RawMessage {
	return json.RawMessage(`{"endpoint":"` + endpoint + `","region":"us-east-1","bucket":"vault","accessKeyId":"a","secretAccessKey":"s"}`)
}

func legacySyncedFile(etag string) CloudSyncSyncedFile {
	return CloudSyncSyncedFile{
		Meta:    json.RawMessage(`{"version":1}`),
		Payload: "bGVnYWN5LWNpcGhlcnRleHQ=",
		ETag:    etag,
	}
}

func mustKey(t *testing.T, prefix, name string) string {
	t.Helper()
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}

// TestCloudSyncS3LegacyOnlySnapshotSurvivesUpload proves the core rename
// hazard is fixed: with only a legacy-named object in the bucket, download
// serves it, and the follow-up upload writes back to that legacy key with a
// matching If-Match instead of creating a second object or failing 412.
func TestCloudSyncS3LegacyOnlySnapshotSurvivesUpload(t *testing.T) {
	fake := newFakeS3Legacy()
	legacyKey := mustKey(t, "", legacyS3SnapshotFileName)
	fake.put(legacyKey, `{"meta":{"version":1},"payload":"bGVnYWN5"}`, `"e1"`)
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	service := &SyncService{}
	config := fake.s3Config(server.URL)

	// Initialize resolves the resource id from the legacy object.
	resource, err := service.CloudSyncS3Initialize(config)
	if err != nil || resource.ResourceID == nil || *resource.ResourceID != `"e1"` {
		t.Fatalf("initialize must resolve the legacy object etag, got %v %v", resource, err)
	}

	// Download serves the legacy object.
	downloaded, err := service.CloudSyncS3Download(config)
	if err != nil || downloaded.SyncedFile == nil {
		t.Fatalf("download: %v %v", downloaded, err)
	}
	if downloaded.SyncedFile.ETag != `"e1"` || downloaded.SyncedFile.Payload != "bGVnYWN5" {
		t.Fatalf("legacy object must be served with its etag, got %+v", downloaded.SyncedFile)
	}

	// Upload writes back to the legacy key (If-Match paired with it) and must
	// not 412 or fork a new-name object.
	uploaded := downloaded.SyncedFile
	uploaded.Payload = "bmV3"
	result, err := service.CloudSyncS3Upload(config, *uploaded)
	if err != nil || result.ResourceID == nil {
		t.Fatalf("upload onto legacy object must succeed, got %v %v", result, err)
	}
	if _, ok := fake.objects[mustKey(t, "", s3SnapshotFileName)]; ok {
		t.Fatal("upload must not create a new-name object while the legacy object exists")
	}
	if got := string(fake.objects[legacyKey].body); !strings.Contains(got, `"payload":"bmV3"`) || !strings.Contains(got, `"etag":"\"e1\""`) {
		t.Fatalf("legacy object must be updated in place with paired etag, got %q", got)
	}
}

// TestCloudSyncS3FreshBucketCreatesNewKey: with neither object present, a
// brand-new snapshot is created under the new key without a precondition.
func TestCloudSyncS3FreshBucketCreatesNewKey(t *testing.T) {
	fake := newFakeS3Legacy()
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	service := &SyncService{}
	config := fake.s3Config(server.URL)

	resource, err := service.CloudSyncS3Initialize(config)
	if err != nil || resource.ResourceID != nil {
		t.Fatalf("empty bucket must resolve null, got %v %v", resource, err)
	}
	downloaded, err := service.CloudSyncS3Download(config)
	if err != nil || downloaded.SyncedFile != nil {
		t.Fatalf("empty bucket must download null, got %v %v", downloaded, err)
	}
	uploaded := legacySyncedFile("")
	uploaded.Payload = "bmV3"
	if _, err := service.CloudSyncS3Upload(config, uploaded); err != nil {
		t.Fatalf("fresh upload: %v", err)
	}
	if _, ok := fake.objects[mustKey(t, "", s3SnapshotFileName)]; !ok {
		t.Fatal("fresh snapshot must be created under the new key")
	}
	if _, ok := fake.objects[mustKey(t, "", legacyS3SnapshotFileName)]; ok {
		t.Fatal("fresh snapshot must not be created under the legacy key")
	}
}

// TestCloudSyncS3MixedBucketPrefersNewKey: when both objects exist, reads and
// writes resolve to the new key and the legacy object is never overwritten or
// deleted by an upload.
func TestCloudSyncS3MixedBucketPrefersNewKey(t *testing.T) {
	fake := newFakeS3Legacy()
	newKey := mustKey(t, "", s3SnapshotFileName)
	legacyKey := mustKey(t, "", legacyS3SnapshotFileName)
	fake.put(newKey, `{"meta":{"version":1},"payload":"bmV3LXNuYXBzaG90"}`, `"e-new"`)
	fake.put(legacyKey, `{"meta":{"version":1},"payload":"bGVnYWN5"}`, `"e-legacy"`)
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	service := &SyncService{}
	config := fake.s3Config(server.URL)

	downloaded, err := service.CloudSyncS3Download(config)
	if err != nil || downloaded.SyncedFile == nil {
		t.Fatalf("download: %v %v", downloaded, err)
	}
	if downloaded.SyncedFile.ETag != `"e-new"` || downloaded.SyncedFile.Payload != "bmV3LXNuYXBzaG90" {
		t.Fatalf("mixed bucket must read the new key, got %+v", downloaded.SyncedFile)
	}

	uploaded := downloaded.SyncedFile
	uploaded.Payload = "dXBkYXRlZA=="
	if _, err := service.CloudSyncS3Upload(config, *uploaded); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if got := fake.objects[legacyKey].body; string(got) != `{"meta":{"version":1},"payload":"bGVnYWN5"}` {
		t.Fatalf("legacy object must never be overwritten by an upload, got %q", got)
	}
	if got := fake.objects[newKey].body; !strings.Contains(string(got), "dXBkYXRlZA==") {
		t.Fatalf("new object must carry the update, got %q", got)
	}
}

// TestCloudSyncS3StaleETagStillConflicts proves the optimistic concurrency
// contract survives the rename: an upload with a stale ETag against the
// resolved key still surfaces ErrConflict.
func TestCloudSyncS3StaleETagStillConflicts(t *testing.T) {
	fake := newFakeS3Legacy()
	fake.put(mustKey(t, "", s3SnapshotFileName), `{"meta":{"version":1},"payload":"bmV3"}`, `"e-current"`)
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	service := &SyncService{}
	config := fake.s3Config(server.URL)

	stale := legacySyncedFile(`"e-stale"`)
	_, err := service.CloudSyncS3Upload(config, stale)
	if !errors.Is(err, cloudsync.ErrConflict) {
		t.Fatalf("stale etag must conflict, got %v", err)
	}
}

// TestCloudSyncS3DeleteRemovesBothKeys: deleting the remote snapshot is an
// explicit user intent, so both keys are removed (missing keys count as
// removed).
func TestCloudSyncS3DeleteRemovesBothKeys(t *testing.T) {
	fake := newFakeS3Legacy()
	fake.put(mustKey(t, "", s3SnapshotFileName), "new", `"e-new"`)
	fake.put(mustKey(t, "", legacyS3SnapshotFileName), "legacy", `"e-legacy"`)
	server := httptest.NewServer(fake.handler())
	defer server.Close()
	service := &SyncService{}
	config := fake.s3Config(server.URL)

	result, err := service.CloudSyncS3Delete(config)
	if err != nil || !result.OK {
		t.Fatalf("delete: %v %v", result, err)
	}
	if len(fake.objects) != 0 {
		t.Fatalf("both keys must be removed, left: %v", fake.objects)
	}
	// Deleting again stays OK (idempotent user intent).
	if result, err := service.CloudSyncS3Delete(config); err != nil || !result.OK {
		t.Fatalf("repeated delete must stay OK, got %v %v", result, err)
	}
}
