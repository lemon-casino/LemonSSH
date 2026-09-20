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
	encoded := "netcatty-sdk-session:" + strings.ReplaceAll(strings.ReplaceAll(string(identity), "{", "%7B"), "}", "%7D")
	// Use QueryEscape-compatible percent encoding for the punctuation that
	// matters to the decoder while keeping this fixture readable.
	encoded = "netcatty-sdk-session:%7B%22v%22%3A1%2C%22id%22%3A%22thread-123%22%2C%22backend%22%3A%22codex%22%7D"
	args := externalAgentArgs("codex", "continue", "gpt-5", "confirm", encoded)
	joined := strings.Join(args, " ")
	if joined != "exec --json --skip-git-repo-check --sandbox read-only --model gpt-5 resume thread-123 continue" {
		t.Fatalf("unexpected codex resume args: %q", joined)
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
	request := ExternalAgentStreamRequest{
		RequestID: `..\\..\\outside`,
		Images: []ExternalAgentImage{{
			Filename:   `..\\escape.png`,
			Base64Data: base64.StdEncoding.EncodeToString([]byte("image")),
		}},
	}
	paths, cleanup := service.stageImages(request)
	defer cleanup()
	if len(paths) != 1 {
		t.Fatalf("expected one staged image, got %v", paths)
	}
	absoluteRoot, _ := filepath.Abs(root)
	absolutePath, _ := filepath.Abs(paths[0])
	relative, err := filepath.Rel(absoluteRoot, absolutePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		t.Fatalf("staged image escaped managed temp: %q", absolutePath)
	}
	if filepath.Base(paths[0]) != "001-escape.png" {
		t.Fatalf("unexpected staged name: %q", filepath.Base(paths[0]))
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
