package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProfileFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readProfileFixtureFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func redirectUserConfigDir(t *testing.T, root string) {
	t.Helper()
	previous := osUserConfigDir
	osUserConfigDir = func() (string, error) { return root, nil }
	t.Cleanup(func() { osUserConfigDir = previous })
}

// TestEnsureProfileDirMigratedFreshMachine: no legacy directory means a
// straight first-run creation of the target.
func TestEnsureProfileDirMigratedFreshMachine(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "lemonssh")
	legacy := filepath.Join(root, "netcatty")

	resolved, err := ensureProfileDirMigrated(target, legacy)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if resolved != target {
		t.Fatalf("resolved %q, want target %q", resolved, target)
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		t.Fatalf("target must be created: %v", err)
	}
}

// TestEnsureProfileDirMigratedFromLegacyOnly: a pure legacy tree is copied
// wholesale, the legacy tree stays untouched, and the marker lands last.
func TestEnsureProfileDirMigratedFromLegacyOnly(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "lemonssh")
	legacy := filepath.Join(root, "netcatty")
	writeProfileFixtureFile(t, filepath.Join(legacy, "profile.db"), "legacy-db")
	writeProfileFixtureFile(t, filepath.Join(legacy, "logs", "session.log"), "log-line")

	resolved, err := ensureProfileDirMigrated(target, legacy)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if resolved != target {
		t.Fatalf("resolved %q, want %q", resolved, target)
	}
	if got := readProfileFixtureFile(t, filepath.Join(target, "profile.db")); got != "legacy-db" {
		t.Fatalf("target profile.db = %q", got)
	}
	if got := readProfileFixtureFile(t, filepath.Join(target, "logs", "session.log")); got != "log-line" {
		t.Fatalf("nested file not copied: %q", got)
	}
	if got := readProfileFixtureFile(t, filepath.Join(legacy, "profile.db")); got != "legacy-db" {
		t.Fatalf("legacy tree must stay untouched, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(target, profileMigrationMarker)); err != nil {
		t.Fatalf("migration marker missing: %v", err)
	}
}

// TestEnsureProfileDirMigratedResumesWithoutOverwrite: with both directories
// present and no marker, missing entries are copied while existing target
// entries (newer data) are never overwritten.
func TestEnsureProfileDirMigratedResumesWithoutOverwrite(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "lemonssh")
	legacy := filepath.Join(root, "netcatty")
	writeProfileFixtureFile(t, filepath.Join(legacy, "profile.db"), "legacy-db")
	writeProfileFixtureFile(t, filepath.Join(legacy, "notes.txt"), "note")
	writeProfileFixtureFile(t, filepath.Join(target, "profile.db"), "newer-db")

	resolved, err := ensureProfileDirMigrated(target, legacy)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if resolved != target {
		t.Fatalf("resolved %q, want %q", resolved, target)
	}
	if got := readProfileFixtureFile(t, filepath.Join(target, "profile.db")); got != "newer-db" {
		t.Fatalf("existing target entry was overwritten: %q", got)
	}
	if got := readProfileFixtureFile(t, filepath.Join(target, "notes.txt")); got != "note" {
		t.Fatalf("missing entry must be copied, got %q", got)
	}
}

// TestEnsureProfileDirMigratedIdempotent: once the marker exists, repeated
// starts are no-ops — data written after the migration is never clobbered by
// a re-copy from the legacy tree.
func TestEnsureProfileDirMigratedIdempotent(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "lemonssh")
	legacy := filepath.Join(root, "netcatty")
	writeProfileFixtureFile(t, filepath.Join(legacy, "profile.db"), "legacy-db")

	if _, err := ensureProfileDirMigrated(target, legacy); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	writeProfileFixtureFile(t, filepath.Join(target, "profile.db"), "mutated-after-migration")
	if err := os.Remove(filepath.Join(target, "logs")); err != nil && !os.IsNotExist(err) {
		t.Fatalf("cleanup: %v", err)
	}

	resolved, err := ensureProfileDirMigrated(target, legacy)
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if resolved != target {
		t.Fatalf("resolved %q, want %q", resolved, target)
	}
	if got := readProfileFixtureFile(t, filepath.Join(target, "profile.db")); got != "mutated-after-migration" {
		t.Fatalf("marker must make the migration a no-op, got %q", got)
	}
}

