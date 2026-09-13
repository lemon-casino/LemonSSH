package main

import (
	"archive/zip"
	"context"
	"github.com/binaricat/netcatty/internal/platform/filesystem"
	"github.com/binaricat/netcatty/internal/terminal/transfer"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCompressedTransferStagesRealZipAndUsesScheduler(t *testing.T) {
	folder := t.TempDir()
	target := filepath.Join(t.TempDir(), "out.zip")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, "hello.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	managed, err := filesystem.NewTempService(root)
	if err != nil {
		t.Fatal(err)
	}
	service := newTransferService()
	service.setTempService(managed)
	first, err := service.StartCompressed(TransferStartRequest{TaskID: "zip", SourcePath: folder, TargetPath: target})
	if err != nil {
		t.Fatal(err)
	}
	if first.Phase != "compressing" || first.ControlKind != "compressed-upload" {
		t.Fatalf("bad initial snapshot %+v", first)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		p, err := service.Progress("zip")
		if err != nil {
			t.Fatal(err)
		}
		if p.State == transfer.StateFailed || time.Now().After(deadline) {
			t.Fatalf("failed %+v", p)
		}
		if p.State == transfer.StateCompleted {
			if p.Phase != "uploading" || p.DoneBytes == 0 {
				t.Fatalf("no upload progress %+v", p)
			}
			break
		}
		time.Sleep(time.Millisecond)
	}
	waitForTempEntries(t, root, 0)
	archive, err := zip.OpenReader(target)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if len(archive.File) != 1 || archive.File[0].Name != "hello.txt" {
		t.Fatal("incorrect zip")
	}
}

func waitForTempEntries(t *testing.T, root string, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("temp entries: %d, want %d", len(entries), want)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestCompressedTransferPausedCleanupAndCancellation(t *testing.T) {
	temp, err := filesystem.NewTempService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := newTransferService()
	service.setTempService(temp)
	request := TransferStartRequest{TaskID: "paused", SourcePath: t.TempDir(), TargetPath: filepath.Join(t.TempDir(), "out.zip")}
	job := &compressedTransfer{phase: "compressing", progress: transfer.Progress{TaskID: request.TaskID, State: transfer.StatePaused}}
	service.compressed = map[string]*compressedTransfer{request.TaskID: job}
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.compressAndStart(request, job)
	}()
	t.Cleanup(func() {
		_ = service.Cancel(request.TaskID)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("compression did not stop")
		}
	})
	waitForTempEntries(t, temp.Root(), 1)
	if count, err := temp.ClearInactive(); err != nil || count != 0 {
		t.Fatalf("clear deleted paused compression: %d, %v", count, err)
	}
	if err := service.Cancel(request.TaskID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("compression cancellation timed out")
	}
	waitForTempEntries(t, temp.Root(), 0)
}

func TestCompressedTransferFailureCleansTemp(t *testing.T) {
	for _, failure := range []string{"compression", "upload"} {
		t.Run(failure, func(t *testing.T) {
			temp, err := filesystem.NewTempService(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			service := newTransferService()
			service.setTempService(temp)
			source := t.TempDir()
			if failure == "compression" {
				source = filepath.Join(source, "missing")
			}
			request := TransferStartRequest{TaskID: failure, SourcePath: source, TargetSessionID: "missing-session", TargetPath: "/out.zip"}
			job := &compressedTransfer{phase: "compressing", progress: transfer.Progress{TaskID: failure, State: transfer.StateRunning}}
			service.compressAndStart(request, job)
			if job.progress.State != transfer.StateFailed {
				t.Fatalf("expected failure: %+v", job.progress)
			}
			waitForTempEntries(t, temp.Root(), 0)
		})
	}
}

func TestTransferCancellationDefersStageDiscardUntilFilesClose(t *testing.T) {
	temp, err := filesystem.NewTempService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fs := &FilesystemService{temp: temp}
	stage, err := fs.StageBegin("upload.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.StageAppend(stage, 0, []byte("upload bytes")); err != nil {
		t.Fatal(err)
	}
	service := newTransferService()
	service.setTempService(temp)
	// Block worker admission deterministically while local handles stay open.
	service.scheduler = transfer.New(transfer.WithHostConcurrency(0))
	_, err = service.Start(TransferStartRequest{TaskID: "cancel", SourcePath: stage, TargetPath: filepath.Join(t.TempDir(), "target")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Cancel("cancel") })
	if err := service.Pause("cancel"); err != nil {
		t.Fatal(err)
	}
	if err := fs.StageDiscard(stage); err != nil {
		t.Fatal(err)
	}
	if result, err := fs.ClearTemp(); err != nil || result.DeletedCount != 0 {
		t.Fatalf("clear during paused upload: %+v, %v", result, err)
	}
	if _, err := os.Stat(stage); err != nil {
		t.Fatalf("discard deleted active upload: %v", err)
	}
	if err := service.Cancel("cancel"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := service.scheduler.Wait(ctx, "cancel"); err != nil {
		t.Fatal(err)
	}
	waitForTempEntries(t, temp.Root(), 0)
}

func TestTransferStartFailureReleasesTempPins(t *testing.T) {
	temp, err := filesystem.NewTempService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fs := &FilesystemService{temp: temp}
	stage, err := fs.StageBegin("failure.txt")
	if err != nil {
		t.Fatal(err)
	}
	service := newTransferService()
	service.setTempService(temp)
	_, err = service.Start(TransferStartRequest{TaskID: "fail", SourcePath: stage, TargetPath: filepath.Join(t.TempDir(), "missing", "target")})
	if err == nil {
		t.Fatal("upload unexpectedly started")
	}
	if err := fs.StageDiscard(stage); err != nil {
		t.Fatal(err)
	}
	waitForTempEntries(t, temp.Root(), 0)
}

func TestTransferStartCopiesAndTruncatesDestination(t *testing.T) {
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source"), filepath.Join(dir, "target")
	if err := os.WriteFile(source, []byte("actual bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, make([]byte, 100), 0600); err != nil {
		t.Fatal(err)
	}
	service := newTransferService()
	_, err := service.Start(TransferStartRequest{TaskID: "copy", SourcePath: source, TargetPath: target})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		p, err := service.Progress("copy")
		if err != nil {
			t.Fatal(err)
		}
		if p.State == transfer.StateCompleted {
			break
		}
		if p.State == transfer.StateFailed || time.Now().After(deadline) {
			t.Fatalf("transfer not complete: %+v", p)
		}
		time.Sleep(time.Millisecond)
	}
	snapshots, err := service.List()
	if err != nil || len(snapshots) != 1 {
		t.Fatalf("reload snapshots: %v %v", snapshots, err)
	}
	if snapshots[0].SourcePath != source || snapshots[0].TargetPath != target || snapshots[0].State != transfer.StateCompleted {
		t.Fatalf("reload snapshot lost identity/completion: %+v", snapshots[0])
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "actual bytes" {
		t.Fatalf("copied %q, %v", data, err)
	}
}

func TestTransferStartDoesNotTruncateSource(t *testing.T) {
	file := filepath.Join(t.TempDir(), "same")
	_ = os.WriteFile(file, []byte("preserved"), 0600)
	_, err := newTransferService().Start(TransferStartRequest{TaskID: "same", SourcePath: file, TargetPath: file})
	if err == nil {
		t.Fatal("same source/destination accepted")
	}
	data, _ := os.ReadFile(file)
	if string(data) != "preserved" {
		t.Fatal("source destroyed")
	}
}
