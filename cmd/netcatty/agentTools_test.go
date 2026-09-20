package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/app/terminaluse"
	"github.com/binaricat/netcatty/internal/capability"
	"github.com/binaricat/netcatty/internal/rpc"
)

type writableSFTP struct {
	fakeSFTPReader
	called, session, content string
}

func (s *writableSFTP) WriteText(id, path, text string) error {
	s.called, s.session, s.content = "write", id, text
	return nil
}
func (s *writableSFTP) Mkdir(id, path string) error       { s.called = "mkdir"; return nil }
func (s *writableSFTP) Remove(id, path string) error      { s.called = "delete"; return nil }
func (s *writableSFTP) Rename(id, old, next string) error { s.called = "rename"; return nil }
func (s *writableSFTP) Chmod(id, path, mode string) error { s.called = "chmod"; return nil }

func TestEveryAdvertisedNativeToolHasAllRPCSurfaces(t *testing.T) {
	host := newAgentHost(AgentHostConfig{SFTP: &writableSFTP{}, Jobs: terminaluse.NewJobQueue(nil), Attachments: newAttachmentRegistry(), Forwards: NewForwardService(nil, nil), VaultRouter: newAgentVaultRouter(nil)})
	handlers, methods := host.capabilityHandlers(), host.methodTable()
	count := 0
	for _, def := range capability.Catalog {
		if def.Domain == "harness" {
			continue
		}
		count++
		if handlers[def.ID] == nil {
			t.Errorf("missing native handler for %s", def.ID)
		}
		for _, surface := range []capability.Surface{capability.SurfaceBuiltin, capability.SurfaceGlobal, capability.SurfacePublic} {
			if method := def.Surfaces[surface].RPCMethod; method != "" && methods[method] == nil {
				t.Errorf("missing %s method %s", surface, method)
			}
		}
	}
	t.Logf("covered %d native capabilities", count)
}

func TestSFTPWriteToolsUseNativeSessionAndPolicy(t *testing.T) {
	sftp := &writableSFTP{}
	host := newAgentHost(AgentHostConfig{SFTP: sftp, PermissionMode: "auto"})
	host.state.updateSessions("chat", []AgentSession{{SessionID: "ui", NativeID: "native", Connected: true}}, false)
	for _, op := range []string{"write", "mkdir", "delete", "rename", "chmod"} {
		params := map[string]any{"sessionId": "ui", "path": "/tmp/a", "content": "", "oldPath": "/tmp/a", "newPath": "/tmp/b", "mode": "600"}
		if _, err := host.dispatch(context.Background(), "netcatty/sftp/"+op, params, "chat", nil); err != nil {
			t.Fatal(err)
		}
		if sftp.called != op {
			t.Fatalf("%s did not run", op)
		}
	}
	if sftp.session != "native" || sftp.content != "" {
		t.Fatal("alias or empty file write lost")
	}
	service := &AgentService{host: host}
	for _, mode := range []string{"observer", "confirm"} {
		_ = service.AgentSetPermissionMode(mode)
		sftp.called = ""
		_, err := host.dispatch(context.Background(), "netcatty/sftp/write", map[string]any{"sessionId": "ui", "path": "/x", "content": "x"}, "chat", nil)
		if err == nil || sftp.called != "" {
			t.Fatalf("mode %s executed denied write", mode)
		}
	}
}

