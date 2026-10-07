package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lemon-casino/lemonssh/internal/rpc"
)

func TestExternalAgentToolModesProduceDifferentPromptsAndConfig(t *testing.T) {
	s := newExternalAgentService("discovery.json", t.TempDir())
	for _, backend := range []string{"claude", "codebuddy", "codex", "opencode", "copilot", "cursor", "grok"} {
		t.Run(backend, func(t *testing.T) {
			cwd := t.TempDir()
			for _, mode := range []string{"mcp", "skills"} {
				r := ExternalAgentStreamRequest{RequestID: backend + mode, ChatSessionID: "chat", SDKBackend: backend, CWD: cwd, Prompt: "inspect", ToolIntegrationMode: mode}
				cleanup, err := s.prepareToolContext(&r)
				if err != nil {
					t.Fatal(err)
				}
				defer cleanup()
				prompt := s.buildPrompt(r, nil)
				if mode == "skills" && (!strings.Contains(prompt, "--chat-session") || strings.Contains(prompt, "injected lemonssh MCP")) {
					t.Fatalf("%s", prompt)
				}
				if mode == "mcp" && (!strings.Contains(prompt, "injected lemonssh MCP") || strings.Contains(prompt, "--chat-session")) {
					t.Fatalf("%s", prompt)
				}
				args, env, restore, err := s.configureAgentTools(r, backend, []string{"prompt"}, cwd)
				if err != nil {
					t.Fatal(err)
				}
				joined := strings.Join(args, " ")
				if backend == "opencode" {
					var config map[string]any
					if err := json.Unmarshal([]byte(env["OPENCODE_CONFIG_CONTENT"]), &config); err != nil {
						t.Fatal(err)
					}
					mcp := config["mcp"].(map[string]any)
					if (len(mcp) > 0) != (mode == "mcp") {
						t.Fatalf("%v", config)
					}
				} else if backend != "cursor" && backend != "grok" {
					if (strings.Contains(joined, "LemonSSH-mcp")) != (mode == "mcp") {
						t.Fatalf("%s %s: %s", backend, mode, joined)
					}
				}
				restore()
			}
		})
	}
}

func TestExternalAgentWorkspaceConfigRestoresWithoutOverwritingUserEdits(t *testing.T) {
	s := newExternalAgentService("", t.TempDir())
	cwd := t.TempDir()
	path := filepath.Join(cwd, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte("{\"mcpServers\":{\"user\":{\"command\":\"existing\"}}}\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	r := ExternalAgentStreamRequest{ChatSessionID: "chat", AgentEnv: map[string]string{"LEMONSSH_TOOL_CLI_DISCOVERY_FILE": "test"}}
	cleanup, err := s.acquireWorkspaceMCP(cwd, "cursor", r)
	if err != nil {
		t.Fatal(err)
	}
	current, _ := os.ReadFile(path)
	if !strings.Contains(string(current), "existing") {
		t.Fatal("existing MCP entries lost")
	}
	cleanup()
	restored, _ := os.ReadFile(path)
	if string(restored) != string(original) {
		t.Fatal("workspace config not restored")
	}
	cleanup, err = s.acquireWorkspaceMCP(cwd, "cursor", r)
	if err != nil {
		t.Fatal(err)
	}
	changed := []byte(`{"mcpServers":{"user":{"command":"updated"}}}`)
	if err := os.WriteFile(path, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	cleanup()
	restored, _ = os.ReadFile(path)
	if string(restored) != string(changed) {
		t.Fatal("concurrent user edits overwritten")
	}
}

func TestExternalAgentWorkspaceRefusesCrossChatScopeCollision(t *testing.T) {
	s := newExternalAgentService("", t.TempDir())
	cwd := t.TempDir()
	a := ExternalAgentStreamRequest{ChatSessionID: "a", AgentEnv: map[string]string{}}
	cleanup, err := s.acquireWorkspaceMCP(cwd, "cursor", a)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	b := a
	b.ChatSessionID = "b"
	if _, err := s.acquireWorkspaceMCP(cwd, "cursor", b); err == nil {
		t.Fatal("different chat replaced active workspace scope")
	}
}

func TestExternalAgentChatTokenPinsScopeAndRevokesOnCleanup(t *testing.T) {
	root := t.TempDir()
	host := newAgentHost(AgentHostConfig{})
	path := filepath.Join(root, "host.json")
	if err := host.Start(path); err != nil {
		t.Fatal(err)
	}
	defer host.Stop()
	s := newExternalAgentService(path, root)
	s.host = host
	request := ExternalAgentStreamRequest{RequestID: "req", ChatSessionID: "original", ToolIntegrationMode: "mcp"}
	cleanup, err := s.prepareToolContext(&request)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	childPath := request.AgentEnv["LEMONSSH_TOOL_CLI_DISCOVERY_FILE"]
	discovery, err := rpc.LoadDiscovery(childPath)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := host.tokens.Verify(discovery.Token)
	if err != nil || principal.ChatSessionID != "original" {
		t.Fatalf("%+v %v", principal, err)
	}
	if _, err := host.dispatch(context.Background(), "lemonssh/getContext", map[string]any{}, "forged", principal); err == nil {
		t.Fatal("forged chat scope accepted")
	}
	cleanup()
	if _, err := host.tokens.Verify(discovery.Token); err == nil {
		t.Fatal("completed turn token still valid")
	}
	if _, err := os.Stat(childPath); !os.IsNotExist(err) {
		t.Fatalf("child discovery not cleaned: %v", err)
	}
}
