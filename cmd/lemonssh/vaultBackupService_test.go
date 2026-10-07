package main

import (
	"path/filepath"
	"testing"
)

// availableCredentials adapts memoryCredentials to the full credentials.Provider
// contract so VaultBackupService treats sealing as available in tests.
type availableCredentials struct {
	memoryCredentials
}

func (a *availableCredentials) Name() string    { return "test-credentials" }
func (a *availableCredentials) Available() bool { return true }

func newTestVaultBackupService(t *testing.T) (*VaultBackupService, *[]recordedEvent) {
	t.Helper()
	service := newVaultBackupService(t.TempDir(), &availableCredentials{})
	emit, events := newEventRecorder()
	service.setEventEmitter(emit)
	return service, events
}

func TestVaultBackupBroadcastsChangedOnlyOnRealMutations(t *testing.T) {
	service, events := newTestVaultBackupService(t)

	first, err := service.CreateVaultBackup(VaultBackupCreateRequest{
		Payload: []byte(`{"hosts":[{"host":"a"}]}`),
		Reason:  "before_restore",
	})
	if err != nil || !first.Created {
		t.Fatalf("first create: created=%v err=%v", first.Created, err)
	}
	if got := len(eventsByName(events, vaultBackupsChangedEvent)); got != 1 {
		t.Fatalf("broadcasts after first create = %d, want 1", got)
	}

	// Identical fingerprint short-circuits without writing: no broadcast.
	duplicate, err := service.CreateVaultBackup(VaultBackupCreateRequest{
		Payload: []byte(`{"hosts":[{"host":"a"}]}`),
		Reason:  "before_restore",
	})
	if err != nil || duplicate.Created {
		t.Fatalf("duplicate create: created=%v err=%v", duplicate.Created, err)
	}
	if got := len(eventsByName(events, vaultBackupsChangedEvent)); got != 1 {
		t.Fatalf("broadcasts after duplicate create = %d, want 1", got)
	}

	second, err := service.CreateVaultBackup(VaultBackupCreateRequest{
		Payload: []byte(`{"hosts":[{"host":"a"},{"host":"b"}]}`),
		Reason:  "before_restore",
	})
	if err != nil || !second.Created {
		t.Fatalf("second create: created=%v err=%v", second.Created, err)
	}
	if got := len(eventsByName(events, vaultBackupsChangedEvent)); got != 2 {
		t.Fatalf("broadcasts after second create = %d, want 2", got)
	}

	// Trimming to one kept backup deletes the older one: one broadcast.
	trim, err := service.TrimVaultBackups(VaultBackupTrimRequest{MaxCount: 1})
	if err != nil || trim.DeletedCount != 1 {
		t.Fatalf("trim: deleted=%d err=%v", trim.DeletedCount, err)
	}
	if got := len(eventsByName(events, vaultBackupsChangedEvent)); got != 3 {
		t.Fatalf("broadcasts after trim = %d, want 3", got)
	}

	// A trim that deletes nothing stays silent.
	empty, err := service.TrimVaultBackups(VaultBackupTrimRequest{MaxCount: 100})
	if err != nil || empty.DeletedCount != 0 {
		t.Fatalf("empty trim: deleted=%d err=%v", empty.DeletedCount, err)
	}
	if got := len(eventsByName(events, vaultBackupsChangedEvent)); got != 3 {
		t.Fatalf("broadcasts after empty trim = %d, want 3", got)
	}
}

func TestVaultBackupBroadcastWithoutEmitterIsNoop(t *testing.T) {
	service := newVaultBackupService(filepath.Join(t.TempDir(), "backups"), &availableCredentials{})
	if _, err := service.CreateVaultBackup(VaultBackupCreateRequest{
		Payload: []byte(`{"notes":[{"id":1}]}`),
		Reason:  "before_restore",
	}); err != nil {
		t.Fatal(err)
	}
}
