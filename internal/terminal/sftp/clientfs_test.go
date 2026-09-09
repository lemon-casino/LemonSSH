package sftp

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	pkgsftp "github.com/pkg/sftp"
)

// clientFSOverInProcessServer dials a real pkg/sftp client against a real
// pkg/sftp server over net.Pipe, then wraps it in the production ClientFS
// adapter. This exercises the wire protocol, not a fake.
func clientFSOverInProcessServer(t *testing.T) (*ClientFS, string) {
	t.Helper()
	root := t.TempDir()
	serverConn, clientConn := net.Pipe()
	server, err := pkgsftp.NewServer(serverConn, pkgsftp.WithServerWorkingDirectory(root))
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve() }()
	t.Cleanup(func() {
		_ = serverConn.Close()
	})
	client, err := pkgsftp.NewClientPipe(clientConn, clientConn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})
	return NewClientFS(client), root
}

func TestClientFSRoundTrip(t *testing.T) {
	fs, root := clientFSOverInProcessServer(t)

	// Create + write.
	writer, err := fs.Create("hello.txt")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	payload := []byte("remote payload")
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	// Read back.
	reader, err := fs.Open("hello.txt")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("round trip mismatch: %q", got)
	}

	// Stat.
	info, err := fs.Stat("hello.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.IsDir || info.Size != int64(len(payload)) {
		t.Fatalf("unexpected stat: %+v", info)
	}

	// ReadDir via a subdirectory.
	if err := fs.Mkdir("sub"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	entries, err := fs.ReadDir(".")
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	names := map[string]bool{}
	for _, entry := range entries {
		names[entry.Name] = true
		if entry.Name == "sub" && !entry.IsDir {
			t.Fatal("sub must be reported as dir")
		}
	}
	if !names["hello.txt"] || !names["sub"] {
		t.Fatalf("listing incomplete: %v", names)
	}

	// Rename, then tree removal.
	if err := fs.Rename("hello.txt", "sub/renamed.txt"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "sub", "renamed.txt")); err != nil {
		t.Fatalf("renamed file missing on disk: %v", err)
	}
	writer2, err := fs.Create("sub/inner.txt")
	if err != nil {
		t.Fatal(err)
	}
	_ = writer2.Close()
	if err := fs.Remove("sub"); err != nil {
		t.Fatalf("recursive remove: %v", err)
	}
	entries, err = fs.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("tree not removed: %+v", entries)
	}
}
