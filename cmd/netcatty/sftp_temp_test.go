package main

import (
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/platform/filesystem"
	pkgsftp "github.com/pkg/sftp"
)

type tempBlockingConn struct {
	net.Conn
	armed   atomic.Bool
	entered chan struct{}
	resume  chan struct{}
}

func (c *tempBlockingConn) Write(data []byte) (int, error) {
	if c.armed.Swap(false) {
		close(c.entered)
		<-c.resume
	}
	return c.Conn.Write(data)
}

func TestSFTPDownloadPinSurvivesRendererReleaseAndClear(t *testing.T) {
	temp, err := filesystem.NewTempService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	remoteRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(remoteRoot, "source.txt"), []byte("download complete"), 0600); err != nil {
		t.Fatal(err)
	}
	serverConn, clientConn := net.Pipe()
	blocked := &tempBlockingConn{Conn: serverConn, entered: make(chan struct{}), resume: make(chan struct{})}
	resume := sync.OnceFunc(func() { close(blocked.resume) })
	t.Cleanup(resume)
	server, err := pkgsftp.NewServer(blocked, pkgsftp.WithServerWorkingDirectory(remoteRoot))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		resume()
		_ = server.Close()
	})
	go func() { _ = server.Serve() }()
	raw, err := pkgsftp.NewClientPipe(clientConn, clientConn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		resume()
		_ = raw.Close()
	})
	service := NewSFTPService(nil, nil)
	service.setTempService(temp)
	service.core.SeedClientSessionForTest("test", raw)
	fs := &FilesystemService{temp: temp}
	path, err := fs.TempFilePath("download.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	blocked.armed.Store(true)
	done := make(chan error, 1)
	go func() {
		remotePath := "/" + filepath.ToSlash(filepath.Join(remoteRoot, "source.txt"))
		_, err := service.Download("test", remotePath, path)
		done <- err
	}()
	select {
	case <-blocked.entered:
	case err := <-done:
		t.Fatalf("download ended before reaching SFTP server: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("download did not reach SFTP server")
	}
	if err := fs.ReleaseTempFile(path); err != nil {
		t.Fatal(err)
	}
	if result, err := fs.ClearTemp(); err != nil || result.DeletedCount != 0 {
		t.Fatalf("clear during download: %+v, %v", result, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("in-flight download removed: %v", err)
	}
	resume()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("download did not finish")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "download complete" {
		t.Fatalf("download: %q, %v", data, err)
	}
	if result, err := fs.ClearTemp(); err != nil || result.DeletedCount != 1 {
		t.Fatalf("download pin leaked: %+v, %v", result, err)
	}
}

func TestSFTPArchiveFailuresCleanManagedTemp(t *testing.T) {
	for _, operation := range []string{"extract-download", "compress", "compressed-upload"} {
		t.Run(operation, func(t *testing.T) {
			temp, err := filesystem.NewTempService(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			service := NewSFTPService(nil, nil)
			service.setTempService(temp)
			folder := t.TempDir()
			if err := os.WriteFile(filepath.Join(folder, "file.txt"), []byte("upload"), 0600); err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "extract-download":
				_, err = service.ExtractArchive("missing", "/archive.zip")
			case "compress":
				_, err = service.UploadCompressedFolder("missing", filepath.Join(folder, "missing"), "/archive.zip")
			case "compressed-upload":
				_, err = service.UploadCompressedFolder("missing", folder, "/archive.zip")
			}
			if err == nil {
				t.Fatal("expected archive operation to fail")
			}
			entries, err := os.ReadDir(temp.Root())
			if err != nil || len(entries) != 0 {
				t.Fatalf("archive failure leaked temp: %v, %v", entries, err)
			}
		})
	}
}
