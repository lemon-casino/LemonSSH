package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/lemon-casino/lemonssh/internal/profile/store"
)

// treeSnapshot maps every regular file under root (relative slash path) to its
// content, so whole-directory comparisons can prove the legacy tree was left
// byte-for-byte untouched.
func treeSnapshot(t *testing.T, root string) map[string][]byte {
	t.Helper()
	snapshot := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snapshot[rel] = content
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return snapshot
}

func seedLegacyProfileDB(t *testing.T, legacy string) {
	t.Helper()
	db, err := store.Open(filepath.Join(legacy, "profile.db"), nil)
	if err != nil {
		t.Fatalf("open legacy profile.db: %v", err)
	}
	// Rows shaped like pre-rename data: the Go profile store keeps old
	// netcatty_-named rows after the brand migration (compat#4).
	mustSetRaw := func(domain, key, value string) {
		t.Helper()
		if err := db.SetRaw(domain, key, []byte(value)); err != nil {
			t.Fatalf("seed %s/%s: %v", domain, key, err)
		}
	}
	mustSetRaw("settings", "netcatty_theme_v1", "dark")
	mustSetRaw("vault", "netcatty_hosts_v1", `["legacy-host"]`)
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy profile.db: %v", err)
	}
}

func redirectConfigRootForLaunch(t *testing.T, root string) {
	t.Helper()
	redirectUserConfigDir(t, root)
	t.Setenv(profileDirEnv, "")
	t.Setenv(profileDirLegacyEnv, "")
}

func mustGetRaw(t *testing.T, db *store.Store, domain, key, want string) {
	t.Helper()
	value, err := db.GetRaw(domain, key)
	if err != nil {
		t.Fatalf("get %s/%s: %v", domain, key, err)
	}
	if string(value) != want {
		t.Fatalf("%s/%s = %q, want %q", domain, key, value, want)
	}
}

// TestOpenProfileStoreUpgradesLegacyTreeEndToEnd walks the real upgrade path:
// a pre-rename %AppData%/netcatty tree is migrated by the startup entry point
// (openProfileStore), the new lemonssh store serves both the migrated rows and
// post-upgrade writes, and the legacy tree stays byte-for-byte untouched.
// A second launch (idempotent, marker present) keeps every value.
func TestOpenProfileStoreUpgradesLegacyTreeEndToEnd(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, legacyProfileDirName)
	seedLegacyProfileDB(t, legacy)
	writeProfileFixtureFile(t, filepath.Join(legacy, "logs", "session-2024.log"), "old session")
	writeProfileFixtureFile(t, filepath.Join(legacy, "known_hosts"), "host old-key")
	legacyBefore := treeSnapshot(t, legacy)

	redirectConfigRootForLaunch(t, root)

	// First launch after the upgrade: migration runs inside openProfileStore.
	db, err := openProfileStore()
	if err != nil {
		t.Fatalf("first launch: %v", err)
	}
	if want := filepath.Join(root, profileDirName, "profile.db"); db.Path() != want {
		t.Fatalf("store path %q, want %q", db.Path(), want)
	}
	// Old rows stay readable under their original netcatty_ names.
	mustGetRaw(t, db, "settings", "netcatty_theme_v1", "dark")
	mustGetRaw(t, db, "vault", "netcatty_hosts_v1", `["legacy-host"]`)
	// Post-upgrade writes land in the new store.
	if err := db.SetRaw("settings", "lemonssh_theme_v1", []byte("light")); err != nil {
		t.Fatalf("post-upgrade write: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close first launch: %v", err)
	}

	// The legacy tree is preserved exactly: same files, same bytes, and it
	// never receives the migration marker.
	legacyAfter := treeSnapshot(t, legacy)
	if len(legacyAfter) != len(legacyBefore) {
		t.Fatalf("legacy tree changed: %d files before, %d after", len(legacyBefore), len(legacyAfter))
	}
	for rel, content := range legacyBefore {
		after, ok := legacyAfter[rel]
		if !ok {
			t.Fatalf("legacy file disappeared: %s", rel)
		}
		if !bytes.Equal(content, after) {
			t.Fatalf("legacy file modified: %s", rel)
		}
	}
	if _, err := os.Stat(filepath.Join(legacy, profileMigrationMarker)); !os.IsNotExist(err) {
		t.Fatalf("legacy tree must not carry the migration marker: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, profileDirName, profileMigrationMarker)); err != nil {
		t.Fatalf("target marker missing: %v", err)
	}

	// Second launch: the marker makes the migration a no-op, so both the
	// migrated rows and the post-upgrade write survive.
	second, err := openProfileStore()
	if err != nil {
		t.Fatalf("second launch: %v", err)
	}
	mustGetRaw(t, second, "settings", "netcatty_theme_v1", "dark")
	mustGetRaw(t, second, "vault", "netcatty_hosts_v1", `["legacy-host"]`)
	mustGetRaw(t, second, "settings", "lemonssh_theme_v1", "light")
	if err := second.Close(); err != nil {
		t.Fatalf("close second launch: %v", err)
	}
}

// TestOpenProfileStoreResumesAfterCrashBeforeMarker simulates a crash between
// the copy and the marker write: the next launch resumes the migration, never
// overwrites the newer target profile.db, and finishes by writing the marker.
func TestOpenProfileStoreResumesAfterCrashBeforeMarker(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, legacyProfileDirName)
	seedLegacyProfileDB(t, legacy)
	writeProfileFixtureFile(t, filepath.Join(legacy, "notes.txt"), "v1")
	legacyBefore := treeSnapshot(t, legacy)

	redirectConfigRootForLaunch(t, root)

	// First launch, then simulate a crash before the marker was durable.
	crashed, err := openProfileStore()
	if err != nil {
		t.Fatalf("first launch: %v", err)
	}
	if err := crashed.SetRaw("settings", "lemonssh_hosts_v1", []byte("fresh")); err != nil {
		t.Fatalf("write before crash: %v", err)
	}
	if err := crashed.Close(); err != nil {
		t.Fatalf("close before crash: %v", err)
	}
	if err := os.Remove(filepath.Join(root, profileDirName, profileMigrationMarker)); err != nil {
		t.Fatalf("simulate lost marker: %v", err)
	}

	// Next start resumes: the newer target profile.db must win over the
	// legacy copy, and the marker must be re-written.
	resumed, err := openProfileStore()
	if err != nil {
		t.Fatalf("resumed launch: %v", err)
	}
	mustGetRaw(t, resumed, "settings", "lemonssh_hosts_v1", "fresh")
	mustGetRaw(t, resumed, "vault", "netcatty_hosts_v1", `["legacy-host"]`)
	if err := resumed.Close(); err != nil {
		t.Fatalf("close resumed launch: %v", err)
	}
	if got := readProfileFixtureFile(t, filepath.Join(root, profileDirName, "notes.txt")); got != "v1" {
		t.Fatalf("missing target entry must be re-copied on resume, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, profileDirName, profileMigrationMarker)); err != nil {
		t.Fatalf("marker must be re-written after resume: %v", err)
	}
	legacyAfter := treeSnapshot(t, legacy)
	for rel, content := range legacyBefore {
		if !bytes.Equal(legacyAfter[rel], content) {
			t.Fatalf("legacy file modified during resume: %s", rel)
		}
	}
}
