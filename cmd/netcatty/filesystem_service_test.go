package main

import (
	"github.com/binaricat/netcatty/internal/platform/filesystem"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalBrowseReturnsRealUploadSources(t *testing.T) {
	service := newFilesystemService()
	managedTemp, tempErr := filesystem.NewTempService(t.TempDir())
	if tempErr != nil {
		t.Fatal(tempErr)
	}
	service.setTempService(managedTemp)
	home, err := service.HomeDir()
	if err != nil || home == "" {
		t.Fatalf("home directory: %q, %v", home, err)
	}
	wantHome, err := os.UserHomeDir()
	if err != nil || home != wantHome {
		t.Fatalf("home = %q, want %q, %v", home, wantHome, err)
	}
	dir := t.TempDir()
	empty, err := service.ListDir(dir)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty directory must return [], got %+v, %v", empty, err)
	}
	if err := os.Mkdir(filepath.Join(dir, "Documents"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.pdf"), []byte("PDFDATA"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := service.ListDir(dir)
	if err != nil || len(files) != 2 {
		t.Fatalf("directory: %+v, %v", files, err)
	}
	if files[0].Name != "Documents" || files[0].Type != "directory" || files[1].Name != "report.pdf" || files[1].Type != "file" || files[1].Size != "7" || files[1].LastModified == "" {
		t.Fatalf("listing: %+v", files)
	}
	reader, err := openLocalForUpload(filepath.Join(dir, files[1].Name))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil || string(data) != "PDFDATA" {
		t.Fatalf("upload source: %q, %v", data, err)
	}
	if _, err := service.ListDir(filepath.Join(dir, "missing")); !os.IsNotExist(err) {
		t.Fatalf("missing directory must report its error, got %v", err)
	}
}

func TestStatPathClassifiesFilesAndDirectories(t *testing.T) {
	service := newFilesystemService()
	managedTemp, tempErr := filesystem.NewTempService(t.TempDir())
	if tempErr != nil {
		t.Fatal(tempErr)
	}
	service.setTempService(managedTemp)
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
	managedTemp, tempErr := filesystem.NewTempService(t.TempDir())
	if tempErr != nil {
		t.Fatal(tempErr)
	}
	service.setTempService(managedTemp)
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
	managedTemp, tempErr := filesystem.NewTempService(t.TempDir())
	if tempErr != nil {
		t.Fatal(tempErr)
	}
	service.setTempService(managedTemp)
	if err := service.StageAppend(filepath.Join(t.TempDir(), "evil.txt"), 0, []byte("x")); err == nil {
		t.Fatal("non-staging path must be rejected")
	}
	if err := service.StageDiscard(filepath.Join(t.TempDir(), "evil.txt")); err == nil {
		t.Fatal("non-staging discard must be rejected")
	}
}

func TestStageFromLocalPathCopiesImmediately(t *testing.T) {
	service := newFilesystemService()
	managedTemp, tempErr := filesystem.NewTempService(t.TempDir())
	if tempErr != nil {
		t.Fatal(tempErr)
	}
	service.setTempService(managedTemp)
	dir := t.TempDir()
	file := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(file, []byte("PDFDATA"), 0o600); err != nil {
		t.Fatal(err)
	}
	staged, size, err := service.StageFromLocalPath(file)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(staged)
	if size != 7 {
		t.Fatalf("size = %d", size)
	}
	data, err := os.ReadFile(staged)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "PDFDATA" {
		t.Fatalf("content = %q", data)
	}
	if _, _, err := service.StageFromLocalPath(filepath.Join(dir, "missing.pdf")); err == nil {
		t.Fatal("missing source must fail")
	}
}
