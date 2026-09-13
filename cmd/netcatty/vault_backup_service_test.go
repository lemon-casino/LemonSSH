package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/binaricat/netcatty/internal/platform/credentials"
)

type memoryKeyring struct {
	m map[string]string
}

func (k *memoryKeyring) Get(service, user string) (string, error) {
	value, ok := k.m[service+"/"+user]
	if !ok {
		return "", errors.New("missing")
	}
	return value, nil
}
func (k *memoryKeyring) Set(service, user, password string) error {
	if k.m == nil {
		k.m = map[string]string{}
	}
	k.m[service+"/"+user] = password
	return nil
}
func (k *memoryKeyring) Delete(service, user string) error {
	delete(k.m, service+"/"+user)
	return nil
}

func sampleVaultPayload(hostLabel string) json.RawMessage {
	body, _ := json.Marshal(map[string]any{
		"hosts":     []any{map[string]any{"id": "h1", "label": hostLabel}},
		"keys":      []any{},
		"snippets":  []any{},
		"syncedAt":  1,
	})
	return body
}

func TestVaultBackupRoundTripAndDedupe(t *testing.T) {
	dir := t.TempDir()
	service := newVaultBackupService(dir, credentials.New(&memoryKeyring{}))
	first, err := service.CreateVaultBackup(VaultBackupCreateRequest{
		Payload: sampleVaultPayload("prod"),
		Reason:  "before_restore",
		MaxCount: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created || first.Backup == nil || first.Backup.Preview.HostCount != 1 {
		t.Fatalf("first backup: %#v", first)
	}
	duplicate, err := service.CreateVaultBackup(VaultBackupCreateRequest{
		Payload: sampleVaultPayload("prod"),
		Reason:  "before_restore",
		MaxCount: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Created || duplicate.Backup == nil || duplicate.Backup.ID != first.Backup.ID {
		t.Fatalf("identical payload must dedupe, got %#v", duplicate)
	}
	listed, err := service.ListVaultBackups()
	if err != nil || len(listed) != 1 {
		t.Fatalf("list: %v %#v", err, listed)
	}
	restored, err := service.ReadVaultBackup(VaultBackupReadRequest{ID: first.Backup.ID})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if json.Unmarshal(restored.Payload, &payload) != nil {
		t.Fatal(restored.Payload)
	}
	hosts, _ := payload["hosts"].([]any)
	host, _ := hosts[0].(map[string]any)
	if host["label"] != "prod" {
		t.Fatalf("restored payload: %#v", payload)
	}
}

func TestVaultBackupRefusesWhenEncryptionUnavailable(t *testing.T) {
	service := newVaultBackupService(t.TempDir(), credentials.New(nil))
	_, err := service.CreateVaultBackup(VaultBackupCreateRequest{Payload: sampleVaultPayload("prod"), Reason: "before_restore"})
	if err == nil {
		t.Fatal("expected encryption unavailable")
	}
	caps := service.GetVaultBackupCapabilities()
	if caps.EncryptionAvailable {
		t.Fatal("capabilities must report encryption unavailable")
	}
}

func TestVaultBackupOpenDirCreatesFolder(t *testing.T) {
	root := t.TempDir()
	service := newVaultBackupService(root, credentials.New(&memoryKeyring{}))
	service.openPath = nil
	result, err := service.OpenVaultBackupDir()
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != filepath.Join(root, "vault-backups") {
		t.Fatalf("path: %s", result.Path)
	}
	if _, err := os.Stat(result.Path); err != nil {
		t.Fatal(err)
	}
}
