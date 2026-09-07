package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStageAndPromote(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "profile", "profile.db")

	// Seed an original store so promotion exercises the backup path.
	original, err := Open(target, nil)
	if err != nil {
		t.Fatalf("open original: %v", err)
	}
	if err := original.SetRaw("vault", "old", []byte("old-value")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := original.Close(); err != nil {
		t.Fatalf("close original: %v", err)
	}

	stagingDir := filepath.Join(dir, "staging")
	stagingPath, err := StageProfile(stagingDir, []Mutation{
		{Domain: "vault", Key: "netcatty_hosts_v1", Value: []byte(`{"hosts":["h1"]}`)},
		{Domain: "settings", Key: "theme", Value: []byte("dark")},
	})
	if err != nil {
		t.Fatalf("stage: %v", err)
	}

	receipt, err := PromoteProfile(stagingPath, target, filepath.Join(dir, "backups"), "fingerprint-1")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if receipt.BackupManifest == nil || receipt.BackupManifest.OriginalSHA256 == "" {
		t.Fatal("promotion of an existing store must record a backup manifest")
	}
	if _, err := os.Stat(receipt.BackupManifest.BackupPath); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	storedReceipt, err := ReadMigrationReceipt(target)
	if err != nil {
		t.Fatalf("read receipt: %v", err)
	}
	if storedReceipt.SourceFingerprint != "fingerprint-1" || storedReceipt.SchemaVersion != SchemaVersion {
		t.Fatalf("receipt mismatch: %+v", storedReceipt)
	}

	promoted, err := Open(target, nil)
	if err != nil {
		t.Fatalf("open promoted: %v", err)
	}
	defer promoted.Close()
	value, err := promoted.GetRaw("vault", "netcatty_hosts_v1")
	if err != nil || string(value) != `{"hosts":["h1"]}` {
		t.Fatalf("promoted data mismatch: %s (%v)", value, err)
	}
	if _, err := promoted.GetRaw("vault", "old"); !errors.Is(err, ErrNoSuchKey) {
		t.Fatal("old key must not leak into the promoted store")
	}
}

func TestPromoteRejectsIncompleteStaging(t *testing.T) {
	dir := t.TempDir()
	stagingPath := filepath.Join(dir, "staging", "profile.db")
	if err := os.MkdirAll(filepath.Dir(stagingPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(stagingPath, []byte("garbage"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := PromoteProfile(stagingPath, filepath.Join(dir, "target.db"), filepath.Join(dir, "backups"), "fp"); err == nil {
		t.Fatal("incomplete staging store must be rejected")
	}
}

func TestCrashBeforePromotionLeavesTargetIntact(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "profile.db")

	original, err := Open(target, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := original.SetRaw("vault", "keep", []byte("me")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := original.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// "Crash" during staging: store exists without the completion marker.
	stagingPath := filepath.Join(dir, "staging", "profile.db")
	if err := os.MkdirAll(filepath.Dir(stagingPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := StageProfile(filepath.Join(dir, "staging"), []Mutation{{Domain: "vault", Key: "x", Value: []byte("y")}}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	_ = os.Remove(stagingPath + "." + completeMarker)

	if _, err := PromoteProfile(stagingPath, target, filepath.Join(dir, "backups"), "fp"); err == nil {
		t.Fatal("promotion after a staging crash must fail")
	}
	after, err := Open(target, nil)
	if err != nil {
		t.Fatalf("reopen target: %v", err)
	}
	defer after.Close()
	value, err := after.GetRaw("vault", "keep")
	if err != nil || string(value) != "me" {
		t.Fatalf("crashed staging must not touch the target, got %s (%v)", value, err)
	}
}
