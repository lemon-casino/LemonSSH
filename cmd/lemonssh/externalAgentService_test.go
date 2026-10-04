package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExternalAgentCodexResumeFlagsPrecedeSubcommand(t *testing.T) {
	identity, err := json.Marshal(map[string]any{"v": 1, "id": "thread-123", "backend": "codex"})
	if err != nil {
		t.Fatal(err)
	}
	encoded := "lemonssh-sdk-session:" + strings.ReplaceAll(strings.ReplaceAll(string(identity), "{", "%7B"), "}", "%7D")
	// Use QueryEscape-compatible percent encoding for the punctuation that
	// matters to the decoder while keeping this fixture readable.
	encoded = "lemonssh-sdk-session:%7B%22v%22%3A1%2C%22id%22%3A%22thread-123%22%2C%22backend%22%3A%22codex%22%7D"
	args := externalAgentArgs("codex", "continue", "gpt-5", "confirm", encoded)
	joined := strings.Join(args, " ")
	if joined != "exec --json --skip-git-repo-check --sandbox read-only --model gpt-5 resume thread-123 continue" {
		t.Fatalf("unexpected codex resume args: %q", joined)
	}
	// Pre-rename session records keep decoding through the legacy prefix.
	legacy := "netcatty-sdk-session:%7B%22v%22%3A1%2C%22id%22%3A%22thread-123%22%2C%22backend%22%3A%22codex%22%7D"
	legacyArgs := externalAgentArgs("codex", "continue", "gpt-5", "confirm", legacy)
	if strings.Join(legacyArgs, " ") != joined {
		t.Fatalf("legacy prefix must decode identically: %q", strings.Join(legacyArgs, " "))
	}
}

func TestExternalAgentPermissionModesFailClosed(t *testing.T) {
	confirm := strings.Join(externalAgentArgs("cursor", "inspect", "", "confirm", ""), " ")
	if !strings.Contains(confirm, "--mode ask") || strings.Contains(confirm, "--force") {
		t.Fatalf("confirm mode must remain read-only/ask: %q", confirm)
	}
	auto := strings.Join(externalAgentArgs("cursor", "edit", "", "auto", ""), " ")
	if !strings.Contains(auto, "--mode agent --force") {
		t.Fatalf("auto mode must explicitly opt into agent writes: %q", auto)
	}
}

func TestExternalAgentAttachmentsStayInsideManagedTemp(t *testing.T) {
	root := t.TempDir()
	service := newExternalAgentService("", root)
	payload := base64.StdEncoding.EncodeToString([]byte("image"))
	request := ExternalAgentStreamRequest{
		RequestID: `..\\..\\outside`,
		Images: []ExternalAgentImage{
			// Windows-style traversal must flatten to the base name on every
			// platform, including Linux where `\` is a legal filename byte.
			{Filename: `..\\escape.png`, Base64Data: payload},
			// POSIX-style traversal.
			{Filename: "../../also-escape.png", Base64Data: payload},
			// Dot-only names carry no usable file name.
			{Filename: `..`, Base64Data: payload},
		},
	}
	paths, cleanup := service.stageImages(request)
	defer cleanup()
	if len(paths) != 3 {
		t.Fatalf("expected three staged images, got %v", paths)
	}
	absoluteRoot, _ := filepath.Abs(root)
	wantNames := []string{"001-escape.png", "002-also-escape.png", "003-attachment.bin"}
	for index, stagedPath := range paths {
		absolutePath, _ := filepath.Abs(stagedPath)
		relative, err := filepath.Rel(absoluteRoot, absolutePath)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			t.Fatalf("staged image escaped managed temp: %q", absolutePath)
		}
		if filepath.Base(stagedPath) != wantNames[index] {
			t.Fatalf("unexpected staged name: %q", filepath.Base(stagedPath))
		}
	}
	cleanup()
	if _, err := os.Stat(externalAttachmentDirectory(root, request.RequestID)); !os.IsNotExist(err) {
		t.Fatalf("staged directory was not cleaned: %v", err)
	}
}

func TestExternalAgentToolResolvesBesidePackagedExecutable(t *testing.T) {
	directory := t.TempDir()
	name := "LemonSSH-tool"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	toolPath := filepath.Join(directory, name)
	if err := os.WriteFile(toolPath, []byte("tool"), 0o700); err != nil {
		t.Fatal(err)
	}
	resolved := resolveExternalAgentToolPathFrom(filepath.Join(directory, "LemonSSH.exe"), "")
	if resolved != toolPath {
		t.Fatalf("packaged tool was not resolved beside the app: %q", resolved)
	}
}

func TestExternalAgentPartialOutputMarksTurnDoneAfterFailure(t *testing.T) {
	var events []string
	service := newExternalAgentService("", t.TempDir())
	service.setEventEmitter(func(name string, payload any) { events = append(events, name) })
	run := &externalAgentRun{}
	service.handleAgentOutputLine("request", "codex", "codex", run, `{"type":"item.completed","item":{"type":"agent_message","text":"collected result"}}`)
	run.mu.Lock()
	emitted := run.emittedText
	run.mu.Unlock()
	if !emitted || len(events) != 1 || events[0] != "ai:sdk-agent:event" {
		t.Fatalf("partial output was not emitted: emitted=%v events=%v", emitted, events)
	}
}
