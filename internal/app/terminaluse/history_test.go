package terminaluse

import (
	"strings"
	"testing"
)

func TestBuildRemoteHistoryProbeValidatesLimit(t *testing.T) {
	if got := BuildRemoteHistoryProbe(0); !strings.Contains(got, `tail -n "1000"`) {
		t.Fatalf("default limit missing: %q", got)
	}
	if got := BuildRemoteHistoryProbe(7); !strings.Contains(got, `tail -n "7"`) {
		t.Fatalf("custom limit missing: %q", got)
	}
	if got := BuildRemoteHistoryProbe(99999999); !strings.Contains(got, `tail -n "50000"`) {
		t.Fatalf("limit not clamped: %q", got)
	}
	if got := BuildRemoteHistoryProbe(12); strings.ContainsAny(got, "`$;\n\n") && strings.Contains(got, "__NC_LIMIT__") {
		t.Fatalf("placeholder not substituted: %q", got)
	}
}

func TestParseRemoteHistoryProbeSections(t *testing.T) {
	output := strings.Join([]string{
		"shell=bash",
		"__NC_BEGIN_bash__",
		"ls -la",
		"cd /tmp",
		"__NC_END_bash__",
		"__NC_BEGIN_zsh__",
		": 1690000000:0;git status",
		"__NC_END_zsh__",
		"__NC_BEGIN_fish__",
		"- cmd: cargo build",
		"  when: 1690000000",
		"__NC_END_fish__",
		"",
	}, "\n")
	result := ParseRemoteHistoryProbe(output)
	if !result.Success || result.Shell != "bash" {
		t.Fatalf("unexpected header parsing: %+v", result)
	}
	if result.Bash != "ls -la\ncd /tmp\n" {
		t.Fatalf("bash section mismatch: %q", result.Bash)
	}
	if result.Zsh != ": 1690000000:0;git status\n" {
		t.Fatalf("zsh section mismatch: %q", result.Zsh)
	}
	if result.Fish != "- cmd: cargo build\n  when: 1690000000\n" {
		t.Fatalf("fish section mismatch: %q", result.Fish)
	}
}

func TestParseRemoteHistoryProbeWithoutFiles(t *testing.T) {
	result := ParseRemoteHistoryProbe("shell=zsh\n__NC_BEGIN_zsh__\n__NC_END_zsh__\n")
	if !result.Success || result.Shell != "zsh" {
		t.Fatalf("unexpected parsing: %+v", result)
	}
	if result.Zsh != "" || result.Bash != "" || result.Fish != "" {
		t.Fatalf("empty sections must stay empty: %+v", result)
	}
}

func TestGetSessionDistroInfoShape(t *testing.T) {
	// Only the failure shape is exercised without a live transport; the probe
	// command itself is a contract with the renderer (os-release first).
	s, _, _ := newAttachTestService(t)
	result := s.GetSessionDistroInfo("missing")
	if result.Success || result.Error == "" {
		t.Fatalf("unknown session must fail: %+v", result)
	}
}
