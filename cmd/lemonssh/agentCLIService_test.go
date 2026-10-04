package main

import (
	"fmt"
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

func TestPrewarmRefreshesShellEnvOnce(t *testing.T) {
	service := newAgentCLIService()
	calls := 0
	service.refreshShellEnvForTest = func() (bool, error) {
		calls++
		return true, nil
	}

	result := service.Prewarm()
	if !result.OK || !result.Refreshed || result.Error != "" {
		t.Fatalf("prewarm must succeed and report the refresh, got %+v", result)
	}
	// Resolve/Discover reuse the sticky warm-up instead of re-running the shell.
	service.Resolve("codex", "", true, false)
	service.Discover(true, false)
	if calls != 1 {
		t.Fatalf("login-shell probe must run once per process, ran %d times", calls)
	}

	// A probe that applies nothing must not claim a refresh. The real probe
	// is platform-specific by design — a permanent no-op on Windows (registry
	// built PATH) and legitimately applied=true on Unix when the login shell
	// merges its PATH — so the no-refresh mapping is asserted through the
	// hook instead of the platform's probe behavior.
	notApplied := newAgentCLIService()
	notApplied.refreshShellEnvForTest = func() (bool, error) { return false, nil }
	if result := notApplied.Prewarm(); !result.OK || result.Refreshed || result.Error != "" {
		t.Fatalf("a service whose probe applies nothing must not claim a refresh, got %+v", result)
	}
	if runtime.GOOS == "windows" {
		// The real Windows probe never touches the environment.
		if result := newAgentCLIService().Prewarm(); !result.OK || result.Refreshed || result.Error != "" {
			t.Fatalf("windows prewarm without a shell probe must not claim a refresh, got %+v", result)
		}
	}
}

func TestPrewarmReportsProbeFailuresInline(t *testing.T) {
	service := newAgentCLIService()
	service.refreshShellEnvForTest = func() (bool, error) {
		return false, fmt.Errorf("login shell probe failed")
	}
	result := service.Prewarm()
	if !result.OK {
		t.Fatalf("prewarm stays best-effort ok, got %+v", result)
	}
	if result.Refreshed || result.Error == "" {
		t.Fatalf("failure must be reported inline: %+v", result)
	}
}

func TestExtractMarkedSegment(t *testing.T) {
	output := "rc noise\nprint hello\n\nLEMONSSH_PATH_BEGIN\n/usr/local/bin:/usr/bin\nLEMONSSH_PATH_END\n"
	if got := extractMarkedSegment(output, shellEnvPathBegin, shellEnvPathEnd); got != "/usr/local/bin:/usr/bin" {
		t.Fatalf("segment between markers expected, got %q", got)
	}
	if got := extractMarkedSegment("no markers", shellEnvPathBegin, shellEnvPathEnd); got != "" {
		t.Fatalf("missing markers must yield empty segment, got %q", got)
	}
	// An unterminated marker pair (e.g. rc noise echoing the marker) must
	// not win: the search walks back to the outermost complete pair.
	noisy := "\nLEMONSSH_PATH_BEGIN\nnoise without an end marker\nLEMONSSH_PATH_BEGIN\n/bin:/usr/bin\nLEMONSSH_PATH_END\n"
	if got := extractMarkedSegment(noisy, shellEnvPathBegin, shellEnvPathEnd); got != "/bin:/usr/bin" {
		t.Fatalf("outermost complete marker pair must win, got %q", got)
	}
	// A marker-shaped substring that is not a marker line never matches.
	inline := "\nLEMONSSH_PATH_BEGIN\n/opt/LEMONSSH_PATH_BEGIN/bin\nLEMONSSH_PATH_END\n"
	if got := extractMarkedSegment(inline, shellEnvPathBegin, shellEnvPathEnd); got != "/opt/LEMONSSH_PATH_BEGIN/bin" {
		t.Fatalf("payload may embed the marker inline, got %q", got)
	}
}

func TestMergePathList(t *testing.T) {
	separator := string(os.PathListSeparator)
	merged := mergePathList("/opt/homebrew/bin"+separator+"/usr/local/bin", "/usr/bin"+separator+"/opt/homebrew/bin")
	want := []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin"}
	if got := filepath.SplitList(merged); len(got) != len(want) {
		t.Fatalf("merged PATH %q must dedupe and keep shell entries first", merged)
	}
	for i, entry := range want {
		if filepath.SplitList(merged)[i] != entry {
			t.Fatalf("merged PATH entry %d = %q, want %q", i, filepath.SplitList(merged)[i], entry)
		}
	}
}
