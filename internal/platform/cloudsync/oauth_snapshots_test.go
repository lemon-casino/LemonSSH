package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestOAuthEncryptedSnapshotCRUDAndETag(t *testing.T) {
	for _, provider := range []string{"google", "onedrive", "github"} {
		t.Run(provider, func(t *testing.T) {
			var stored []byte
			etag := `"revision-1"`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fixture-token" {
					t.Error("bearer token missing")
					w.WriteHeader(401)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("ETag", etag)
				if r.Method == "GET" {
					if provider == "google" && r.URL.Path == "/drive/v3/files" {
						if r.URL.Query().Get("spaces") != "appDataFolder" {
							t.Error("outside appDataFolder")
						}
						if stored == nil {
							io.WriteString(w, `{"files":[]}`)
						} else {
							io.WriteString(w, `{"files":[{"id":"file-1"}]}`)
						}
						return
					}
					if provider == "google" && r.URL.Path == "/drive/v3/files/file-1/revisions" {
						io.WriteString(w, `{"revisions":[{"id":"rev-2","modifiedTime":"2026-01-02T00:00:00Z"},{"id":"rev-1","modifiedTime":"2026-01-01T00:00:00Z"}]}`)
						return
					}
					if provider == "google" && r.URL.Path == "/drive/v3/files/file-1/revisions/rev-1" {
						io.WriteString(w, `{"meta":{"version":6},"payload":"cmV2aXNpb24tb25l"}`)
						return
					}
					if provider == "github" && r.URL.Path == "/gists" {
						if stored == nil {
							io.WriteString(w, `[]`)
						} else {
							json.NewEncoder(w).Encode([]gist{{ID: "file-1", Description: gistDescription, Files: map[string]gistFile{syncFileName: {Content: string(stored)}}}})
						}
						return
					}
					if stored == nil {
						w.WriteHeader(404)
						return
					}
					if provider == "github" {
						g := gist{ID: "file-1", Description: gistDescription, Files: map[string]gistFile{syncFileName: {Content: string(stored), Size: len(stored)}}}
						g.History = append(g.History, struct {
							Version string `json:"version"`
							Date    string `json:"committed_at"`
						}{"sha", "2026-01-01T00:00:00Z"})
						json.NewEncoder(w).Encode(g)
						return
					}
					if provider == "onedrive" && !strings.HasSuffix(r.URL.Path, "/content") {
						fmt.Fprintf(w, `{"id":"file-1","eTag":%q}`, etag)
						return
					}
					w.Write(stored)
					return
				}
				if r.Method == "DELETE" {
					stored = nil
					w.WriteHeader(204)
					return
				}
				if r.Header.Get("If-Match") != "" && r.Header.Get("If-Match") != etag {
					w.WriteHeader(412)
					return
				}
				body, _ := io.ReadAll(r.Body)
				if provider == "google" && r.Method == "POST" {
					_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
					parts := multipart.NewReader(strings.NewReader(string(body)), params["boundary"])
					meta, err := parts.NextPart()
					if err != nil {
						t.Error(err)
						return
					}
					metadata, _ := io.ReadAll(meta)
					if !strings.Contains(string(metadata), `"parents":["appDataFolder"]`) {
						t.Error("missing appDataFolder parent")
					}
					file, err := parts.NextPart()
					if err != nil {
						t.Error(err)
						return
					}
					body, _ = io.ReadAll(file)
				}
				if provider == "github" {
					var g gist
					if err := json.Unmarshal(body, &g); err != nil {
						t.Error(err)
						return
					}
					body = []byte(g.Files[syncFileName].Content)
				}
				stored = body
				io.WriteString(w, `{"id":"file-1"}`)
			}))
			defer server.Close()
			c := NewOAuthClient()
			defer c.Close()
			c.googleAPI, c.googleUpload, c.githubAPI, c.graph = server.URL, server.URL+"/upload/drive/v3", server.URL, server.URL
			ctx := context.Background()
			find := c.GoogleFind
			upload := c.GoogleCreate
			download := c.GoogleDownload
			remove := c.GoogleDelete
			if provider == "onedrive" {
				find, upload, download, remove = c.OneDriveFind, c.OneDriveUpload, c.OneDriveDownload, c.OneDriveDelete
			}
			if provider == "github" {
				find, upload, download, remove = c.GitHubFind, c.GitHubUpload, c.GitHubDownload, c.GitHubDelete
			}
			o := FileOptions{AccessToken: "fixture-token"}
			if result, err := find(ctx, o); err != nil || result.FileID != nil {
				t.Fatalf("empty: %v", err)
			}
			o.SyncedFile = &SyncedFile{Meta: json.RawMessage(`{"version":7,"syncSchemaVersion":2,"iv":"fixture-iv","salt":"fixture-salt","kdf":"PBKDF2"}`), Payload: "ZW5jcnlwdGVkLWZpeHR1cmU="}
			created, err := upload(ctx, o)
			if err != nil || created.FileID == nil {
				t.Fatalf("create: %v", err)
			}
			o.FileID = *created.FileID
			got, err := download(ctx, o)
			if err != nil {
				t.Fatal(err)
			}
			if got.SyncedFile == nil || got.SyncedFile.Payload != o.SyncedFile.Payload || !reflect.DeepEqual(got.SyncedFile.Meta, o.SyncedFile.Meta) || got.SyncedFile.ETag != etag {
				t.Fatalf("fixture/etag changed: %#v", got.SyncedFile)
			}
			o.SyncedFile.ETag = `"stale"`
			if provider == "google" {
				_, err = c.GoogleUpdate(ctx, o)
			} else {
				_, err = upload(ctx, o)
			}
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("stale update: %v", err)
			}
			o.SyncedFile.ETag = etag
			if provider == "google" {
				_, err = c.GoogleUpdate(ctx, o)
			} else {
				_, err = upload(ctx, o)
			}
			if err != nil {
				t.Fatal(err)
			}
			if provider == "github" {
				history, err := c.GitHubHistory(ctx, o)
				if err != nil || len(history) != 1 {
					t.Fatalf("history: %v", err)
				}
				o.Revision = "sha"
				if _, err := download(ctx, o); err != nil {
					t.Fatal(err)
				}
				o.Revision = ""
			}
			if provider == "google" {
				history, err := c.GoogleRevisionHistory(ctx, o)
				if err != nil || len(history) != 2 {
					t.Fatalf("google history: %v", err)
				}
				if history[0].Version != "rev-2" || history[1].Version != "rev-1" {
					t.Fatalf("google history order: %#v", history)
				}
				o.Revision = "rev-1"
				downloaded, err := download(ctx, o)
				if err != nil || downloaded.SyncedFile == nil {
					t.Fatalf("google revision download: %v", err)
				}
				if !strings.Contains(string(downloaded.SyncedFile.Payload), "cmV2aXNpb24tb25l") {
					t.Fatalf("google revision content: %s", downloaded.SyncedFile.Payload)
				}
				o.Revision = ""
			}
			if _, err = remove(ctx, o); err != nil {
				t.Fatal(err)
			}
			if result, err := download(ctx, o); err != nil || result.SyncedFile != nil {
				t.Fatalf("delete/read: %v", err)
			}
		})
	}
}

func TestOAuthSnapshotFailClosed(t *testing.T) {
	c := NewOAuthClient()
	defer c.Close()
	ctx := context.Background()
	if _, err := c.GoogleUpdate(ctx, FileOptions{FileID: "../escape", SyncedFile: &SyncedFile{Meta: json.RawMessage(`{}`), Payload: "eA=="}}); err == nil {
		t.Fatal("invalid ID allowed")
	}
	if _, err := c.OneDriveUpload(ctx, FileOptions{FileName: "other-file"}); err == nil {
		t.Fatal("arbitrary file allowed")
	}
	if _, err := c.GitHubUpload(ctx, FileOptions{SyncedFile: &SyncedFile{Meta: json.RawMessage(`{}`), Payload: "plaintext"}}); err == nil {
		t.Fatal("non-base64 snapshot allowed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://127.0.0.1/private?secret=fixture")
		w.WriteHeader(302)
	}))
	defer server.Close()
	c.googleAPI = server.URL
	if _, err := c.User(ctx, "google", UserOptions{AccessToken: "fixture"}); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("redirect not contained: %v", err)
	}
}
