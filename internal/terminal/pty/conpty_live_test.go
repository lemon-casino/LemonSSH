//go:build windows

package pty

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConptyLiveSmoke(t *testing.T) {
	session := NewSession(BuildConfig("conpty-smoke", "cmd.exe", "", []string{"/q"}, nil, 100, 30))
	backend := NewPlatformBackend()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := session.Start(ctx, backend); err != nil {
		t.Fatalf("start cmd.exe via ConPTY: %v", err)
	}

	// Collect output concurrently (mutex: the race detector verifies it).
	var outputMu sync.Mutex
	var outputBuf bytes.Buffer
	appendOutput := func(data []byte) {
		outputMu.Lock()
		defer outputMu.Unlock()
		outputBuf.Write(data)
	}
	output := func() string {
		outputMu.Lock()
		defer outputMu.Unlock()
		return outputBuf.String()
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := session.readOnce(buf)
			if n > 0 {
				appendOutput(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	// The cmd banner/prompt should appear.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output(), "Microsoft Windows") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(output(), "Microsoft Windows") {
		t.Fatalf("banner missing from ConPTY output: %q", output())
	}

	// Echo a marker command through the PTY input.
	generation := session.Generation()
	if _, err := session.Write(generation, []byte("echo NETCATTY_CONPTY_OK\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output(), "NETCATTY_CONPTY_OK") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(output(), "NETCATTY_CONPTY_OK") {
		t.Fatalf("echo output missing: %q", output())
	}

	// Resize must succeed against the live ConPTY.
	if err := session.Resize(generation, 120, 40); err != nil {
		t.Fatalf("resize: %v", err)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("output reader did not terminate after close")
	}
}
