package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/binaricat/lemonssh/internal/app/updateuse"
	"github.com/binaricat/lemonssh/internal/profile/store"
)

func newUpdateServiceFixture(t *testing.T) (*UpdateService, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "profile.db"), nil)
	if err != nil {
		t.Fatalf("open profile store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return newUpdateService("1.0.0", filepath.Join(dir, "updates"), "", db), db
}

func TestUpdateServiceAutoUpdateDefaultsEnabled(t *testing.T) {
	service, _ := newUpdateServiceFixture(t)
	if got := service.GetAutoUpdate()["enabled"]; got != true {
		t.Fatalf("GetAutoUpdate() = %v, want enabled by default", got)
	}
}

func TestUpdateServiceSetAutoUpdatePersists(t *testing.T) {
	service, db := newUpdateServiceFixture(t)
	if result := service.SetAutoUpdate(false); result["success"] != true {
		t.Fatalf("SetAutoUpdate() = %v, want success", result)
	}
	if service.GetAutoUpdate()["enabled"] != false {
		t.Fatal("SetAutoUpdate(false) did not flip the runtime toggle")
	}

	// The toggle survives a service rebuild from the same store.
	reborn := newUpdateService("1.0.0", t.TempDir(), "", db)
	if reborn.GetAutoUpdate()["enabled"] != false {
		t.Fatal("auto-update toggle did not persist across service rebuilds")
	}
}

func TestUpdateServiceDevVersionNeverChecks(t *testing.T) {
	service := newUpdateService("0.0.0", t.TempDir(), "", nil)
	result := service.CheckForUpdate()
	if result.Available || result.Supported {
		t.Fatalf("CheckForUpdate() = %+v, want unsupported for a dev build", result)
	}
	snapshot := service.GetUpdateStatus()
	if snapshot.IsChecking || snapshot.Status != "idle" {
		t.Fatalf("GetUpdateStatus() = %+v, want idle", snapshot)
	}
}

func TestUpdateServiceInstallWithoutDownloadFails(t *testing.T) {
	service := newUpdateService("1.0.0", t.TempDir(), "", nil)
	if err := service.InstallUpdate(); err == nil {
		t.Fatal("InstallUpdate() without a ready artifact must fail")
	}
}

func TestUpdateServiceEventNamesMatchFrontendContract(t *testing.T) {
	// The frontend facade subscribes to these exact names
	// (infrastructure/runtime/wails/wailsRuntimeClient.ts); keep both sides
	// in lockstep.
	source, err := os.ReadFile(filepath.Join("..", "..", "infrastructure", "runtime", "wails", "wailsRuntimeClient.ts"))
	if err != nil {
		t.Skipf("frontend source unavailable: %v", err)
	}
	for _, name := range []string{
		updateuse.EventAvailable,
		updateuse.EventNotAvailable,
		updateuse.EventDownloadProgress,
		updateuse.EventDownloaded,
		updateuse.EventError,
	} {
		if !strings.Contains(string(source), `"`+name+`"`) {
			t.Fatalf("frontend runtime client does not subscribe to %q", name)
		}
	}
}