// TestProfileDirCandidatesEnvOverride: an explicit LEMONSSH_PROFILE_DIR wins
// outright and disables migration (legacy stays empty).
func TestProfileDirCandidatesEnvOverride(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "portable")
	t.Setenv(profileDirEnv, dir)
	t.Setenv(profileDirLegacyEnv, "")
	target, legacy := profileDirCandidates()
	if target != dir || legacy != "" {
		t.Fatalf("target=%q legacy=%q, want (%q, \"\")", target, legacy, dir)
	}
}

// TestProfileDirCandidatesLegacyEnvFallback: the legacy env var only
// redirects the migration source; the target stays the default directory.
func TestProfileDirCandidatesLegacyEnvFallback(t *testing.T) {
	root := t.TempDir()
	redirectUserConfigDir(t, root)
	legacyDir := filepath.Join(t.TempDir(), "custom")
	t.Setenv(profileDirEnv, "")
	t.Setenv(profileDirLegacyEnv, legacyDir)

	target, legacy := profileDirCandidates()
	if want := filepath.Join(root, profileDirName); target != want {
		t.Fatalf("target %q, want %q", target, want)
	}
	if legacy != legacyDir {
		t.Fatalf("legacy %q, want %q", legacy, legacyDir)
	}

	t.Setenv(profileDirLegacyEnv, "")
	target, legacy = profileDirCandidates()
	if want := filepath.Join(root, legacyProfileDirName); legacy != want {
		t.Fatalf("default legacy %q, want %q", legacy, want)
	}
}

// TestBaseProfileDirFallsBackToLegacyOnMigrationFailure: a failed copy must
// keep this session on the legacy directory (no empty-looking first launch)
// and must not write the marker, so the next start retries.
func TestBaseProfileDirFallsBackToLegacyOnMigrationFailure(t *testing.T) {
	root := t.TempDir()
	redirectUserConfigDir(t, root)
	legacy := filepath.Join(root, "netcatty")
	writeProfileFixtureFile(t, filepath.Join(legacy, "profile.db"), "legacy-db")
	t.Setenv(profileDirEnv, "")
	t.Setenv(profileDirLegacyEnv, "")

	previous := copyProfileTree
	copyProfileTree = func(src, dst string) error { return errors.New("disk full") }
	t.Cleanup(func() { copyProfileTree = previous })

	resolved := baseProfileDir()
	if resolved != legacy {
		t.Fatalf("baseProfileDir %q, want legacy %q after failed migration", resolved, legacy)
	}
	if _, err := os.Stat(filepath.Join(root, profileDirName, profileMigrationMarker)); !os.IsNotExist(err) {
		t.Fatalf("marker must not be written on failure: %v", err)
	}
}

// TestBaseProfileDirCreatesTargetWhenUserConfigFails: without a resolvable
// config dir the historical "." fallback keeps working.
func TestBaseProfileDirCreatesTargetWhenUserConfigFails(t *testing.T) {
	previous := osUserConfigDir
	osUserConfigDir = func() (string, error) { return "", errors.New("no config dir") }
	t.Cleanup(func() { osUserConfigDir = previous })
	t.Setenv(profileDirEnv, "")
	t.Setenv(profileDirLegacyEnv, "")

	if resolved := baseProfileDir(); resolved != "." {
		t.Fatalf("baseProfileDir %q, want \".\"", resolved)
	}
	if !strings.HasPrefix(profileMigrationMarker, ".lemonssh") {
		t.Fatal("marker name must use the lemonssh prefix")
	}
}
