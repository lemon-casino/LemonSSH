package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveAgentCLIFromSelectedDirectory(t *testing.T) {
	directory := t.TempDir()
	name := "codex"
	content := "#!/bin/sh\necho 'codex-cli 1.2.3'\n"
	if runtime.GOOS == "windows" {
		name = "codex.cmd"
		content = "@echo off\r\necho codex-cli 1.2.3\r\n"
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}

	result := resolveAgentCLI("codex", directory, false)
	if !result.Available || result.Path != path {
		t.Fatalf("expected selected directory executable, got %+v", result)
	}
	if result.Version == nil || *result.Version != "codex-cli 1.2.3" {
		t.Fatalf("unexpected version: %+v", result.Version)
	}
}

func TestResolveAgentCLIRejectsUnknownCommand(t *testing.T) {
	if result := resolveAgentCLI("powershell", t.TempDir(), false); result.Available || result.Path != "" {
		t.Fatalf("unknown commands must fail closed: %+v", result)
	}
}
