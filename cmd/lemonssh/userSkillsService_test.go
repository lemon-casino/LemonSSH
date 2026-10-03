package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeUserSkill(t *testing.T, root, directory, name, description, body string) {
	t.Helper()
	skillDirectory := filepath.Join(root, directory)
	if err := os.MkdirAll(skillDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + body
	if err := os.WriteFile(filepath.Join(skillDirectory, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestUserSkillsScanAndBuildContext(t *testing.T) {
	root := t.TempDir()
	writeUserSkill(t, root, "Remote Audit", "Remote Audit", "Inspect remote Linux security and services.", "Run read-only checks first and report the collected output.")
	service := newUserSkillsService(root)

	status := service.GetStatus()
	if !status.OK || status.ReadyCount != 1 || status.WarningCount != 0 {
		t.Fatalf("unexpected status: %+v", status)
	}
	context := service.BuildContext("Please perform a remote Linux security audit", nil)
	if !context.OK || !strings.Contains(context.Context, "Run read-only checks first") {
		t.Fatalf("matching skill was not injected: %+v", context)
	}
}

func TestUserSkillsRejectDuplicateSlugsAndOversizedFiles(t *testing.T) {
	root := t.TempDir()
	writeUserSkill(t, root, "first", "Shared Skill", "First description.", "first")
	writeUserSkill(t, root, "second", "Shared Skill", "Second description.", "second")
	oversizedDirectory := filepath.Join(root, "oversized")
	if err := os.MkdirAll(oversizedDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oversizedDirectory, "SKILL.md"), []byte(strings.Repeat("x", maxUserSkillBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}

	status := newUserSkillsService(root).GetStatus()
	if status.ReadyCount != 0 || status.WarningCount != 3 {
		t.Fatalf("unsafe or duplicate skills were accepted: %+v", status)
	}
	joined := strings.Join(status.Warnings, "\n")
	if !strings.Contains(joined, "Duplicate skill slug") || !strings.Contains(joined, "too large") {
		t.Fatalf("expected validation warnings, got %q", joined)
	}
}
