package filesystem

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestListDirectoryWindowsHiddenAttribute(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hidden.txt")
	if err := os.WriteFile(path, []byte("hidden"), 0o600); err != nil {
		t.Fatal(err)
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.SetFileAttributes(name, syscall.FILE_ATTRIBUTE_HIDDEN); err != nil {
		t.Fatal(err)
	}
	files, err := ListDirectory(dir)
	if err != nil || len(files) != 1 || !files[0].Hidden {
		t.Fatalf("hidden file: %+v, %v", files, err)
	}
}
