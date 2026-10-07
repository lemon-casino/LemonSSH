package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSystemKnownHostsFilesCoverKnownHostsVariants(t *testing.T) {
	files := systemKnownHostsFiles(t.TempDir())
	if len(files) != 2 {
		t.Fatalf("scan targets: %v", files)
	}
	if filepath.Base(files[0]) != "known_hosts" || filepath.Base(files[1]) != "known_hosts2" {
		t.Fatalf("scan targets: %v", files)
	}
}

func TestReadSystemKnownHostsMissingFilesReturnEmpty(t *testing.T) {
	content, err := readSystemKnownHosts(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// No files written: the read must report "nothing found" without an error
	// so the renderer can tell an empty scan from an unavailable bridge.
	if content != "" {
		t.Fatalf("expected empty scan, got %q", content)
	}
}

func TestReadSystemKnownHostsSkipsMissingSecondFile(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "known_hosts"), []byte("host ssh-ed25519 AAAA\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	content, err := readSystemKnownHosts(home)
	if err != nil {
		t.Fatal(err)
	}
	if content != "host ssh-ed25519 AAAA\n" {
		t.Fatalf("scan content %q", content)
	}
}

func TestReadSystemKnownHostsConcatenatesAndSeparates(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "known_hosts"), []byte("a ssh-ed25519 AAAA"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "known_hosts2"), []byte("b ssh-rsa BBBB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	content, err := readSystemKnownHosts(home)
	if err != nil {
		t.Fatal(err)
	}
	if content != "a ssh-ed25519 AAAA\nb ssh-rsa BBBB\n" {
		t.Fatalf("scan content %q", content)
	}
}
