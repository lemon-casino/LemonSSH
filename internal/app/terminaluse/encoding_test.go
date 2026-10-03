package terminaluse

import (
	"bytes"
	"testing"

	"github.com/binaricat/lemonssh/internal/platform/charset"
	"github.com/binaricat/lemonssh/internal/terminal/dataplane"
)

func newEncodingTestService(t *testing.T) *Service {
	t.Helper()
	controller := dataplane.NewRouteController()
	dp := dataplane.NewServer(controller, "127.0.0.1:0")
	return New(controller, dp, nil)
}

func TestSetSessionEncodingValidatesAndNormalizes(t *testing.T) {
	s := newEncodingTestService(t)

	if err := s.SetSessionEncoding("missing", "gb18030"); err == nil {
		t.Fatal("expected error for unknown session")
	}

	s.SeedSessionForTest("t1")
	if err := s.SetSessionEncoding("t1", "gb18030"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := s.SessionEncoding("t1"); got != charset.GB18030 {
		t.Fatalf("SessionEncoding = %q, want gb18030", got)
	}
	// Alias aliases of the GB family normalize to the canonical value.
	if err := s.SetSessionEncoding("t1", "GBK"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := s.SessionEncoding("t1"); got != charset.GB18030 {
		t.Fatalf("SessionEncoding after GBK = %q, want gb18030", got)
	}
	if err := s.SetSessionEncoding("t1", ""); err != nil {
		t.Fatalf("set empty: %v", err)
	}
	if got := s.SessionEncoding("t1"); got != charset.UTF8 {
		t.Fatalf("SessionEncoding after empty = %q, want utf-8", got)
	}
}

func TestEncodeInputTranscodesNonASCII(t *testing.T) {
	// UTF-8 sessions pass through untouched.
	utf8Payload := []byte("列出 /var/log")
	if got := encodeInput(utf8Payload, charset.UTF8); !bytes.Equal(got, utf8Payload) {
		t.Fatalf("utf-8 input changed: % X", got)
	}
	// ASCII control/input bytes are identical under every supported charset.
	ascii := []byte("ls -la\r\n")
	if got := encodeInput(ascii, charset.GB18030); !bytes.Equal(got, ascii) {
		t.Fatalf("ascii input changed: % X", got)
	}
	// Non-ASCII keystrokes encode to GB18030 bytes.
	got := encodeInput([]byte("数据"), charset.GB18030)
	want := []byte{0xCA, 0xFD, 0xBE, 0xDD}
	if !bytes.Equal(got, want) {
		t.Fatalf("gb18030 input = % X, want % X", got, want)
	}
}

type recordingStdin struct {
	bytes.Buffer
}

func (r *recordingStdin) Write(p []byte) (int, error) { return r.Buffer.Write(p) }
func (r *recordingStdin) Close() error                { return nil }

func TestWriteEncodesInputToSessionCharset(t *testing.T) {
	s := newEncodingTestService(t)
	stdin := &recordingStdin{}
	s.mu.Lock()
	s.sessions["t1"] = &terminalSession{stdin: stdin}
	s.mu.Unlock()

	// UTF-8 sessions write the renderer payload unchanged.
	if _, err := s.Write("t1", []byte("ls\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if stdin.String() != "ls\r\n" {
		t.Fatalf("utf-8 write = %q", stdin.String())
	}

	// After the encoding switch, non-ASCII input reaches the stream as GB18030
	// while ASCII stays byte-identical.
	if err := s.SetSessionEncoding("t1", "gb18030"); err != nil {
		t.Fatalf("set: %v", err)
	}
	stdin.Reset()
	if _, err := s.Write("t1", []byte("ls\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := s.Write("t1", []byte("数据")); err != nil {
		t.Fatalf("write: %v", err)
	}
	want := "ls\r\n" + string([]byte{0xCA, 0xFD, 0xBE, 0xDD})
	if stdin.String() != want {
		t.Fatalf("gb18030 write = % X, want % X", stdin.Bytes(), []byte(want))
	}
}
