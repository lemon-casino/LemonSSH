package ssh

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// proxyHelperSource is a tiny netcat: it bridges a TCP connection to its own
// stdin/stdout so the test can drive an SSH handshake through the command
// proxy transport without a real external proxy.
const proxyHelperSource = `package main

import (
	"io"
	"net"
	"os"
)

func main() {
	conn, err := net.Dial("tcp", os.Args[1])
	if err != nil {
		os.Exit(1)
	}
	defer conn.Close()
	go func() { _, _ = io.Copy(conn, os.Stdin) }()
	_, _ = io.Copy(os.Stdout, conn)
}
`

// orphanProxyHelperSource exits immediately while an orphaned grandchild keeps
// the shared stdout/stderr pipes open. This reproduces the teardown wedge: a
// child whose exit os/exec's Wait cannot observe, because a descendant holds
// the I/O pipes and starves the stderr copier of EOF. The proxy transport must
// still tear down in bounded time instead of blocking the dial forever. The
// orphan ignores its stdin entirely (the previous code closed stdin first and
// relied on that to unwedge it) and only leaves once the sentinel appears.
const orphanProxyHelperSource = `package main

import (
	"os"
	"os/exec"
	"time"
)

func main() {
	if len(os.Args) > 2 && os.Args[1] == "orphan" {
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(os.Args[2]); err == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		return
	}
	orphan := exec.Command(os.Args[0], "orphan", os.Args[1])
	orphan.Stdout = os.Stdout
	orphan.Stderr = os.Stderr
	if orphan.Start() != nil {
		os.Exit(1)
	}
	// Exit while the orphan keeps the pipes open: Wait must not wait for I/O
	// EOF indefinitely.
	os.Exit(0)
}
`

func buildProxyHelper(t *testing.T) string {
	t.Helper()
	return buildHelperInDir(t, t.TempDir(), proxyHelperSource)
}

func buildHelperInDir(t *testing.T, dir, source string) string {
	t.Helper()
	src := filepath.Join(dir, "helper.go")
	if err := os.WriteFile(src, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "proxyhelper")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, src)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build proxy helper: %v\n%s", err, output)
	}
	return out
}

func TestExpandProxyTokens(t *testing.T) {
	got := expandProxyTokens("connect %h:%p %% host", "example.com", "2222")
	if got != "connect example.com:2222 % host" {
		t.Fatalf("expanded %q", got)
	}
}

func TestDialCommandProxyCarriesSSHHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		// Echo server: the proxy transport must carry application bytes both ways.
		buf := make([]byte, 5)
		if _, readErr := io.ReadFull(conn, buf); readErr != nil {
			return
		}
		_, _ = conn.Write(buf)
		_ = conn.Close()
	}()

	helper := buildProxyHelper(t)
	// Quote only the helper path: quoting the whole command line would make
	// `sh -c` treat "path args" as a single program name (exit 127, not found).
	command := quoteCommandForShell(helper) + " 127.0.0.1:%p"
	conn, err := DialCommandProxy(context.Background(), command, "127.0.0.1:"+itoaPort(listener.Addr().(*net.TCPAddr).Port))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("ping!")); err != nil {
		t.Fatalf("write through proxy: %v", err)
	}
	buf := make([]byte, 5)
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read through proxy: %v", err)
	}
	if string(buf) != "ping!" {
		t.Fatalf("payload %q", buf)
	}
	<-serverDone
}

func TestDialCommandProxyFailsClosedOnEmptyCommand(t *testing.T) {
	if _, err := DialCommandProxy(context.Background(), "   ", "127.0.0.1:22"); err == nil {
		t.Fatal("empty proxy command must fail closed")
	}
}

// TestDialCommandProxyBoundedTeardownWithUnobservableExit is the regression
// guard for the Linux CI wedge: when the proxy child exits but an orphaned
// descendant keeps its I/O pipes open, os/exec's Wait cannot observe the exit
// and previously blocked forever. DialCommandProxy must still return a
// transport, and tearing it down must stay bounded so the SSH handshake
// deadline and transport close can never wedge on the proxy.
func TestDialCommandProxyBoundedTeardownWithUnobservableExit(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "orphan.done")
	// The orphan keeps the helper's executable image locked while it sleeps on
	// Windows, so the sentinel-driven cleanup below removes the directory with
	// retries before t.TempDir's own cleanup runs (cleanups execute last in,
	// first out).
	t.Cleanup(func() {
		if err := os.WriteFile(sentinel, nil, 0o600); err != nil {
			t.Logf("write orphan sentinel: %v", err)
		}
		// The orphan polls for the sentinel every 50ms; give it a few cycles
		// to exit before removing the directory tree that holds the sentinel.
		time.Sleep(250 * time.Millisecond)
		deadline := time.Now().Add(30 * time.Second)
		for {
			err := os.RemoveAll(dir)
			if err == nil || time.Now().After(deadline) {
				if err != nil {
					t.Logf("orphan helper cleanup: %v", err)
				}
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
	helper := buildHelperInDir(t, dir, orphanProxyHelperSource)
	command := quoteCommandForShell(helper) + " " + quoteCommandForShell(sentinel)
	conn, err := DialCommandProxy(context.Background(), command, "127.0.0.1:22")
	if err != nil {
		t.Fatalf("unobservable child exit must still yield a transport: %v", err)
	}
	start := time.Now()
	_ = conn.Close()
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("teardown took %s; proxy child teardown must stay bounded", elapsed)
	}
}

func TestDialCommandProxyFailsFastOnDeadCommand(t *testing.T) {
	dead := "definitely-not-a-real-binary-3f9a1c"
	if runtime.GOOS == "windows" {
		dead += ".exe"
	}
	if _, err := DialCommandProxy(context.Background(), dead, "127.0.0.1:22"); err == nil {
		t.Fatal("missing proxy binary must fail fast")
	}
}

func itoaPort(port int) string {
	if port == 0 {
		return "0"
	}
	var digits []byte
	for port > 0 {
		digits = append([]byte{byte('0' + port%10)}, digits...)
		port /= 10
	}
	return string(digits)
}

// quoteCommandForShell quotes an executable path so the platform's shell (or
// splitCommand on the direct-exec Windows path) treats it as one word even
// when the temp directory contains spaces. Arguments must stay outside the
// quotes: they belong to the command, not the program name.
func quoteCommandForShell(path string) string {
	if runtime.GOOS == "windows" {
		return "\"" + path + "\""
	}
	return "'" + path + "'"
}
