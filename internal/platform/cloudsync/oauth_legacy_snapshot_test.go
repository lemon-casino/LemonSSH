package cloudsync

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const legacyFixtureSnapshot = `{"meta":{"version":1},"payload":"bGVnYWN5LWNpcGhlcnRleHQ="}`

func TestValidFileNameAcceptsLegacyName(t *testing.T) {
	for _, name := range []string{"", syncFileName, legacySyncFileName} {
		if err := validFileName(name); err != nil {
			t.Fatalf("validFileName(%q) = %v, want nil", name, err)
		}
	}
	if err := validFileName("other.json"); err == nil {
		t.Fatal("arbitrary file names must be rejected")
	}
}

// TestGitHubLegacyIdentityRoundTrip proves a pre-rename gist stays a first
// class citizen: Find matches the legacy name/description pair, downloads
// serve the legacy file, and PATCH updates the gist in place under its
// original identity instead of renaming it.
func TestGitHubLegacyIdentityRoundTrip(t *testing.T) {
	var patchBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/gists":
			legacy := gist{
				ID:          "file-1",
				Description: legacyGistDescription,
				Files:       map[string]gistFile{legacySyncFileName: {Content: legacyFixtureSnapshot, Size: len(legacyFixtureSnapshot)}},
			}
			_ = json.NewEncoder(w).Encode([]gist{legacy})
		case r.Method == "GET" && r.URL.Path == "/gists/file-1":
			legacy := gist{
				ID:          "file-1",
				Description: legacyGistDescription,
				Files:       map[string]gistFile{legacySyncFileName: {Content: legacyFixtureSnapshot, Size: len(legacyFixtureSnapshot)}},
			}
			_ = json.NewEncoder(w).Encode(legacy)
		case r.Method == "PATCH" && r.URL.Path == "/gists/file-1":
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &patchBody); err != nil {
				t.Errorf("patch body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"file-1"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c := NewOAuthClient()
	defer c.Close()
	c.githubAPI = server.URL
	ctx := context.Background()

	// Find without a stored file id must discover the legacy gist.
	found, err := c.GitHubFind(ctx, FileOptions{AccessToken: "fixture-token"})
	if err != nil || found.FileID == nil || *found.FileID != "file-1" {
		t.Fatalf("legacy gist must be discoverable, got %v %v", found, err)
	}

	// Download must serve the legacy file content. Snapshots travel base64
	// encoded: the served payload decodes to "legacy-ciphertext".
	downloaded, err := c.GitHubDownload(ctx, FileOptions{AccessToken: "fixture-token", FileID: "file-1"})
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if downloaded.SyncedFile == nil || string(downloaded.SyncedFile.Payload) != "bGVnYWN5LWNpcGhlcnRleHQ=" {
		t.Fatalf("legacy payload must be served, got %v", downloaded.SyncedFile)
	}

	// Upload must PATCH under the original legacy identity, not rename it.
	etag := `"patched-etag"`
	uploaded, err := c.GitHubUpload(ctx, FileOptions{
		AccessToken: "fixture-token",
		FileID:      "file-1",
		SyncedFile: &SyncedFile{
			Meta:    json.RawMessage(`{"version":1}`),
			Payload: "bmV3LWNpcGhlcnRleHQ=",
			ETag:    etag,
		},
	})
	if err != nil || uploaded.FileID == nil || *uploaded.FileID != "file-1" {
		t.Fatalf("upload onto legacy gist: %v %v", uploaded, err)
	}
	if patchBody["description"] != legacyGistDescription {
		t.Fatalf("PATCH must keep the legacy description, got %v", patchBody["description"])
	}
	files, _ := patchBody["files"].(map[string]any)
	if _, ok := files[legacySyncFileName]; !ok {
		t.Fatalf("PATCH must update the legacy file name, got %v", files)
	}
	if _, ok := files[syncFileName]; ok {
		t.Fatalf("PATCH must not introduce the new file name, got %v", files)
	}
}

// TestGitHubFindMatchesNewIdentity: a gist created under the current identity
// is still discovered after the rename.
func TestGitHubFindMatchesNewIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		current := gist{
			ID:          "file-2",
			Description: gistDescription,
			Files:       map[string]gistFile{syncFileName: {Content: legacyFixtureSnapshot, Size: len(legacyFixtureSnapshot)}},
		}
		_ = json.NewEncoder(w).Encode([]gist{current})
	}))
	defer server.Close()

	c := NewOAuthClient()
	defer c.Close()
	c.githubAPI = server.URL
	found, err := c.GitHubFind(context.Background(), FileOptions{AccessToken: "fixture-token"})
	if err != nil || found.FileID == nil || *found.FileID != "file-2" {
		t.Fatalf("current-identity gist must be discoverable, got %v %v", found, err)
	}
}

