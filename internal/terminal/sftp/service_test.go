package sftp

import (
	"bufio"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/sftp"
)

// inProcessServer spins up a real pkg/sftp server over an in-memory pipe
// serving a temp directory, and returns a client connected to it. This gives
// true request/response coverage without a live SSH host.
func inProcessServer(t *testing.T) (RemoteFS, string) {
	t.Helper()
	root := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("alpha.txt", "alpha-content")
	write("beta.log", strings.Repeat("b", 5000))
	if err := os.Mkdir(filepath.Join(root, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	// One synchronous in-memory pipe: the server reads requests and writes
	// responses on its end; the client uses the other end for both directions.
	serverEnd, clientEnd := net.Pipe()
	server, err := sftp.NewServer(serverEnd, sftp.WithServerWorkingDirectory(root))
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve() }()
	t.Cleanup(func() {
		_ = server.Close()
		_ = serverEnd.Close()
		_ = clientEnd.Close()
	})

	client, err := sftp.NewClientPipe(clientEnd, clientEnd)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return &clientFS{client: client, root: root}, root
}

type clientFS struct {
	client *sftp.Client
	root   string
}

// Paths sent to the client stay relative to the server working directory.
func (fs *clientFS) abs(p string) string { return p }

func (fs *clientFS) ReadDir(dir string) ([]Entry, error) {
	entries, err := fs.client.ReadDir(fs.abs(dir))
	if err != nil {
		return nil, err
	}
	result := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, Entry{
			Name:    entry.Name(),
			IsDir:   entry.IsDir(),
			Size:    entry.Size(),
			ModTime: entry.ModTime(),
			Symlink: entry.Mode()&os.ModeSymlink != 0,
		})
	}
	return result, nil
}

func (fs *clientFS) Stat(path string) (FileInfo, error) {
	info, err := fs.client.Stat(fs.abs(path))
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{Path: path, IsDir: info.IsDir(), Size: info.Size(), ModTime: info.ModTime()}, nil
}

func (fs *clientFS) Mkdir(dir string) error   { return fs.client.Mkdir(fs.abs(dir)) }
func (fs *clientFS) Remove(path string) error { return fs.client.Remove(fs.abs(path)) }
func (fs *clientFS) Rename(oldPath, newPath string) error {
	return fs.client.Rename(fs.abs(oldPath), fs.abs(newPath))
}
func (fs *clientFS) Open(path string) (io.ReadCloser, error) { return fs.client.Open(fs.abs(path)) }
func (fs *clientFS) Create(path string) (io.WriteCloser, error) {
	return fs.client.Create(fs.abs(path))
}

var _ RemoteFS = (*clientFS)(nil)

func TestBrowsingListsAndSorts(t *testing.T) {
	remoteFS, _ := inProcessServer(t)
	entries, err := remoteFS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	SortEntries(entries)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if !entries[0].IsDir || entries[0].Name != "subdir" {
		t.Fatalf("directories must sort first, got %+v", entries[0])
	}
	if entries[1].Name != "alpha.txt" || entries[2].Name != "beta.log" {
		t.Fatalf("file ordering mismatch: %s, %s", entries[1].Name, entries[2].Name)
	}
}

func TestStatAndMutations(t *testing.T) {
	remoteFS, _ := inProcessServer(t)
	info, err := remoteFS.Stat("alpha.txt")
	if err != nil || info.IsDir || info.Size != int64(len("alpha-content")) {
		t.Fatalf("stat mismatch: %+v (%v)", info, err)
	}
	if err := remoteFS.Mkdir("created"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := remoteFS.Rename("created", "renamed"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	writer, err := remoteFS.Create("renamed/new-file.txt")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := writer.Write([]byte("written-through-sftp")); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	reader, err := remoteFS.Open("renamed/new-file.txt")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	if string(content) != "written-through-sftp" {
		t.Fatalf("content mismatch: %s", content)
	}
	if err := remoteFS.Remove("renamed/new-file.txt"); err != nil {
		t.Fatalf("remove: %v", err)
	}
}

func TestReadRange(t *testing.T) {
	remoteFS, _ := inProcessServer(t)
	reader, err := remoteFS.Open("beta.log")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	buffered := bufio.NewReader(reader)
	chunk := make([]byte, 100)
	if _, err := io.ReadFull(buffered, chunk); err != nil {
		t.Fatal(err)
	}
	for _, b := range chunk {
		if b != 'b' {
			t.Fatal("unexpected range content")
		}
	}
}

func TestSessionClientBound(t *testing.T) {
	session := NewSession(2)
	release1, err := session.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	release2, err := session.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.Acquire(); err == nil {
		t.Fatal("third concurrent client must be rejected")
	}
	release1()
	release2()
	release3, err := session.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	release3()
}

func TestNormalizePath(t *testing.T) {
	base := "/home/user"
	if got, err := NormalizePath(base, ""); err != nil || got != base {
		t.Fatalf("empty relative must return base, got %s (%v)", got, err)
	}
	if got, err := NormalizePath(base, "sub/file.txt"); err != nil || got != "/home/user/sub/file.txt" {
		t.Fatalf("join mismatch: %s (%v)", got, err)
	}
	if _, err := NormalizePath(base, "back\\slash"); err == nil {
		t.Fatal("backslash names must be rejected")
	}
	if _, err := NormalizePath(base, "../../etc/passwd"); err == nil {
		t.Fatal("escape attempts must be rejected")
	}
}

func TestNormalizePathAcceptsAbsoluteUnixPaths(t *testing.T) {
	got, err := NormalizePath(".", "/root")
	if err != nil {
		t.Fatalf("/root against dot base must succeed: %v", err)
	}
	if got != "/root" {
		t.Fatalf("got %q", got)
	}
	got, err = NormalizePath(".", "/root/file.txt")
	if err != nil || got != "/root/file.txt" {
		t.Fatalf("absolute child: %q %v", got, err)
	}
	if _, err := NormalizePath(".", "/root/../etc/passwd"); err == nil {
		t.Fatal("absolute paths must still refuse parent escape")
	}
}

func TestReadPartialThenFull(t *testing.T) {
	remoteFS, _ := inProcessServer(t)
	reader, err := remoteFS.Open("alpha.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	first := make([]byte, 5)
	if _, err := io.ReadFull(reader, first); err != nil {
		t.Fatal(err)
	}
	rest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(first)+string(rest) != "alpha-content" {
		t.Fatalf("partial read mismatch: %q + %q", first, rest)
	}
}
