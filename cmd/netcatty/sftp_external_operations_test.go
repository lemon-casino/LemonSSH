package main

import (
	"github.com/binaricat/netcatty/internal/platform/filesystem"
	"os"
	"path/filepath"
	"testing"
)

func TestExternalOpenManagedTempValidationAndCleanup(t *testing.T) {
	root := t.TempDir()
	temp, err := filesystem.NewTempService(root)
	if err != nil {
		t.Fatal(err)
	}
	s := newFilesystemService()
	s.setTempService(temp)
	target, err := s.TempFilePath("notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ValidateTempFile(target); err == nil {
		t.Fatal("missing download accepted")
	}
	if err := os.WriteFile(target, []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.ValidateTempFile(target); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{outside, root} {
		if err := s.ValidateTempFile(p); err == nil {
			t.Fatalf("accepted %s", p)
		}
		if err := s.DeleteTempFile(p); err == nil {
			t.Fatalf("deleted %s", p)
		}
	}
	if err := s.DeleteTempFile(target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("download remains: %v", err)
	}
	if err := s.DeleteTempFile(target); err != nil {
		t.Fatal(err)
	}
	if err := s.OpenWithSystemDefault(target); err == nil {
		t.Fatal("opened missing file")
	}
	if err := s.OpenWithApplication(outside, filepath.Join(root, "missing.exe")); err == nil {
		t.Fatal("missing application accepted")
	}
}

func TestParseSFTPPermissions(t *testing.T) {
	for text, want := range map[string]os.FileMode{"000": 0, "644": 0644, "0755": 0755, "4755": 0755 | os.ModeSetuid, "2750": 0750 | os.ModeSetgid, "1777": 0777 | os.ModeSticky} {
		got, err := parseSFTPPermissions(text)
		if err != nil || got != want {
			t.Errorf("%s: %v %v, want %v", text, got, err, want)
		}
	}
	for _, text := range []string{"", "77777", "888", "-1", "0x777", " 755", "755x"} {
		if _, err := parseSFTPPermissions(text); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
}
