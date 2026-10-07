package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListDirectorySymlinkTargets(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "folder"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"file", "folder", "missing"} {
		if err := os.Symlink(filepath.Join(dir, target), filepath.Join(dir, target+"-link")); err != nil {
			t.Skipf("symlink creation unavailable: %v", err)
		}
	}
	files, err := ListDirectory(dir)
	if err != nil || len(files) != 5 {
		t.Fatalf("listing: %+v, %v", files, err)
	}
	for _, file := range files {
		switch file.Name {
		case "file-link":
			if file.Type != "symlink" || file.LinkTarget == nil || *file.LinkTarget != "file" || file.Size != "5" {
				t.Fatalf("file symlink: %+v", file)
			}
		case "folder-link":
			if file.Type != "symlink" || file.LinkTarget == nil || *file.LinkTarget != "directory" {
				t.Fatalf("directory symlink: %+v", file)
			}
		case "missing-link":
			if file.Type != "symlink" || file.LinkTarget != nil {
				t.Fatalf("broken symlink: %+v", file)
			}
		}
	}
}
