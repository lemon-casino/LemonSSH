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

func buildProxyHelper(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "helper.go")
	if err := os.WriteFile(src, []byte(proxyHelperSource), 0o600); err != nil {
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
	command := quoteCommandForShell(helper + " 127.0.0.1:%p")
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

func quoteCommandForShell(command string) string {
	if runtime.GOOS == "windows" {
		return command
	}
	return "'" + command + "'"
}
