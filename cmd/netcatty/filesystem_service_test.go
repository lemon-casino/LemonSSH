package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStatPathClassifiesFilesAndDirectories(t *testing.T) {
	service := newFilesystemService()
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	stat, err := service.StatPath(file)
	if err != nil {
		t.Fatal(err)
	}
	if stat.IsDir || stat.Name != "a.txt" || stat.Size != 5 {
		t.Fatalf("file stat: %+v", stat)
	}
	dirStat, err := service.StatPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !dirStat.IsDir {
		t.Fatalf("dir stat: %+v", dirStat)
	}
	if _, err := service.StatPath(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing path must fail")
	}
}