// TestOneDriveLegacySnapshotFallback proves the name-addressed OneDrive
// snapshot falls back to the legacy name on reads and keeps writing to the
// name that actually exists.
func TestOneDriveLegacySnapshotFallback(t *testing.T) {
	const legacyPath = "/me/drive/special/approot:/" + legacySyncFileName
	var putPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "PUT":
			putPath = r.URL.Path
			if r.Header.Get("If-Match") != `"e1"` {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			_, _ = io.WriteString(w, `{"id":"file-1"}`)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, ":/content"):
			_, _ = io.WriteString(w, legacyFixtureSnapshot)
		case r.Method == "GET" && r.URL.Path == legacyPath:
			_, _ = io.WriteString(w, `{"id":"file-1","eTag":"e1"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c := NewOAuthClient()
	defer c.Close()
	c.graph = server.URL
	ctx := context.Background()
	options := FileOptions{AccessToken: "fixture-token", FileName: legacySyncFileName}

	found, err := c.OneDriveFind(ctx, options)
	if err != nil || found.FileID == nil || *found.FileID != "file-1" || found.ETag != "e1" {
		t.Fatalf("legacy OneDrive snapshot must be discoverable, got %v %v", found, err)
	}

	uploaded, err := c.OneDriveUpload(ctx, FileOptions{
		AccessToken: "fixture-token",
		FileName:    legacySyncFileName,
		SyncedFile: &SyncedFile{
			Meta:    json.RawMessage(`{"version":1}`),
			Payload: "bmV3LWNpcGhlcnRleHQ=",
			ETag:    `"e1"`,
		},
	})
	if err != nil || uploaded.FileID == nil {
		t.Fatalf("upload onto legacy snapshot: %v %v", uploaded, err)
	}
	if putPath != legacyPath+":/content" {
		t.Fatalf("upload must write back to the legacy name, got %q", putPath)
	}

	downloaded, err := c.OneDriveDownload(ctx, options)
	if err != nil || downloaded.SyncedFile == nil {
		t.Fatalf("download: %v %v", downloaded, err)
	}
	// Snapshots travel base64 encoded: the served payload decodes to
	// "legacy-ciphertext".
	if string(downloaded.SyncedFile.Payload) != "bGVnYWN5LWNpcGhlcnRleHQ=" {
		t.Fatalf("legacy payload must be served, got %q", downloaded.SyncedFile.Payload)
	}
}

// TestOneDriveFreshSnapshotUsesNewName: with neither name present, uploads
// create the snapshot under the current name.
func TestOneDriveFreshSnapshotUsesNewName(t *testing.T) {
	var putPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method == "PUT" {
			putPath = r.URL.Path
			_, _ = io.WriteString(w, `{"id":"file-9"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := NewOAuthClient()
	defer c.Close()
	c.graph = server.URL
	uploaded, err := c.OneDriveUpload(context.Background(), FileOptions{
		AccessToken: "fixture-token",
		SyncedFile: &SyncedFile{
			Meta:    json.RawMessage(`{"version":1}`),
			Payload: "bmV3",
			ETag:    "",
		},
	})
	if err != nil || uploaded.FileID == nil || *uploaded.FileID != "file-9" {
		t.Fatalf("fresh upload: %v %v", uploaded, err)
	}
	if putPath != appRootSnapshotPath(syncFileName)+":/content" {
		t.Fatalf("fresh snapshot must be created under %s, got %q", syncFileName, putPath)
	}
}
