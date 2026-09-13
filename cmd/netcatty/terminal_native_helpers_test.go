package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/platform/filesystem"
	"github.com/binaricat/netcatty/internal/terminal/pty"
	"github.com/binaricat/netcatty/internal/terminal/supervised"
)

func verifiedTestHelper(t *testing.T, kind string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "resources", kind, helperPlatformDir(), helperBinaryName(kind)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); os.IsNotExist(err) {
		t.Skip("bundled " + kind + " unavailable")
	}
	pin, err := os.ReadFile(path + ".manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest supervised.Manifest
	if err = json.Unmarshal(pin, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Path = filepath.Base(path)
	if err = supervised.Verify(manifest, filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	return path
}

func captureNativeOutput(local *pty.Session) (func() string, <-chan struct{}) {
	var mu sync.Mutex
	var output bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := local.ReadOnce(buf)
			if n > 0 {
				mu.Lock()
				if output.Len() < 64*1024 {
					output.Write(buf[:n])
				}
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	return func() string { mu.Lock(); defer mu.Unlock(); return output.String() }, done
}

// This intentionally stops before encrypted ET traffic: the remote fixture
// implements SSH bootstrap and a byte tunnel, not an invented ET wire peer.
func TestETBundledClientConsumesGoBootstrap(t *testing.T) {
	path := verifiedTestHelper(t, "et")
	transport, _, _ := etRemoteFixture(t)
	temp, err := filesystem.NewTempService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := newEtBridge(context.Background(), transport, MoshStartRequest{}, temp, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()
	local := pty.NewSession(pty.Config{Shell: path, Args: bridge.args, CWD: bridge.directory, Env: helperEnvironment(bridge.env)})
	if err = local.Start(context.Background(), pty.NewPlatformBackend()); err != nil {
		t.Fatal(err)
	}
	output, drained := captureNativeOutput(local)
	defer func() { local.Close(); <-drained }()
	exit := make(chan error, 1)
	go func() { exit <- local.Wait() }()
	select {
	case <-bridge.ready:
	case err := <-exit:
		t.Fatalf("bundled ET exited before handoff: %v %s", err, output())
	case <-time.After(15 * time.Second):
		t.Fatalf("bundled ET did not consume handoff: %s", output())
	}
}

func TestMoshBundledRestartResetsNonce(t *testing.T) {
	path := verifiedTestHelper(t, "mosh")
	var nonces [2][]byte
	for attempt := 0; attempt < 2; attempt++ {
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		local := pty.NewSession(pty.Config{Shell: path, Args: []string{"127.0.0.1", strconv.Itoa(conn.LocalAddr().(*net.UDPAddr).Port)}, Env: helperEnvironment(map[string]string{"MOSH_KEY": "AAAAAAAAAAAAAAAAAAAAAA", "TERM": "xterm-256color"})})
		if err = local.Start(context.Background(), pty.NewPlatformBackend()); err != nil {
			conn.Close()
			t.Fatal(err)
		}
		output, drained := captureNativeOutput(local)
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		buf := make([]byte, 4096)
		n, _, err := conn.ReadFromUDP(buf)
		local.Close()
		conn.Close()
		<-drained
		if err != nil || n < 24 {
			t.Fatalf("no Mosh wire packet: %v %s", err, output())
		}
		nonces[attempt] = append([]byte(nil), buf[:8]...)
	}
	if !bytes.Equal(nonces[0], nonces[1]) {
		t.Fatal("bundled client nonce behavior changed; reevaluate process-resume contract")
	}
}
