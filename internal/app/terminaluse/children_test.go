package terminaluse

import (
	"testing"
)

func TestSanitizeChildCommandStripsControlCharacters(t *testing.T) {
	got := sanitizeChildCommand("vi\tm\r\x1b]0;pwned\x07file")
	if got != "vi m  ]0;pwned file" {
		t.Fatalf("control characters survived: %q", got)
	}
	if sanitizeChildCommand("  ") != "" {
		t.Fatal("whitespace-only command should trim to empty")
	}
}

func TestPlatformChildProcessesSmoke(t *testing.T) {
	// A parent pid nothing realistically owns; the enumeration must degrade to
	// an empty list instead of an error on every platform.
	children := platformChildProcesses(1 << 22)
	if len(children) != 0 {
		t.Fatalf("unexpected children for a fresh pid: %+v", children)
	}
}
