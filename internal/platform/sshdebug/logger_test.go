package sshdebug

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDisabledLoggerWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh-debug.log")
	SetPath(path)
	t.Cleanup(func() { SetEnabled(false); SetPath("") })
	SetEnabled(false)
	Logf("should not appear")
	_, err := os.Stat(path)
	if !os.IsNotExist(err) {
		t.Fatal("disabled logger must not create the file")
	}
}

func TestEnabledLoggerAppendsLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh-debug.log")
	SetPath(path)
	t.Cleanup(func() { SetEnabled(false); SetPath("") })
	SetEnabled(true)
	if !Enabled() {
		t.Fatal("enabled state not persisted")
	}
	Logf("dial start host=%s port=%d", "example", 22)
	LogError("dial failed err=%v", "boom")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "dial start host=example port=22") {
		t.Fatalf("info line missing: %q", content)
	}
	if !strings.Contains(content, "ERROR dial failed err=boom") {
		t.Fatalf("error line missing: %q", content)
	}
	if !strings.HasPrefix(content, "20") {
		t.Fatalf("lines must be timestamped: %q", content)
	}
	if strings.Count(content, "\n") != 2 {
		t.Fatalf("unexpected line count: %q", content)
	}
}
