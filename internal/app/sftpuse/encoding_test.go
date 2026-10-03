package sftpuse

import (
	"testing"

	"github.com/binaricat/lemonssh/internal/platform/charset"
)

// gbShuJu is the GB18030 byte sequence for "数据"; Go strings carry raw bytes
// exactly as the SFTP wire delivers them.
var gbShuJu = string([]byte{0xCA, 0xFD, 0xBE, 0xDD})

func TestResolveEncodingPinsExplicitAndResolvesAuto(t *testing.T) {
	s := New(nil, nil)

	// Default sessions resolve to UTF-8.
	if got := s.resolveEncoding("s1", ""); got != charset.UTF8 {
		t.Fatalf("empty request = %q, want utf-8", got)
	}

	// Explicit requests pin the session.
	if got := s.resolveEncoding("s1", "gb18030"); got != charset.GB18030 {
		t.Fatalf("explicit = %q, want gb18030", got)
	}
	// Auto/empty requests reuse the pin.
	if got := s.resolveEncoding("s1", ""); got != charset.GB18030 {
		t.Fatalf("auto after pin = %q, want gb18030", got)
	}
	if got := s.resolveEncoding("s1", "auto"); got != charset.GB18030 {
		t.Fatalf("auto request after pin = %q, want gb18030", got)
	}

	// Other sessions keep their own state.
	if got := s.resolveEncoding("s2", ""); got != charset.UTF8 {
		t.Fatalf("unpinned session = %q, want utf-8", got)
	}

	// Explicit UTF-8 re-pins.
	if got := s.resolveEncoding("s1", "utf-8"); got != charset.UTF8 {
		t.Fatalf("explicit utf-8 = %q, want utf-8", got)
	}
	if got := s.resolvedEncoding("s1"); got != charset.UTF8 {
		t.Fatalf("resolvedEncoding = %q, want utf-8", got)
	}
}

func TestProbeAndDecodeNamesPinsLegacyCharset(t *testing.T) {
	s := New(nil, nil)

	names := []string{"ascii.txt", gbShuJu + ".log", "中文.md"}
	decoded := s.probeAndDecodeNames("s1", names)
	if decoded[0] != "ascii.txt" || decoded[1] != "数据.log" || decoded[2] != "中文.md" {
		t.Fatalf("decoded = %q", decoded)
	}
	// The probe proved a legacy charset and pinned the session so subsequent
	// operations encode paths back to the same bytes.
	if got := s.resolvedEncoding("s1"); got != charset.GB18030 {
		t.Fatalf("post-probe encoding = %q, want gb18030", got)
	}

	// Encode via the resolved session state round-trips the legacy bytes.
	if got := s.encodePath("s1", "", "/dir/数据.log"); got != "/dir/"+gbShuJu+".log" {
		t.Fatalf("encodePath = % X", []byte(got))
	}
}

func TestProbeAndDecodeNamesKeepsUTF8WhenValid(t *testing.T) {
	s := New(nil, nil)
	decoded := s.probeAndDecodeNames("s1", []string{"plain.txt", "中文.txt"})
	if decoded[0] != "plain.txt" || decoded[1] != "中文.txt" {
		t.Fatalf("decoded = %q", decoded)
	}
	// Valid UTF-8 cannot disprove UTF-8; the session keeps its default so an
	// explicit re-list does not silently switch encodings.
	if got := s.resolvedEncoding("s1"); got != charset.UTF8 {
		t.Fatalf("encoding after clean probe = %q, want utf-8", got)
	}
}

func TestCloseDropsEncodingState(t *testing.T) {
	s := New(nil, nil)
	s.mu.Lock()
	s.encodings["s1"] = charset.GB18030
	s.mu.Unlock()
	// Close without a live session only cleans bookkeeping.
	if err := s.Close("s1"); err != nil {
		t.Fatalf("close: %v", err)
	}
	if got := s.resolveEncoding("s1", ""); got != charset.UTF8 {
		t.Fatalf("encoding after close = %q, want utf-8", got)
	}
}
