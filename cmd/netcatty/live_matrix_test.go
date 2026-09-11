// Live verification matrix (SSH-01 / SFTP-01 / NET-01 / TERM-03.3). Skipped
// unless NETCATTY_LIVE_HOST is set; credentials arrive only through the
// environment and are never committed. Run:
//
//	NETCATTY_LIVE_HOST=192.168.0.6 NETCATTY_LIVE_USER=root \
//	NETCATTY_LIVE_PASSWORD='...' go test -count=1 -run Live ./cmd/netcatty/
package main

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgsftp "github.com/pkg/sftp"

	"github.com/binaricat/netcatty/internal/terminal/mosh"
	"github.com/binaricat/netcatty/internal/terminal/ssh"
)

func liveConfig(t *testing.T) (host, user, password string, port uint16) {
	t.Helper()
	host = os.Getenv("NETCATTY_LIVE_HOST")
	user = os.Getenv("NETCATTY_LIVE_USER")
	password = os.Getenv("NETCATTY_LIVE_PASSWORD")
	if host == "" || user == "" || password == "" {
		t.Skip("live matrix requires NETCATTY_LIVE_HOST/USER/PASSWORD")
	}
	port = 22
	return host, user, password, port
}

func liveDial(t *testing.T, host, user, password string, port uint16) *ssh.Transport {
	t.Helper()
	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	knownHosts := ssh.NewKnownHosts(knownHostsPath)
	config, err := ssh.BuildDialConfigErr(ssh.ConnectInput{
		Hostname: host,
		Port:     port,
		Username: user,
		Password: password,
	}, ssh.StrictPolicy(knownHosts), nil)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := ssh.Dial(context.Background(), config)
	if err != nil {
		t.Fatalf("live ssh dial %s@%s:%d: %v", user, host, port, err)
	}
	return transport
}

func TestLiveSSHExecEcho(t *testing.T) {
	host, user, password, port := liveConfig(t)
	transport := liveDial(t, host, user, password, port)
	defer transport.Close()

	session, err := transport.Client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	output, err := session.CombinedOutput("echo live-ok-$((40+2))")
	if err != nil {
		t.Fatalf("live exec: %v (%q)", err, string(output))
	}
	if !strings.Contains(string(output), "live-ok-42") {
		t.Fatalf("unexpected output %q", string(output))
	}
}

func TestLiveSFTPListAndRoundTrip(t *testing.T) {
	host, user, password, port := liveConfig(t)
	transport := liveDial(t, host, user, password, port)
	defer transport.Close()

	session, err := transport.Client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	sftpClient, err := pkgsftp.NewClient(transport.Client)
	if err != nil {
		_ = session.Close()
		t.Fatalf("sftp subsystem: %v", err)
	}
	defer sftpClient.Close()

	entries, err := sftpClient.ReadDir("/")
	if err != nil {
		t.Fatalf("sftp readdir /: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("root listing empty")
	}

	remote := "/tmp/netcatty-live-roundtrip"
	if err := os.WriteFile(filepath.Join(os.TempDir(), "netcatty-live-src"), []byte("roundtrip"), 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(filepath.Join(os.TempDir(), "netcatty-live-src"))
	if err != nil {
		t.Fatal(err)
	}
	destination, err := sftpClient.Create(remote)
	if err != nil {
		t.Fatalf("sftp create: %v", err)
	}
	if _, err := io.Copy(destination, source); err != nil {
		t.Fatalf("sftp write: %v", err)
	}
	_ = destination.Close()
	_ = source.Close()
	readBack, err := sftpClient.Open(remote)
	if err != nil {
		t.Fatalf("sftp open: %v", err)
	}
	content, err := io.ReadAll(readBack)
	_ = readBack.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "roundtrip" {
		t.Fatalf("roundtrip payload %q", content)
	}
	if err := sftpClient.Remove(remote); err != nil {
		t.Fatalf("sftp cleanup: %v", err)
	}
}

func TestLiveRemoteForwardEcho(t *testing.T) {
	host, user, password, port := liveConfig(t)
	transport := liveDial(t, host, user, password, port)
	defer transport.Close()

	listener, err := transport.Client.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("remote forward listen: %v", err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- conn
		}
	}()

	dialSession, err := transport.Client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer dialSession.Close()
	forwardedPort := listener.Addr().(*net.TCPAddr).Port
	// Dial the forwarded port from the remote side so traffic rides the tunnel.
	go func() {
		_ = dialSession.Run("sleep 0.2; exec 3<>/dev/tcp/127.0.0.1/" + itoaLive(forwardedPort) + "; printf fwd-ok >&3; exec 3<&-")
	}()

	select {
	case conn := <-accepted:
		buf := make([]byte, 6)
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, err := io.ReadFull(conn, buf); err != nil {
			t.Fatalf("forwarded read: %v", err)
		}
		if string(buf) != "fwd-ok" {
			t.Fatalf("forwarded payload %q", buf)
		}
		_ = conn.Close()
	case <-time.After(8 * time.Second):
		t.Fatal("no connection arrived through the remote forward")
	}
}

func TestLiveMoshHandshakeParses(t *testing.T) {
	host, user, password, port := liveConfig(t)
	transport := liveDial(t, host, user, password, port)
	defer transport.Close()

	probe, err := transport.Client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	probeOutput, err := probe.CombinedOutput("command -v mosh-server || echo __mosh_missing__")
	_ = probe.Close()
	if err != nil {
		t.Fatalf("probe mosh-server: %v", err)
	}
	if strings.Contains(string(probeOutput), "__mosh_missing__") {
		t.Skip("mosh-server not installed on the live host; install mosh to complete this check")
	}

	bootstrap, err := transport.Client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer bootstrap.Close()
	stdout, err := bootstrap.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.Start("mosh-server new -s -c 256"); err != nil {
		t.Fatalf("start mosh-server: %v", err)
	}

	done := make(chan error, 1)
	var connect mosh.Connect
	go func() {
		buffer := make([]byte, 4096)
		var pending []byte
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			n, readErr := stdout.Read(buffer)
			if n > 0 {
				pending = append(pending, buffer[:n]...)
				if parsed, _, ok, parseErr := mosh.ParseConnect(pending); parseErr == nil && ok {
					connect = parsed
					done <- nil
					return
				}
				if len(pending) > 1<<16 && !mosh.TrailingMarkerPartial(string(pending)) {
					pending = pending[len(pending)-128:]
				}
			}
			if readErr != nil {
				done <- mosh.ErrNoConnectLine
				return
			}
		}
		done <- mosh.ErrNoConnectLine
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("mosh handshake: %v", err)
		}
	case <-time.After(25 * time.Second):
		t.Fatal("mosh handshake timed out")
	}
	if connect.Port == 0 || connect.Key == "" {
		t.Fatal("mosh CONNECT line did not parse")
	}
}

func itoaLive(value int) string {
	if value == 0 {
		return "0"
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