func TestToolChatAndPrincipalScopeCannotBeForged(t *testing.T) {
	host := newAgentHost(AgentHostConfig{SFTP: &writableSFTP{}, PermissionMode: "auto"})
	host.state.updateSessions("chat-a", []AgentSession{{SessionID: "a", Connected: true}}, false)
	host.state.updateSessions("chat-b", []AgentSession{{SessionID: "b", Connected: true}}, false)
	_, err := host.dispatch(context.Background(), "netcatty/sftp/read", map[string]any{"sessionId": "b", "path": "/secret", "chatSessionId": "chat-b"}, "chat-a", nil)
	var denied *rpc.ScopeError
	if !errors.As(err, &denied) {
		t.Fatalf("forged scope accepted: %v", err)
	}
	host.state.updateSessions("", []AgentSession{{SessionID: "a"}, {SessionID: "b"}}, false)
	result, err := host.dispatch(context.Background(), "public/getEnvironment", nil, "__external_mcp__", &rpc.Principal{Scope: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	sessions := result.(map[string]any)["sessions"].([]AgentSession)
	if len(sessions) != 1 || sessions[0].SessionID != "a" {
		t.Fatalf("principal scope leaked: %v", sessions)
	}
}

func TestVaultToolsReachApplicationOwnerAndRespectStop(t *testing.T) {
	var router *AgentVaultRouter
	var operation string
	router = newAgentVaultRouter(func(_ string, payload any) {
		request := payload.(map[string]any)
		operation = request["op"].(string)
		_ = router.Respond(request["requestId"].(string), map[string]any{"ok": true, "id": "created"})
	})
	host := newAgentHost(AgentHostConfig{VaultRouter: router, PermissionMode: "auto"})
	for _, def := range capability.Catalog {
		if def.Domain != "vault" && (def.Domain != "portforward" || def.ID == "portforward.tunnels.list") {
			continue
		}
		if _, err := host.dispatch(context.Background(), def.AgentRPCMethod(), nil, "chat", nil); err != nil {
			t.Fatalf("%s: %v", def.ID, err)
		}
		if operation != strings.TrimPrefix(def.ID, "vault.") {
			t.Fatalf("wrong operation %s for %s", operation, def.ID)
		}
	}
	host.setChatCancelled("chat", true)
	operation = ""
	_, err := host.dispatch(context.Background(), "vault/notes/create", nil, "chat", nil)
	if err == nil || operation != "" {
		t.Fatal("stopped chat mutated vault")
	}
}

type cancellableAgentRunner struct{ started chan struct{} }

func (r cancellableAgentRunner) Run(ctx context.Context, _ string, sink func([]byte)) (int, bool, error) {
	close(r.started)
	<-ctx.Done()
	return -1, false, ctx.Err()
}

func TestChatStopCancelsNativeCommandAndApproval(t *testing.T) {
	started := make(chan struct{})
	host := newAgentHost(AgentHostConfig{PermissionMode: "auto", Jobs: terminaluse.NewJobQueue(func(id string) (terminaluse.CommandRunner, error) {
		if id != "native" {
			return nil, fmt.Errorf("wrong terminal: %s", id)
		}
		return cancellableAgentRunner{started}, nil
	})})
	host.state.updateSessions("chat", []AgentSession{{SessionID: "ui", NativeID: "native"}}, false)
	done := make(chan error, 1)
	go func() {
		_, err := host.dispatch(context.Background(), "netcatty/exec", map[string]any{"sessionId": "ui", "command": "wait"}, "chat", nil)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("command never started")
	}
	host.setChatCancelled("chat", true)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancel lost")
		}
	case <-time.After(time.Second):
		t.Fatal("command did not cancel")
	}

	prompt := make(chan struct{})
	host.approvals = newInteractionRouter(func(name string, _ any) {
		if name == "agent:interaction" {
			close(prompt)
		}
	})
	host.state.mode = capability.ModeConfirm
	host.setChatCancelled("chat", false)
	go func() {
		_, err := host.dispatch(context.Background(), "netcatty/exec", map[string]any{"sessionId": "ui", "command": "wait"}, "chat", nil)
		done <- err
	}()
	select {
	case <-prompt:
	case <-time.After(time.Second):
		t.Fatal("approval not requested")
	}
	host.setChatCancelled("chat", true)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("pending approval ran after stop")
		}
	case <-time.After(time.Second):
		t.Fatal("approval did not cancel")
	}
}
