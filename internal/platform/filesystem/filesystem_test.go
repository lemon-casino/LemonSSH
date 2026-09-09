package filesystem

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTempServiceWriteReadRemove(t *testing.T) {
	root := t.TempDir()
	service, err := NewTempService(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.WriteFile("sftp/download/report.txt", []byte("content")); err != nil {
		t.Fatal(err)
	}
	data, err := service.ReadFile("sftp/download/report.txt")
	if err != nil || string(data) != "content" {
		t.Fatalf("read mismatch: %s (%v)", data, err)
	}
	entries, bytesCount, err := service.Usage(context.Background())
	if err != nil || entries != 3 || bytesCount != int64(len("content")) {
		t.Fatalf("usage: %d entries %d bytes (%v)", entries, bytesCount, err)
	}
	if err := service.Remove("sftp/download/report.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReadFile("sftp/download/report.txt"); err == nil {
		t.Fatal("removed file must be gone")
	}
}

func TestTempServiceRefusesEscapes(t *testing.T) {
	root := t.TempDir()
	service, err := NewTempService(root)
	if err != nil {
		t.Fatal(err)
	}
	// Forward-slash traversal escapes on every platform. Drive-letter paths
	// and backslash traversal only escape on Windows.
	escapeNames := []string{"sub/../../../evil"}
	if runtime.GOOS == "windows" {
		escapeNames = append(escapeNames, "C:/Windows/evil", `..\..\..\Windows\evil`)
	}
	for _, name := range escapeNames {
		if _, err := service.FilePath(name); !errors.Is(err, ErrPathEscapesRoot) {
			t.Fatalf("escape %q accepted: %v", name, err)
		}
		if err := service.WriteFile(name, []byte("x")); err == nil {
			t.Fatalf("escape write %q accepted", name)
		}
	}
}

func TestTempRootSubstitutionDetected(t *testing.T) {
	// If the caller swaps the temp root for another managed instance, the old
	// service must not be able to reach the new root's files.
	serviceA, err := NewTempService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := serviceA.WriteFile("secret.txt", []byte("a")); err != nil {
		t.Fatal(err)
	}
	serviceB, err := NewTempService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := serviceB.WriteFile("secret.txt", []byte("b")); err != nil {
		t.Fatal(err)
	}
	data, err := serviceA.ReadFile("secret.txt")
	if err != nil || string(data) != "a" {
		t.Fatalf("root substitution crossed services: %s (%v)", data, err)
	}
}

func TestClearKeepsRoot(t *testing.T) {
	root := t.TempDir()
	service, err := NewTempService(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.WriteFile("a.txt", []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := service.Mkdir("nested"); err != nil {
		t.Fatal(err)
	}
	if err := service.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReadFile("a.txt"); err == nil {
		t.Fatal("clear must empty the root")
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("root must survive clear: %v", err)
	}
}

func TestBrokenSymlinkToleratedAtLeafOnly(t *testing.T) {
	if os.Getenv("SKIP_SYMLINK") == "1" {
		t.Skip("symlinks unavailable")
	}
	root := t.TempDir()
	service, err := NewTempService(root)
	if err != nil {
		t.Fatal(err)
	}
	// Windows requires privilege for symlinks in some setups; tolerate that.
	if err := os.Symlink(filepath.Join(root, "nowhere"), filepath.Join(root, "dangling")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	// The dangling symlink IS the leaf: Remove works through it.
	if err := service.Remove("dangling"); err != nil {
		t.Fatalf("leaf dangling symlink removal: %v", err)
	}
	// A dangling symlink used as an intermediate path component must be
	// refused (it escapes the managed namespace).
	if err := os.Symlink(filepath.Join(root, "nowhere2"), filepath.Join(root, "jump")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.FilePath("jump/evil.txt"); err == nil {
		t.Fatal("dangling intermediate symlink must be refused")
	}
}

func TestExtractArchiveWithZipSlipProtection(t *testing.T) {
	destination := t.TempDir()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)

	safe, err := writer.Create("ok/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = safe.Write([]byte("safe"))
	slip, err := writer.Create(`..\..\evil.txt`)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = slip.Write([]byte("evil"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(t.TempDir(), "upload.zip")
	if err := os.WriteFile(archivePath, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = ExtractArchive(archivePath, destination)
	if err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("zip-slip must abort extraction: %v", err)
	}
	// Safe entries extracted before the abort may exist, but nothing may
	// exist outside the destination root.
	if _, err := os.Stat(filepath.Join(destination, "ok", "file.txt")); err != nil {
		t.Fatalf("safe entry missing: %v", err)
	}
	parent := filepath.Dir(destination)
	entries, _ := os.ReadDir(parent)
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "evil") {
			t.Fatalf("zip-slip wrote outside destination: %s", entry.Name())
		}
	}
}

func TestExtractArchiveCleanRun(t *testing.T) {
	destination := t.TempDir()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	safe, err := writer.Create("docs/readme.md")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = safe.Write([]byte("hello"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "upload.zip")
	if err := os.WriteFile(archivePath, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	extracted, err := ExtractArchive(archivePath, destination)
	if err != nil || extracted != 1 {
		t.Fatalf("extract: %d (%v)", extracted, err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "docs", "readme.md"))
	if err != nil || string(data) != "hello" {
		t.Fatalf("extracted content mismatch: %s (%v)", data, err)
	}
}
