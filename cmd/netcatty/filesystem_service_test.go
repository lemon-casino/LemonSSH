package main

import (
	"errors"
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

func TestStageOperationsRequireOwnedContainedFile(t *testing.T) {
	temp, err := filesystem.NewTempService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := &FilesystemService{temp: temp}
	stage, err := service.StageBegin("owned.txt")
	if err != nil {
		t.Fatal(err)
	}
	unowned := filepath.Join(temp.Root(), filesystem.StagedUploadPrefix+"unowned")
	embedded := filepath.Join(temp.Root(), "not-"+filesystem.StagedUploadPrefix+"owned")
	outside := filepath.Join(t.TempDir(), filepath.Base(stage))
	nested := filepath.Join(temp.Root(), "nested", filepath.Base(stage))
	if err := os.Mkdir(filepath.Dir(nested), 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{unowned, embedded, outside, nested} {
		if err := os.WriteFile(path, []byte("preserve"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := service.StageAppend(path, 0, []byte("damage")); !errors.Is(err, filesystem.ErrNotStagedFile) {
			t.Fatalf("append accepted %s: %v", path, err)
		}
		if err := service.StageDiscard(path); !errors.Is(err, filesystem.ErrNotStagedFile) {
			t.Fatalf("discard accepted %s: %v", path, err)
		}
		if data, err := os.ReadFile(path); err != nil || string(data) != "preserve" {
			t.Fatalf("unowned path modified: %q, %v", data, err)
		}
	}
	if err := service.StageAppend(stage, -1, []byte("bad offset")); err == nil {
		t.Fatal("negative offset accepted")
	}
	// Keeping the original file under a different name avoids inode reuse.
	if err := os.Rename(stage, stage+".original"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stage, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := service.StageAppend(stage, 0, []byte("bad")); err == nil {
		t.Fatal("append accepted replacement file")
	}
	if err := service.StageDiscard(stage); err == nil {
		t.Fatal("discard accepted replacement file")
	}
}

func TestStageOperationsRejectSymlinkReplacement(t *testing.T) {
	temp, err := filesystem.NewTempService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := &FilesystemService{temp: temp}
	stage, err := service.StageBegin("link.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(stage, stage+".original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(stage+".original", stage); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := service.StageAppend(stage, 0, []byte("bad")); err == nil {
		t.Fatal("append followed symlink")
	}
	if err := service.StageDiscard(stage); err == nil {
		t.Fatal("discard accepted symlink")
	}
}

func TestClearTempProtectsExternalDownloadUntilReleased(t *testing.T) {
	temp, err := filesystem.NewTempService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := &FilesystemService{temp: temp}
	path, err := service.TempFilePath("download.txt")
	if err != nil {
		t.Fatal(err)
	}
	pin, err := temp.Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := service.ReleaseTempFile(path); err != nil {
		t.Fatal(err)
	}
	if result, err := service.ClearTemp(); err != nil || !result.Success || result.DeletedCount != 0 {
		t.Fatalf("clear while downloading: %+v, %v", result, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("active download deleted: %v", err)
	}
	pin()
	if result, err := service.ClearTemp(); err != nil || !result.Success || result.DeletedCount != 1 {
		t.Fatalf("clear after release: %+v, %v", result, err)
	}
	// Failed downloads can be deleted before a destination file was created.
	missing, err := service.TempFilePath("failed.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteTempFile(missing); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(missing, []byte("orphan"), 0600); err != nil {
		t.Fatal(err)
	}
	if result, err := service.ClearTemp(); err != nil || result.DeletedCount != 1 {
		t.Fatalf("failed download reservation leaked: %+v, %v", result, err)
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
