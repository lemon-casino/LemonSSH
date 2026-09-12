package main

import (
	"archive/zip"
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
	archive, err := zip.OpenReader(target)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if len(archive.File) != 1 || archive.File[0].Name != "hello.txt" {
		t.Fatal("incorrect zip")
	}
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
