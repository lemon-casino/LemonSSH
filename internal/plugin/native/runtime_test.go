package native

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/plugin/permissions"
)

func helperBinary(t *testing.T, source string) (path, digest string) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "helper.go")
	if err := os.WriteFile(src, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "helper")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, src)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v\n%s", err, output)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return out, hex.EncodeToString(sum[:])
}

const echoHelper = `package main
import (
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
)
func main() {
	for {
		var header [4]byte
		if _, err := io.ReadFull(os.Stdin, header[:]); err != nil { return }
		n := binary.BigEndian.Uint32(header[:])
		body := make([]byte, n)
		if _, err := io.ReadFull(os.Stdin, body); err != nil { return }
		var req struct { ID uint64 ` + "`json:\"id\"`" + `; Method string ` + "`json:\"method\"`" + ` }
		_ = json.Unmarshal(body, &req)
		resp, _ := json.Marshal(map[string]any{"id": req.ID, "result": json.RawMessage(` + "`\"ok\"`" + `)})
		var out [4]byte
		binary.BigEndian.PutUint32(out[:], uint32(len(resp)))
		_, _ = os.Stdout.Write(out[:])
		_, _ = os.Stdout.Write(resp)
	}
}
`

const floodHelper = `package main
import "os"
func main() {
	buf := make([]byte, 1024)
	for i := range buf { buf[i] = 'A' }
	for { _, _ = os.Stdout.Write(buf) }
}
`

func TestStartStopLifecycle(t *testing.T) {
	path, digest := helperBinary(t, echoHelper)
	r := NewRuntime(nil, 4)
	if err := r.Start(context.Background(), Spec{PluginID: "p1", BinaryPath: path, SHA256: digest}); err != nil {
		t.Fatal(err)
	}
	if !r.IsRunning("p1") {
		t.Fatal("must be running")
	}
	if err := r.Stop("p1"); err != nil && !errors.Is(err, ErrNotRunning) {
		// A helper that exits cleanly on stdin close is success.
		t.Log(err)
	}
	if r.IsRunning("p1") {
		t.Fatal("must be stopped")
	}
}

func TestAuthorizeGate(t *testing.T) {
	path, digest := helperBinary(t, echoHelper)
	r := NewRuntime(func(pluginID string) error {
		return errors.New("not authorized for " + pluginID)
	}, 4)
	err := r.Start(context.Background(), Spec{PluginID: "p1", BinaryPath: path, SHA256: digest})
	if !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("unauthorized must fail: %v", err)
	}
}

func TestBrokerGate(t *testing.T) {
	path, digest := helperBinary(t, echoHelper)
	broker := permissions.NewBroker(nil)
	r := NewRuntime(nil, 4)
	r.SetBroker(broker)
	err := r.Start(context.Background(), Spec{PluginID: "p1", BinaryPath: path, SHA256: digest})
	if !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("missing grant must fail: %v", err)
	}
	if _, err := broker.Grant("p1", "companion.execute:write", permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}
	if err := r.Start(context.Background(), Spec{PluginID: "p1", BinaryPath: path, SHA256: digest}); err != nil {
		t.Fatal(err)
	}
	_ = r.Stop("p1")
}

func TestHashMismatch(t *testing.T) {
	path, _ := helperBinary(t, echoHelper)
	r := NewRuntime(nil, 4)
	err := r.Start(context.Background(), Spec{PluginID: "p1", BinaryPath: path, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("expected hash mismatch, got %v", err)
	}
}

func TestRejectsNodeShebang(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wrapper")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env node\nconsole.log(1)\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	r := NewRuntime(nil, 4)
	err := r.Start(context.Background(), Spec{PluginID: "p1", BinaryPath: path})
	if !errors.Is(err, ErrNodeForbidden) {
		t.Fatalf("shebang must be rejected, got %v", err)
	}
}

func TestRejectsSymlink(t *testing.T) {
	path, digest := helperBinary(t, echoHelper)
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(path, link); err != nil {
		t.Skip("symlink not supported")
	}
	r := NewRuntime(nil, 4)
	err := r.Start(context.Background(), Spec{PluginID: "p1", BinaryPath: link, SHA256: digest})
	if !errors.Is(err, ErrBinaryUnsafe) {
		t.Fatalf("symlink must be rejected, got %v", err)
	}
}

func TestMaxConcurrency(t *testing.T) {
	path, digest := helperBinary(t, echoHelper)
	r := NewRuntime(nil, 2)
	if err := r.Start(context.Background(), Spec{PluginID: "a", BinaryPath: path, SHA256: digest}); err != nil {
		t.Fatal(err)
	}
	if err := r.Start(context.Background(), Spec{PluginID: "b", BinaryPath: path, SHA256: digest}); err != nil {
		t.Fatal(err)
	}
	err := r.Start(context.Background(), Spec{PluginID: "c", BinaryPath: path, SHA256: digest})
	if !errors.Is(err, ErrMaxNativeprocs) {
		t.Fatalf("expected max concurrency, got %v", err)
	}
	r.StopAll()
}

func TestFramedRPCRoundTrip(t *testing.T) {
	path, digest := helperBinary(t, echoHelper)
	r := NewRuntime(nil, 4)
	if err := r.Start(context.Background(), Spec{PluginID: "p1", BinaryPath: path, SHA256: digest}); err != nil {
		t.Fatal(err)
	}
	defer r.Stop("p1")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := r.Call(ctx, "p1", "ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != `"ok"` {
		t.Fatalf("result %s", result)
	}
}

func TestMalformedRPCQuarantines(t *testing.T) {
	path, digest := helperBinary(t, floodHelper)
	r := NewRuntime(nil, 4)
	if err := r.Start(context.Background(), Spec{PluginID: "p1", BinaryPath: path, SHA256: digest}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if r.IsQuarantined("p1") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("flood must quarantine the plugin")
}

func TestStopAll(t *testing.T) {
	path, digest := helperBinary(t, echoHelper)
	r := NewRuntime(nil, 8)
	if err := r.Start(context.Background(), Spec{PluginID: "p1", BinaryPath: path, SHA256: digest}); err != nil {
		t.Fatal(err)
	}
	r.StopAll()
	if r.IsRunning("p1") {
		t.Fatal("stopAll must clear registry")
	}
}

func TestWriteFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	body := []byte(`{"id":1}`)
	if err := writeFrame(&buf, body); err != nil {
		t.Fatal(err)
	}
	got, err := readFrame(bufio.NewReader(bytes.NewReader(buf.Bytes())))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("%q != %q", got, body)
	}
}

func TestReadFrameRejectsOversize(t *testing.T) {
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], maxFrameBytes+1)
	_, err := readFrame(bufio.NewReader(bytes.NewReader(header[:])))
	if !errors.Is(err, ErrMalformedRPC) {
		t.Fatalf("expected malformed, got %v", err)
	}
}

func TestHashFile(t *testing.T) {
	path, digest := helperBinary(t, echoHelper)
	got, err := HashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != digest {
		t.Fatalf("%s != %s", got, digest)
	}
}

func TestCallOnMissingProcess(t *testing.T) {
	r := NewRuntime(nil, 4)
	_, err := r.Call(context.Background(), "ghost", "ping", nil)
	if !errors.Is(err, ErrNotRunning) {
		t.Fatalf("got %v", err)
	}
}
