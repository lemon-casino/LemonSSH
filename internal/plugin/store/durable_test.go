package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Loss of a committed install, or replay of an uncommitted stage, breaks recovery.
func TestDiskStoreRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("a", 64)
	if _, err = s.Install("kept", "1.0.0", hash, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StageInstall("pending", "1.0.0", hash, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	recovered, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := recovered.Get("kept"); !ok {
		t.Fatal("committed install lost")
	}
	if _, ok := recovered.Get("pending"); ok {
		t.Fatal("uncommitted install replayed")
	}
	again, err := Open(path)
	if err != nil || len(again.List()) != 1 {
		t.Fatal("recovery not durable", err)
	}
}

func TestRecoveredBackupSurvivesAnotherInterruptedWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Install("kept", "1.0.0", strings.Repeat("a", 64), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveAtomic(path); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte(`broken`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(path); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.List()) != 1 {
		t.Fatal("good backup overwritten by corrupt primary")
	}
}

func TestSettingDiskFailureRollsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Install("kept", "1.0.0", strings.Repeat("a", 64), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetSetting("kept", "mode", json.RawMessage(`"dark"`)); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path+".tmp", 0700); err != nil {
		t.Fatal(err)
	}
	if err = s.SetSetting("kept", "mode", json.RawMessage(`"light"`)); err == nil {
		t.Fatal("failed disk write accepted")
	}
	record, _ := s.Get("kept")
	if string(record.Settings["mode"]) != `"dark"` {
		t.Fatal("setting mutation leaked after failed disk write")
	}
}

func TestMissingPrimaryCorruptBackupFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.json")
	if err := os.WriteFile(path+".bak", []byte(`broken`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := New().LoadAtomic(path); err == nil {
		t.Fatal("corrupt backup silently treated as fresh store")
	}
}
