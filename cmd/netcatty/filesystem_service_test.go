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

func TestStageBeginAppendDiscardRoundTrip(t *testing.T) {
	service := newFilesystemService()
	tempPath, err := service.StageBegin("notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempPath)
	if filepath.Base(tempPath) == "notes.txt" {
		t.Fatal("staged path must be temp, not caller-controlled name")
	}
	if err := service.StageAppend(tempPath, 0, []byte("hello ")); err != nil {
		t.Fatal(err)
	}
	if err := service.StageAppend(tempPath, 6, []byte("world")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(tempPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Fatalf("content = %q", data)
	}
	if err := service.StageDiscard(tempPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Fatal("staged file must be removed")
	}
}

func TestStageAppendRejectsNonStagingPaths(t *testing.T) {
	service := newFilesystemService()
	if err := service.StageAppend(filepath.Join(t.TempDir(), "evil.txt"), 0, []byte("x")); err == nil {
		t.Fatal("non-staging path must be rejected")
	}
	if err := service.StageDiscard(filepath.Join(t.TempDir(), "evil.txt")); err == nil {
		t.Fatal("non-staging discard must be rejected")
	}
}
