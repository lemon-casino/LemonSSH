package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func persistSnapshot(t *testing.T, records []*PackageRecord) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plugins.json")
	encoded, err := json.Marshal(snapshotFile{Plugins: records})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSaveLoadAtomicRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugins.json")
	store := New()
	record, err := store.Install("acme", "1.2.3", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", json.RawMessage(`{"apiVersion":2}`))
	if err != nil {
		t.Fatal(err)
	}
	record.State = StateEnabled
	if err := store.SaveAtomic(path); err != nil {
		t.Fatal(err)
	}
	// A second save rotates the first snapshot to .bak; the very first save on
	// a fresh install has nothing to rotate.
	if err := store.SaveAtomic(path); err != nil {
		t.Fatal(err)
	}

	reloaded := New()
	if err := reloaded.LoadAtomic(path); err != nil {
		t.Fatal(err)
	}
	got, ok := reloaded.Get("acme")
	if !ok || got.Version != "1.2.3" || got.State != StateEnabled {
		t.Fatalf("reloaded %+v", got)
	}
	// Previous snapshot rotated to .bak.
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
}

func TestLoadAtomicMissingFileLeavesStoreUntouched(t *testing.T) {
	store := New()
	if _, err := store.Install("keep", "1.0.0", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := store.LoadAtomic(filepath.Join(t.TempDir(), "absent.json")); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Get("keep"); !ok {
		t.Fatal("fresh-install inventory must survive a missing snapshot")
	}
}

func TestLoadAtomicFallsBackToBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugins.json")
	good := persistSnapshot(t, []*PackageRecord{{PluginID: "good", Version: "2.0.0", State: StateInstalled}})
	if err := os.Rename(good, path+".bak"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := New()
	if err := store.LoadAtomic(path); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Get("good"); !ok {
		t.Fatal("backup snapshot must be recovered")
	}
}

func TestRecoverStagedDropsInterruptedPublish(t *testing.T) {
	store := New()
	if _, err := store.StageInstall("pending", "1.0.0", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if len(store.StagedPlugins()) != 1 {
		t.Fatal("staged record missing")
	}
	if removed := store.RecoverStaged(); removed != 1 {
		t.Fatalf("recovery removed %d", removed)
	}
	if len(store.StagedPlugins()) != 0 {
		t.Fatal("staged record survived recovery")
	}
}
