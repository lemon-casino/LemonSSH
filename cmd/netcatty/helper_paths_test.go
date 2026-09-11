package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveHelperBinaryPrefersExplicitThenEnvThenRepo(t *testing.T) {
	root := t.TempDir()
	name := helperBinaryName("mosh")
	explicit := filepath.Join(root, "explicit", name)
	envBin := filepath.Join(root, "env", name)
	repoBin := filepath.Join(root, "resources", "mosh", helperPlatformDir(), name)
	for _, path := range []string{explicit, envBin, repoBin} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	got, err := resolveHelperBinary("mosh", explicit, filepath.Dir(envBin), "", root)
	if err != nil {
		t.Fatal(err)
	}
	if got != explicit {
		t.Fatalf("explicit path first: %s", got)
	}

	got, err = resolveHelperBinary("mosh", "", filepath.Dir(envBin), "", root)
	if err != nil {
		t.Fatal(err)
	}
	if got != envBin {
		t.Fatalf("env root next: %s", got)
	}

	got, err = resolveHelperBinary("mosh", "", "", "", root)
	if err != nil {
		t.Fatal(err)
	}
	if got != repoBin {
		t.Fatalf("repo layout last: %s", got)
	}
}

func TestResolveHelperBinaryFailsClosedWhenMissing(t *testing.T) {
	_, err := resolveHelperBinary("mosh", "", "", "", t.TempDir())
	if err == nil {
		t.Fatal("missing helper must fail closed")
	}
}

func TestHelperPlatformDirMatchesHost(t *testing.T) {
	dir := helperPlatformDir()
	switch runtime.GOOS {
	case "windows":
		if dir != "win32-x64" && dir != "win32-arm64" {
			t.Fatalf("windows dir %s", dir)
		}
	case "darwin":
		if dir != "darwin-universal" {
			t.Fatalf("darwin dir %s", dir)
		}
	case "linux":
		if dir != "linux-x64" && dir != "linux-arm64" {
			t.Fatalf("linux dir %s", dir)
		}
	}
}
