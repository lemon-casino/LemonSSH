package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

type codebuddyWriter struct {
	sync.Mutex
	bytes.Buffer
	closed bool
}

func (w *codebuddyWriter) Write(data []byte) (int, error) {
	w.Lock()
	defer w.Unlock()
	if w.closed {
		return 0, io.ErrClosedPipe
	}
	return w.Buffer.Write(data)
}
func (w *codebuddyWriter) Close() error { w.Lock(); w.closed = true; w.Unlock(); return nil }
func (w *codebuddyWriter) messages(t *testing.T) []map[string]any {
	t.Helper()
	w.Lock()
	defer w.Unlock()
	out := []map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(w.Buffer.String()), "\n") {
		if line == "" {
			continue
		}
		var value map[string]any
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			t.Fatal(err)
		}
		out = append(out, value)
	}
	return out
}

func codebuddyTestRun(t *testing.T, s *ExternalAgentService, id, chat string) (*externalAgentRun, *codebuddyWriter) {
	t.Helper()
	writer := &codebuddyWriter{}
	run := &externalAgentRun{requestID: id, chatSessionID: chat}
	run.codebuddy = newCodebuddyRun(writer, ExternalAgentStreamRequest{RequestID: id, ChatSessionID: chat, ToolIntegrationMode: "mcp"}, "question")
	s.active[id] = run
	t.Cleanup(func() { s.closeCodebuddy(run) })
	return run, writer
}

func codebuddyRequest(controlID, protocolID string) map[string]any {
	return map[string]any{"type": "control_request", "request_id": controlID, "request": map[string]any{
		"subtype": "elicitation_create", "mode": "form", "message": "Choose a target", "requestedSchema": map[string]any{"type": "object"},
		"_meta": map[string]any{"codebuddy.ai": map[string]any{"elicitationId": protocolID}},
	}}
}

