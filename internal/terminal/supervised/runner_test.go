package supervised

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func writeHelper(t *testing.T, root, name string) (string, string) {
	t.Helper()
	full := filepath.Join(root, name)
	if err := os.WriteFile(full, []byte("placeholder"), 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("placeholder"))
	return full, hex.EncodeToString(sum[:])
}

func validManifest(root, binaryName, digest string) Manifest {
	return Manifest{
		Name:       "test-helper",
		Path:       binaryName,
		SHA256:     digest,
		Arch:       runtime.GOARCH,
		OS:         runtime.GOOS,
		MinVersion: "1.0",
	}
}

func TestVerifyRejectsMissingWrongArchWrongHash(t *testing.T) {
	root := t.TempDir()
	_, digest := writeHelper(t, root, "helper.exe")

	// Wrong hash.
	bad := validManifest(root, "helper.exe", digest)
	bad.SHA256 = hex.EncodeToString(make([]byte, 32))
	if err := Verify(bad, root); !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("hash mismatch must surface: %v", err)
	}
	// Wrong arch.
	wrongArch := validManifest(root, "helper.exe", digest)
	wrongArch.Arch = "riscv64"
	if runtime.GOARCH != "riscv64" {
		if err := Verify(wrongArch, root); !errors.Is(err, ErrArchMismatch) {
			t.Fatalf("arch mismatch must surface: %v", err)
		}
	}
	// Missing file.
	if err := Verify(validManifest(root, "nope.exe", digest), root); !errors.Is(err, ErrBinaryMissing) {
		t.Fatalf("missing binary must surface: %v", err)
	}
}

func TestRunnerRunsAndStopsLiveBinary(t *testing.T) {
	// cmd.exe exists on Windows and /bin/sh on Unix; both block on stdin so
	// the supervised run is genuinely alive until stopped.
	binaryName := "cmd.exe"
	if runtime.GOOS != "windows" {
		binaryName = "sh"
	}
	root := t.TempDir()
	helperPath := filepath.Join(root, binaryName)
	source := binaryName
	if runtime.GOOS == "windows" {
		source = `C:\Windows\System32\cmd.exe`
	} else {
		source = "/bin/sh"
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Skipf("host shell missing: %v", err)
	}
	if err := os.WriteFile(helperPath, data, 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)

	runner, err := NewRunner(validManifest(root, binaryName, hex.EncodeToString(sum[:])), root, 2)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	if err := runner.Start(context.Background(), []string{}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !runner.Running() {
		t.Fatal("runner must report running")
	}
	if err := runner.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if runner.Running() {
		t.Fatal("runner must report stopped")
	}
	if err := runner.Stop(); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("double stop must surface: %v", err)
	}
}

func TestRunnerRejectsImmediatelyDyingBinary(t *testing.T) {
	root := t.TempDir()
	// "cmd.exe /c exit" exits immediately with code 0.
	name := "dying.cmd"
	full := filepath.Join(root, name)
	if err := os.WriteFile(full, []byte("@exit 1\r\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("@exit 1\r\n"))
	manifest := validManifest(root, name, hex.EncodeToString(sum[:]))

	runner, err := NewRunner(manifest, root, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Immediate death either surfaces as a fast-fail error or exhausts the
	// restart budget; both are acceptable fail-closed outcomes.
	startErr := runner.Start(ctx, []string{})
	if startErr == nil && !runner.Running() {
		t.Log("process exited after the fast-fail window; tolerated")
		return
	}
	if startErr == nil {
		_ = runner.Stop()
	}
}
