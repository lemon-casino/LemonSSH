package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/binaricat/netcatty/internal/platform/filesystem"
)

// The boot sweep must remove staging leftovers from a previous process
// (fresh process holds no leases) while preserving external-edit downloads
// and anything outside the known staging prefixes. Leftovers are created as
// raw filesystem entries, because entries registered on THIS instance are
// active leases and must survive the sweep.
func TestSweepTempOrphansRemovesStagingLeftoversOnly(t *testing.T) {
	temp, err := filesystem.NewTempService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stagedFile, err := os.CreateTemp(temp.Root(), filesystem.StagedUploadPrefix+"abandoned-upload.bin")
	if err != nil {
		t.Fatal(err)
	}
	_ = stagedFile.Close()
	transferDir, err := os.MkdirTemp(temp.Root(), filesystem.TransferTempPrefix)
	if err != nil {
		t.Fatal(err)
	}
	externalDownload := filepath.Join(temp.Root(), "external-edit-report.pdf")
	if err := os.WriteFile(externalDownload, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}

	removed := sweepTempOrphans(temp)

	if removed != 2 {
		t.Fatalf("removed %d orphan entries, want 2", removed)
	}
	if _, err := os.Stat(stagedFile.Name()); !os.IsNotExist(err) {
		t.Fatalf("staged leftover survived the sweep: %v", err)
	}
	if _, err := os.Stat(transferDir); !os.IsNotExist(err) {
		t.Fatalf("transfer staging dir survived the sweep: %v", err)
	}
	if _, err := os.Stat(externalDownload); err != nil {
		t.Fatalf("external-edit download must be preserved: %v", err)
	}

	// A second sweep is idempotent: nothing left to remove.
	if again := sweepTempOrphans(temp); again != 0 {
		t.Fatalf("second sweep removed %d entries, want 0", again)
	}

	// Entries leased by the running process are never swept.
	leased, err := temp.CreateStagingFile("in-flight.bin")
	if err != nil {
		t.Fatal(err)
	}
	_ = leased.Close()
	if again := sweepTempOrphans(temp); again != 0 {
		t.Fatalf("sweep removed %d leased entries, want 0", again)
	}
	if _, err := os.Stat(leased.Name()); err != nil {
		t.Fatalf("leased staging entry must survive: %v", err)
	}
}