func TestCodebuddyInitializationAdvertisesElicitationBeforeSendingPrompt(t *testing.T) {
	s := newExternalAgentService("", t.TempDir())
	run, writer := codebuddyTestRun(t, s, "req", "chat")
	if err := s.initializeCodebuddy(run); err != nil {
		t.Fatal(err)
	}
	messages := writer.messages(t)
	if len(messages) != 1 || stringAt(messages[0], "request", "subtype") != "initialize" {
		t.Fatalf("%v", messages)
	}
	request := messages[0]["request"].(map[string]any)
	if request["capabilities"].(map[string]any)["elicitation"].(map[string]any)["form"] != true {
		t.Fatal("elicitation capability missing")
	}
	s.handleCodebuddyControl(run, map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": codebuddyInitializeID}})
	messages = writer.messages(t)
	if len(messages) != 2 || stringAt(messages[1], "message", "content") != "question" {
		t.Fatalf("%v", messages)
	}
}

func TestCodebuddyElicitationResponseIsScopedAndSingleUse(t *testing.T) {
	s := newExternalAgentService("", t.TempDir())
	a, wa := codebuddyTestRun(t, s, "a", "chat-a")
	b, wb := codebuddyTestRun(t, s, "b", "chat-b")
	s.handleCodebuddyControl(a, codebuddyRequest("wire-a", "same-id"))
	s.handleCodebuddyControl(b, codebuddyRequest("wire-b", "same-id"))
	if got := s.RespondCodebuddyElicitation(codebuddyElicitationID(a, "same-id"), "accept", map[string]any{"target": "dev"}); !got.OK {
		t.Fatalf("%+v", got)
	}
	messages := wa.messages(t)
	if len(messages) != 1 || stringAt(messages[0], "response", "request_id") != "wire-a" || stringAt(messages[0], "response", "response", "action") != "accept" || stringAt(messages[0], "response", "response", "content", "target") != "dev" {
		t.Fatalf("%v", messages)
	}
	if len(wb.messages(t)) != 0 {
		t.Fatal("response leaked into another chat")
	}
	if s.RespondCodebuddyElicitation(codebuddyElicitationID(a, "same-id"), "accept", nil).OK {
		t.Fatal("duplicate response accepted")
	}
	if s.RespondCodebuddyElicitation(codebuddyElicitationID(b, "same-id"), "invalid", nil).OK {
		t.Fatal("invalid action accepted")
	}
	if !s.RespondCodebuddyElicitation(codebuddyElicitationID(b, "same-id"), "decline", nil).OK {
		t.Fatal("invalid action consumed pending request")
	}
}

func TestCodebuddyCompleteAndCancelClearPendingCards(t *testing.T) {
	s := newExternalAgentService("", t.TempDir())
	run, writer := codebuddyTestRun(t, s, "req", "chat")
	var cleared []string
	s.emit = func(_ string, payload any) {
		p := payload.(map[string]any)
		event, _ := p["event"].(map[string]any)
		if stringAt(event, "type") == "elicitation-complete" {
			cleared = append(cleared, stringAt(event, "notification", "elicitationId"))
		}
	}
	s.handleCodebuddyControl(run, codebuddyRequest("w1", "one"))
	s.handleCodebuddyControl(run, map[string]any{"type": "control_notification", "channel": "elicitation", "data": map[string]any{"elicitationId": "one"}})
	if len(cleared) != 1 || len(writer.messages(t)) != 1 {
		t.Fatalf("cleared=%v", cleared)
	}
	s.handleCodebuddyControl(run, codebuddyRequest("w2", "two"))
	s.Cancel("req", "chat")
	if len(cleared) != 2 || len(run.codebuddy.pending) != 0 {
		t.Fatalf("cleared=%v pending=%v", cleared, run.codebuddy.pending)
	}
	if s.RespondCodebuddyElicitation(codebuddyElicitationID(run, "two"), "accept", nil).OK {
		t.Fatal("cancelled card accepted")
	}
}

func TestCodebuddyDuplicateAndExpiredElicitationsResolveCancellation(t *testing.T) {
	s := newExternalAgentService("", t.TempDir())
	run, writer := codebuddyTestRun(t, s, "req", "chat")
	s.handleCodebuddyControl(run, codebuddyRequest("first", "same"))
	s.handleCodebuddyControl(run, codebuddyRequest("second", "same"))
	key := codebuddyElicitationID(run, "same")
	pending := run.codebuddy.pending[key]
	s.expireCodebuddyElicitation(run, key, pending)
	messages := writer.messages(t)
	if len(messages) != 2 || stringAt(messages[0], "response", "request_id") != "first" || stringAt(messages[1], "response", "request_id") != "second" {
		t.Fatalf("%v", messages)
	}
}

func TestCodebuddySkillsCommandCannotEscapeLocalWrapper(t *testing.T) {
	tool := resolveExternalAgentToolPath()
	prefix := shellQuote(tool)
	valid := prefix + ` exec --chat-session "chat" -- "uptime"`
	if !isLemonSSHCLICommand(valid, tool, "chat") {
		t.Fatalf("valid wrapper rejected: %s", valid)
	}
	if !isLemonSSHCLICommand(prefix+` capabilities`, tool, "chat") {
		t.Fatal("capability discovery was blocked")
	}
	for _, command := range []string{prefix + ` exec --chat-session chat --chat-session forged -- uptime`, prefix + ` exec -- uptime --chat-session chat`, valid + `; rm x`, valid + ` && whoami`, valid + ` | cat`, prefix + ` exec --chat-session other -- "uptime"`, prefix + ` exec --chat-session chat -- "$(whoami)"`, `other-tool exec --chat-session chat`, prefix + ` exec --chat-session chat -- "unclosed`} {
		if isLemonSSHCLICommand(command, tool, "chat") {
			t.Fatalf("unsafe wrapper allowed: %s", command)
		}
	}
}

func TestCodebuddyPreservesSessionUsageAndProtocolErrors(t *testing.T) {
	s := newExternalAgentService("", t.TempDir())
	run, _ := codebuddyTestRun(t, s, "req", "chat")
	var kinds []string
	s.emit = func(_ string, payload any) {
		p := payload.(map[string]any)
		event, _ := p["event"].(map[string]any)
		kinds = append(kinds, stringAt(event, "type"))
	}
	s.handleAgentOutputLine("req", "codebuddy", "fake", run, `{"type":"result","session_id":"resume-me","is_error":true,"errors":["quota"],"usage":{"input_tokens":12,"output_tokens":3}}`)
	if strings.Join(kinds, ",") != "session-id,usage" {
		t.Fatalf("events=%v", kinds)
	}
	if !strings.Contains(run.protocolError, "quota") {
		t.Fatalf("protocol error dropped: %+v", run.protocolError)
	}
}

func TestCodebuddyControlPipeHelper(t *testing.T) {
	if os.Getenv("LEMONSSH_TEST_CODEBUDDY_PIPE") != "1" {
		return
	}
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	var init map[string]any
	if decoder.Decode(&init) != nil || stringAt(init, "request", "subtype") != "initialize" {
		os.Exit(2)
	}
	_ = encoder.Encode(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": codebuddyInitializeID}})
	var prompt map[string]any
	if decoder.Decode(&prompt) != nil || stringAt(prompt, "type") != "user" {
		os.Exit(3)
	}
	_ = encoder.Encode(codebuddyRequest("pipe-request", "pipe-card"))
	var response map[string]any
	if decoder.Decode(&response) != nil || stringAt(response, "response", "response", "action") != "accept" {
		os.Exit(4)
	}
	_ = encoder.Encode(map[string]any{"type": "result", "result": "confirmed"})
	os.Exit(0)
}

func TestCodebuddyRealProcessControlRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCodebuddyControlPipeHelper$")
	cmd.Env = append(os.Environ(), "LEMONSSH_TEST_CODEBUDDY_PIPE=1")
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	s := newExternalAgentService("", t.TempDir())
	run := &externalAgentRun{requestID: "pipe", chatSessionID: "chat", command: cmd, cancel: cancel}
	run.codebuddy = newCodebuddyRun(stdin, ExternalAgentStreamRequest{RequestID: "pipe", ChatSessionID: "chat"}, "ask")
	s.active["pipe"] = run
	done := make(chan string, 1)
	s.emit = func(name string, payload any) {
		p := payload.(map[string]any)
		event, _ := p["event"].(map[string]any)
		if stringAt(event, "type") == "elicitation-create" {
			result := s.RespondCodebuddyElicitation(stringAt(event, "elicitationId"), "accept", map[string]any{"answer": "yes"})
			if !result.OK {
				done <- result.Error
			}
		}
		if name == "ai:sdk-agent:done" {
			done <- ""
		}
		if name == "ai:sdk-agent:error" {
			done <- fmtSprint(p["error"])
		}
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go s.consumeAgentRun("pipe", "codebuddy", "fake", run, stdout, stderr, func() {})
	if err := s.initializeCodebuddy(run); err != nil {
		t.Fatal(err)
	}
	select {
	case failure := <-done:
		if failure != "" {
			t.Fatal(failure)
		}
	case <-ctx.Done():
		t.Fatal("CodeBuddy control round trip timed out")
	}
	run.mu.Lock()
	emitted := run.emittedText
	run.mu.Unlock()
	if !emitted {
		t.Fatal("final response missing")
	}
}

func fmtSprint(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return "unexpected error"
}
