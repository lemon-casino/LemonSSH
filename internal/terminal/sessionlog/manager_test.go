package sessionlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartAppendStopAndRestart(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(dir)
	path, err := manager.Start("s1", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(path), "netcatty-script-") {
		t.Fatalf("default name: %s", path)
	}
	if filepath.Dir(path) != dir {
		t.Fatalf("dir: %s", filepath.Dir(path))
	}
	manager.Append("s1", []byte("first\r\n"))
	if err := manager.Stop("s1"); err != nil {
		t.Fatal(err)
	}
	manager.Append("s1", []byte("ignored"))
	second, err := manager.Start("s1", filepath.Join(dir, "second.log"))
	if err != nil {
		t.Fatal(err)
	}
	manager.Append("s1", []byte("second\r\n"))
	if err := manager.Stop("s1"); err != nil {
		t.Fatal(err)
	}
	firstData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstData) != "first\r\n" {
		t.Fatalf("first file: %q", firstData)
	}
	secondData, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(secondData) != "second\r\n" {
		t.Fatalf("second file: %q", secondData)
	}
}

func TestStopWithoutStartFailsAndAppendWithoutStartIsNoop(t *testing.T) {
	manager := NewManager(t.TempDir())
	if err := manager.Stop("missing"); err == nil {
		t.Fatal("stop without start must fail")
	}
	manager.Append("missing", []byte("noop"))
}

func TestStartRejectsEmptySession(t *testing.T) {
	manager := NewManager(t.TempDir())
	if _, err := manager.Start("", ""); err == nil {
		t.Fatal("empty session must fail")
	}
}
