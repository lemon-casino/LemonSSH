package main

// Service-level tests for the Codex App Server integration. All tests inject
// an in-process fake app-server via transportFactory — no codex binary and no
// real subprocess is involved.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// codexRecordedEvent is one emitted Wails event.
type codexRecordedEvent struct {
	name    string
	payload map[string]any
}

// codexEventRecorder collects emit() calls and mirrors them onto a channel for
// synchronous waiting.
type codexEventRecorder struct {
	mu     sync.Mutex
	events []codexRecordedEvent
	ch     chan codexRecordedEvent
}

func newCodexEventRecorder() *codexEventRecorder {
	return &codexEventRecorder{ch: make(chan codexRecordedEvent, 128)}
}

func (r *codexEventRecorder) emit(name string, payload any) {
	event := codexRecordedEvent{name: name}
	if payload, ok := payload.(map[string]any); ok {
		event.payload = payload
	}
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
	r.ch <- event
}

func (r *codexEventRecorder) all() []codexRecordedEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]codexRecordedEvent(nil), r.events...)
}

func waitForCodexEvent(t *testing.T, recorder *codexEventRecorder, name string, timeout time.Duration) codexRecordedEvent {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case event := <-recorder.ch:
			if event.name == name {
				return event
			}
		case <-deadline:
			t.Fatalf("timed out waiting for event %q; got %v", name, recorder.all())
		}
	}
}

// newServiceWithFakeAppServer wires a service to an in-process fake server.
// handle runs for every client→server request (use fakeReply inside).
func newServiceWithFakeAppServer(t *testing.T, handle func(fake *fakeAppServer, req fakeRequest)) (*ExternalAgentService, *fakeAppServer, *codexEventRecorder) {
	t.Helper()
	var fake *fakeAppServer
	fake = newFakeAppServer(t, func(req fakeRequest) {
		if handle != nil {
			handle(fake, req)
		}
	})
	// The service builds its own client around the transport; fake.client
	// stays unstarted so only one reader consumes the stdout pipe.
	service := newExternalAgentService("", t.TempDir())
	service.transportFactory = func(executable string, args []string, env []string, dir string) (codexAppServerTransport, error) {
		return fake.transport, nil
	}
	recorder := newCodexEventRecorder()
	service.setEventEmitter(recorder.emit)
	return service, fake, recorder
}

// fakeExecutablePath creates a stat-able stand-in for the codex binary so
// resolveExecutable accepts it without a real CLI on PATH.
func fakeExecutablePath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(path, []byte("fake"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// defaultAppServerHandle answers the standard lifecycle like the real server
// (shapes verified against codex-cli 0.130.0-alpha.5).
func defaultAppServerHandle(fake *fakeAppServer, req fakeRequest) {
	switch req.method {
	case "initialize":
		fake.reply(req.id, map[string]any{"userAgent": "lemonssh-test", "codexHome": "C:\\x", "platformFamily": "windows", "platformOs": "windows"})
	case "thread/start", "thread/resume":
		fake.reply(req.id, map[string]any{
			"thread":         map[string]any{"id": "thread-1", "sessionId": "thread-1", "status": map[string]any{"type": "idle"}},
			"model":          "gpt-test",
			"approvalPolicy": "on-request",
		})
	case "turn/start":
		fake.reply(req.id, map[string]any{"turn": map[string]any{"id": "turn-1", "status": "inProgress"}})
	case "turn/steer":
		fake.reply(req.id, map[string]any{"turnId": "turn-1"})
	case "turn/interrupt":
		fake.reply(req.id, map[string]any{})
	case "model/list":
		fake.reply(req.id, map[string]any{"data": []any{
			map[string]any{"id": "gpt-test", "displayName": "GPT Test", "description": "d", "isDefault": true, "defaultReasoningEffort": "low", "supportedReasoningEfforts": []any{map[string]any{"effort": "low"}}},
			map[string]any{"id": "gpt-mini", "displayName": "GPT Mini", "isDefault": false},
		}, "nextCursor": nil})
	}
}

func streamRequest(requestID, chatSessionID, permissionMode string) ExternalAgentStreamRequest {
	return ExternalAgentStreamRequest{
		RequestID:      requestID,
		ChatSessionID:  chatSessionID,
		SDKBackend:     "codex",
		Prompt:         "say PONG",
		CWD:            "C:\\work",
		PermissionMode: permissionMode,
		CodexRuntime:   "app-server",
	}
}

func TestCodexAppServerStreamRunsProtocolTurn(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	var threadStartParams map[string]any
	var turnStartParams map[string]any
	service, fake, recorder := newServiceWithFakeAppServer(t, func(fake *fakeAppServer, req fakeRequest) {
		switch req.method {
		case "thread/start":
			threadStartParams = req.params
			defaultAppServerHandle(fake, req)
		case "turn/start":
			turnStartParams = req.params
			defaultAppServerHandle(fake, req)
		default:
			defaultAppServerHandle(fake, req)
		}
	})

	cleanupCalled := false
	result, handled := service.streamViaCodexAppServer(streamRequest("req-1", "chat-1", "confirm"), "codex-fake", "say PONG", nil, func() { cleanupCalled = true })
	if !handled || !result.OK {
		t.Fatalf("stream should be handled by app-server: handled=%v result=%+v", handled, result)
	}

	// thread/start must carry the confirm-mode fail-closed policy.
	if threadStartParams["approvalPolicy"] != "on-request" || threadStartParams["sandbox"] != "read-only" {
		t.Fatalf("confirm mode mapping wrong: %v", threadStartParams)
	}
	if threadStartParams["cwd"] != "C:\\work" {
		t.Fatalf("cwd not forwarded: %v", threadStartParams)
	}
	if turnStartParams["threadId"] != "thread-1" {
		t.Fatalf("turn not bound to thread: %v", turnStartParams)
	}
	input, _ := turnStartParams["input"].([]any)
	if len(input) == 0 || input[0].(map[string]any)["text"] != "say PONG" {
		t.Fatalf("prompt not forwarded as text input: %v", turnStartParams)
	}

	// session-id event carries the protocol thread and runtime marker.
	sessionEvent := waitForCodexEvent(t, recorder, "ai:sdk-agent:event", 2*time.Second)
	session := sessionEvent.payload["event"].(map[string]any)
	if session["type"] != "session-id" || session["sessionId"] != "thread-1" || session["runtime"] != "app-server" {
		t.Fatalf("session-id event mismatch: %v", session)
	}

	// Streamed agent deltas surface as text-delta events.
	fake.notify(t, "item/agentMessage/delta", map[string]any{"threadId": "thread-1", "turnId": "turn-1", "itemId": "m1", "delta": "PONG"})
	fake.notify(t, "thread/tokenUsage/updated", map[string]any{"threadId": "thread-1", "tokenUsage": map[string]any{"total": map[string]any{"inputTokens": 10, "outputTokens": 5, "totalTokens": 15}}})
	fake.notify(t, "turn/completed", map[string]any{"threadId": "thread-1", "turn": map[string]any{"id": "turn-1", "status": "completed"}})

	done := waitForCodexEvent(t, recorder, "ai:sdk-agent:done", 2*time.Second)
	if done.payload["requestId"] != "req-1" {
		t.Fatalf("done event bound to wrong request: %v", done.payload)
	}
	sawDelta, sawUsage := false, false
	for _, event := range recorder.all() {
		if event.name != "ai:sdk-agent:event" {
			continue
		}
		payload := event.payload["event"].(map[string]any)
		if payload["type"] == "text-delta" && payload["textDelta"] == "PONG" {
			sawDelta = true
		}
		if payload["type"] == "usage" && payload["totalTokens"] == float64(15) {
			sawUsage = true
		}
	}
	if !sawDelta || !sawUsage {
		t.Fatalf("text-delta/usage events missing: %v", recorder.all())
	}
	if !cleanupCalled {
		t.Fatalf("attachment cleanup did not run after turn completion")
	}
}

func TestCodexAppServerStreamResumeFallsBackToFreshThread(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	var resumeParams map[string]any
	service, _, recorder := newServiceWithFakeAppServer(t, func(fake *fakeAppServer, req fakeRequest) {
		switch req.method {
		case "thread/resume":
			resumeParams = req.params
			_ = fakeWriteError(fake, req.id, -32600, "invalid thread id")
		default:
			defaultAppServerHandle(fake, req)
		}
	})

	request := streamRequest("req-1", "chat-1", "confirm")
	request.ExistingSessionID = "stale-thread"
	result, handled := service.streamViaCodexAppServer(request, "codex-fake", "go", nil, func() {})
	if !handled || !result.OK {
		t.Fatalf("stream should still succeed after resume failure: %+v", result)
	}
	if resumeParams["threadId"] != "stale-thread" {
		t.Fatalf("resume should pass the decoded session id: %v", resumeParams)
	}
	foundFallbackWarning := false
	for _, event := range recorder.all() {
		if event.name != "ai:sdk-agent:event" {
			continue
		}
		payload := event.payload["event"].(map[string]any)
		if payload["type"] == "warning" && strings.Contains(payload["message"].(string), "Could not resume Codex thread") {
			foundFallbackWarning = true
		}
	}
	if !foundFallbackWarning {
		t.Fatalf("resume fallback must be announced, got %v", recorder.all())
	}
}

func TestCodexAppServerStreamDegradesToExecOnLaunchFailure(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	service := newExternalAgentService("", t.TempDir())
	recorder := newCodexEventRecorder()
	service.setEventEmitter(recorder.emit)
	service.transportFactory = func(executable string, args []string, env []string, dir string) (codexAppServerTransport, error) {
		return nil, errFakeLaunch{}
	}

	result, handled := service.streamViaCodexAppServer(streamRequest("req-1", "chat-1", "confirm"), "codex-fake", "go", nil, func() {})
	if handled {
		t.Fatalf("launch failure must hand the request back to exec mode")
	}
	if result.OK {
		t.Fatalf("no fake success allowed on degradation")
	}
	status := waitForCodexEvent(t, recorder, "ai:sdk-agent:event", 2*time.Second)
	payload := status.payload["event"].(map[string]any)
	if payload["type"] != "status" || !strings.Contains(payload["message"].(string), "falling back to CLI exec mode") {
		t.Fatalf("degradation must be announced on the stream: %v", payload)
	}
}

type errFakeLaunch struct{}

func (errFakeLaunch) Error() string { return "codex executable exploded" }

func TestCodexAppServerPermissionModeMapping(t *testing.T) {
	cases := []struct {
		mode           string
		approvalPolicy string
		sandbox        string
	}{
		{"observer", "never", "read-only"},
		{"confirm", "on-request", "read-only"},
		{"auto", "never", "danger-full-access"},
		{"", "on-request", "read-only"},
	}
	for _, tc := range cases {
		policy, sandbox := codexAppServerApproval(tc.mode)
		if policy != tc.approvalPolicy || sandbox != tc.sandbox {
			t.Fatalf("mode %q: got (%q,%q) want (%q,%q)", tc.mode, policy, sandbox, tc.approvalPolicy, tc.sandbox)
		}
	}
}

func TestCodexAppServerApprovalInteractionRoundTrip(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	service, fake, recorder := newServiceWithFakeAppServer(t, defaultAppServerHandle)
	if _, handled := service.streamViaCodexAppServer(streamRequest("req-1", "chat-1", "confirm"), "codex-fake", "go", nil, func() {}); !handled {
		t.Fatalf("stream setup failed")
	}

	fake.serverRequest(t, 21, "item/commandExecution/requestApproval", map[string]any{
		"threadId": "thread-1", "turnId": "turn-1", "itemId": "item-9", "command": "rm -rf /",
		"cwd": "C:\\work",
	})

	interactionEvent := waitForCodexEvent(t, recorder, codexAppServerInteractionEvent, 2*time.Second)
	interaction := interactionEvent.payload
	if interaction["interactionId"] == "" || interaction["source"] != "codex-app-server" || interaction["kind"] != "command" {
		t.Fatalf("interaction payload mismatch: %v", interaction)
	}
	if interaction["chatSessionId"] != "chat-1" || interaction["toolName"] != "rm -rf /" || interaction["itemId"] != "item-9" {
		t.Fatalf("interaction routing fields mismatch: %v", interaction)
	}
	decisions, _ := interaction["availableDecisions"].([]string)
	if len(decisions) == 0 || decisions[0] != "accept" {
		t.Fatalf("availableDecisions missing: %v", interaction)
	}
	interactionID := interaction["interactionId"].(string)

	if result := service.RespondCodexAppServerInteraction(map[string]any{"interactionId": interactionID, "decision": "once"}); !result["ok"].(bool) {
		t.Fatalf("respond failed: %v", result)
	}
	response := waitForChannel(t, fake.responses, 2*time.Second)
	if response.id != 21 || !strings.Contains(string(response.result), `"accept"`) {
		t.Fatalf("server response mismatch: %+v", response)
	}
	cleared := waitForCodexEvent(t, recorder, codexAppServerInteractionClearedEvent, 2*time.Second)
	ids, _ := cleared.payload["interactionIds"].([]string)
	if len(ids) != 1 || ids[0] != interactionID {
		t.Fatalf("cleared event mismatch: %v", cleared.payload)
	}
	// Double respond must fail typed (already resolved).
	if result := service.RespondCodexAppServerInteraction(map[string]any{"interactionId": interactionID, "decision": "once"}); result["ok"] != false {
		t.Fatalf("duplicate respond must fail: %v", result)
	}
}

func TestCodexAppServerUserInputInteractionRoundTrip(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	service, fake, recorder := newServiceWithFakeAppServer(t, defaultAppServerHandle)
	if _, handled := service.streamViaCodexAppServer(streamRequest("req-1", "chat-1", "confirm"), "codex-fake", "go", nil, func() {}); !handled {
		t.Fatalf("stream setup failed")
	}

	fake.serverRequest(t, 22, "item/tool/requestUserInput", map[string]any{
		"threadId": "thread-1", "turnId": "turn-1", "itemId": "q-1",
		"questions": []any{map[string]any{"id": "q1", "header": "Deploy", "question": "Proceed?", "options": []any{map[string]any{"label": "Yes"}}}},
	})
	interaction := waitForCodexEvent(t, recorder, codexAppServerInteractionEvent, 2*time.Second)
	if interaction.payload["kind"] != "user-input" {
		t.Fatalf("expected user-input interaction: %v", interaction.payload)
	}
	questions, _ := interaction.payload["questions"].([]any)
	if len(questions) != 1 || questions[0].(map[string]any)["id"] != "q1" {
		t.Fatalf("questions not forwarded: %v", interaction.payload)
	}
	interactionID := interaction.payload["interactionId"].(string)

	answers := map[string]any{"q1": map[string]any{"answers": []any{"Yes"}}}
	if result := service.RespondCodexAppServerInteraction(map[string]any{"interactionId": interactionID, "answers": answers}); !result["ok"].(bool) {
		t.Fatalf("respond failed: %v", result)
	}
	response := waitForChannel(t, fake.responses, 2*time.Second)
	var payload map[string]any
	if err := json.Unmarshal(response.result, &payload); err != nil {
		t.Fatalf("response decode: %v", err)
	}
	inner := payload["answers"].(map[string]any)["q1"].(map[string]any)["answers"].([]any)
	if len(inner) != 1 || inner[0] != "Yes" {
		t.Fatalf("answers not passed through: %v", payload)
	}
}

func TestCodexAppServerPermissionsInteractionGrantsRequestedProfile(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	service, fake, recorder := newServiceWithFakeAppServer(t, defaultAppServerHandle)
	if _, handled := service.streamViaCodexAppServer(streamRequest("req-1", "chat-1", "confirm"), "codex-fake", "go", nil, func() {}); !handled {
		t.Fatalf("stream setup failed")
	}

	requested := map[string]any{"fileSystem": map[string]any{"write": []any{"C:\\work"}}}
	fake.serverRequest(t, 23, "item/permissions/requestApproval", map[string]any{
		"threadId": "thread-1", "turnId": "turn-1", "itemId": "p-1", "permissions": requested, "cwd": "C:\\work",
	})
	interaction := waitForCodexEvent(t, recorder, codexAppServerInteractionEvent, 2*time.Second)
	interactionID := interaction.payload["interactionId"].(string)

	if result := service.RespondCodexAppServerInteraction(map[string]any{"interactionId": interactionID, "decision": "session"}); !result["ok"].(bool) {
		t.Fatalf("respond failed: %v", result)
	}
	response := waitForChannel(t, fake.responses, 2*time.Second)
	var payload map[string]any
	if err := json.Unmarshal(response.result, &payload); err != nil {
		t.Fatalf("response decode: %v", err)
	}
	if payload["scope"] != "session" {
		t.Fatalf("scope mismatch: %v", payload)
	}
	if _, ok := payload["permissions"].(map[string]any); !ok {
		t.Fatalf("requested permissions not echoed: %v", payload)
	}

	// A rejection answers with a JSON-RPC error (server treats as denial).
	fake.serverRequest(t, 24, "item/permissions/requestApproval", map[string]any{
		"threadId": "thread-1", "turnId": "turn-1", "itemId": "p-2", "permissions": requested,
	})
	interaction2 := waitForCodexEvent(t, recorder, codexAppServerInteractionEvent, 2*time.Second)
	if result := service.RespondCodexAppServerInteraction(map[string]any{"interactionId": interaction2.payload["interactionId"].(string), "decision": "reject"}); !result["ok"].(bool) {
		t.Fatalf("reject respond failed: %v", result)
	}
	response2 := waitForChannel(t, fake.responses, 2*time.Second)
	if response2.id != 24 || response2.errMsg == "" {
		t.Fatalf("rejection must be a typed error: %+v", response2)
	}
}

func TestCodexAppServerSteerInjectsMidTurn(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	var steerParams map[string]any
	service, _, _ := newServiceWithFakeAppServer(t, func(fake *fakeAppServer, req fakeRequest) {
		if req.method == "turn/steer" {
			steerParams = req.params
		}
		defaultAppServerHandle(fake, req)
	})
	if _, handled := service.streamViaCodexAppServer(streamRequest("req-1", "chat-1", "confirm"), "codex-fake", "go", nil, func() {}); !handled {
		t.Fatalf("stream setup failed")
	}

	result := service.Steer("req-1", "chat-1", "also do X", nil, "msg-1")
	if result.Status != "accepted" {
		t.Fatalf("steer should be accepted via protocol: %+v", result)
	}
	if steerParams["expectedTurnId"] != "turn-1" || steerParams["threadId"] != "thread-1" {
		t.Fatalf("turn/steer params mismatch: %v", steerParams)
	}

	// Wrong chat session must not steer.
	if result := service.Steer("req-1", "other-chat", "nope", nil, ""); result.Status != "inactive" {
		t.Fatalf("steer for foreign session should be inactive: %+v", result)
	}
}

func TestCodexAppServerCancelInterruptsTurnKeepsProcessAlive(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	var interrupted map[string]any
	service, fake, recorder := newServiceWithFakeAppServer(t, func(fake *fakeAppServer, req fakeRequest) {
		if req.method == "turn/interrupt" {
			interrupted = req.params
		}
		defaultAppServerHandle(fake, req)
	})
	if _, handled := service.streamViaCodexAppServer(streamRequest("req-1", "chat-1", "confirm"), "codex-fake", "go", nil, func() {}); !handled {
		t.Fatalf("stream setup failed")
	}

	if result := service.Cancel("req-1", "chat-1"); !result.OK {
		t.Fatalf("cancel failed: %+v", result)
	}
	if interrupted["threadId"] != "thread-1" || interrupted["turnId"] != "turn-1" {
		t.Fatalf("turn/interrupt params mismatch: %v", interrupted)
	}
	waitForCodexEvent(t, recorder, "ai:sdk-agent:done", 2*time.Second)
	if fake.transport.killCalls() != 0 {
		t.Fatalf("cancel must not kill the shared app-server process")
	}
}

func TestCodexAppServerStatusRequiresRealHandshake(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	// Happy path: handshake succeeds → OK. The agentCommand points at a temp
	// file so resolveExecutable accepts it as the "codex" binary.
	service, _, _ := newServiceWithFakeAppServer(t, defaultAppServerHandle)
	if result := service.CodexAppServerStatus(fakeExecutablePath(t), nil); !result.OK {
		t.Fatalf("handshake success should report OK: %+v", result)
	}

	// Failure: server never answers initialize → typed error, never OK.
	failing := newFakeAppServer(t, nil) // replies to nothing
	t.Cleanup(func() { failing.close() })
	service2 := newExternalAgentService("", t.TempDir())
	service2.transportFactory = func(executable string, args []string, env []string, dir string) (codexAppServerTransport, error) {
		return failing.transport, nil
	}
	result := service2.CodexAppServerStatus(fakeExecutablePath(t), nil)
	if result.OK {
		t.Fatalf("unanswered handshake must not report OK: %+v", result)
	}
	if !strings.Contains(result.Error, "handshake failed") {
		t.Fatalf("status error should explain the failure: %+v", result)
	}
}

func TestCodexAppServerListModelsQueriesProtocolCatalog(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	var listed bool
	service, _, _ := newServiceWithFakeAppServer(t, func(fake *fakeAppServer, req fakeRequest) {
		if req.method == "model/list" {
			listed = true
		}
		defaultAppServerHandle(fake, req)
	})

	result := service.ListModels("codex", "", "", "models_codex", nil, fakeExecutablePath(t), "app-server")
	if !result.OK || !listed {
		t.Fatalf("model list should query the protocol: %+v listed=%v", result, listed)
	}
	if len(result.Models) != 2 {
		t.Fatalf("unexpected model count: %+v", result.Models)
	}
	if result.Models[0]["id"] != "gpt-test" || result.Models[0]["name"] != "GPT Test" {
		t.Fatalf("model mapping mismatch: %+v", result.Models[0])
	}
	if result.Models[0]["defaultThinkingLevel"] != "low" {
		t.Fatalf("reasoning efforts not mapped: %+v", result.Models[0])
	}
	if result.CurrentModelID != "gpt-test" {
		t.Fatalf("default model not detected: %+v", result)
	}
	if result.Warning != "" {
		t.Fatalf("healthy catalog must not warn: %q", result.Warning)
	}

	// Non-codex backends keep the preset fallback behaviour.
	fallback := service.ListModels("claude", "", "", "", nil, "", "")
	if !fallback.OK || fallback.Warning == "" || len(fallback.Models) != 0 {
		t.Fatalf("non-codex backends must keep preset fallback: %+v", fallback)
	}
}

func TestCodexAppServerInteractionTimeoutDeniesAndClears(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	service, fake, recorder := newServiceWithFakeAppServer(t, defaultAppServerHandle)
	if _, handled := service.streamViaCodexAppServer(streamRequest("req-1", "chat-1", "confirm"), "codex-fake", "go", nil, func() {}); !handled {
		t.Fatalf("stream setup failed")
	}

	fake.serverRequest(t, 31, "item/commandExecution/requestApproval", map[string]any{"threadId": "thread-1", "turnId": "turn-1", "itemId": "i-1", "command": "make"})
	interaction := waitForCodexEvent(t, recorder, codexAppServerInteractionEvent, 2*time.Second)
	interactionID := interaction.payload["interactionId"].(string)

	// Renderer takes ownership: cancel the timeout → server stays pending.
	if result := service.CancelCodexAppServerInteractionTimeout(interactionID); result["ok"] != true || result["cancelled"] != true {
		t.Fatalf("cancel timeout failed: %v", result)
	}

	fake.serverRequest(t, 32, "item/commandExecution/requestApproval", map[string]any{"threadId": "thread-1", "turnId": "turn-1", "itemId": "i-2", "command": "make"})
	interaction2 := waitForCodexEvent(t, recorder, codexAppServerInteractionEvent, 2*time.Second)
	service.timeoutCodexAppServerInteraction(interaction2.payload["interactionId"].(string))
	response := waitForChannel(t, fake.responses, 2*time.Second)
	if response.id != 32 || response.errMsg == "" {
		t.Fatalf("timeout must deny the server request: %+v", response)
	}
	cleared := waitForCodexEvent(t, recorder, codexAppServerInteractionClearedEvent, 2*time.Second)
	if cleared.payload["chatSessionId"] != "chat-1" {
		t.Fatalf("cleared event must carry the chat session: %v", cleared.payload)
	}
}

func TestCodexAppServerRunFinishesOnErrorTurn(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	service, fake, recorder := newServiceWithFakeAppServer(t, defaultAppServerHandle)
	if _, handled := service.streamViaCodexAppServer(streamRequest("req-1", "chat-1", "confirm"), "codex-fake", "go", nil, func() {}); !handled {
		t.Fatalf("stream setup failed")
	}

	fake.notify(t, "turn/completed", map[string]any{"threadId": "thread-1", "turn": map[string]any{
		"id": "turn-1", "status": "failed", "error": map[string]any{"message": "quota exceeded"},
	}})
	failed := waitForCodexEvent(t, recorder, "ai:sdk-agent:error", 2*time.Second)
	if failed.payload["requestId"] != "req-1" || !strings.Contains(failed.payload["error"].(string), "quota exceeded") {
		t.Fatalf("failed turn must error the stream: %v", failed.payload)
	}
}

func TestCodexAppServerRespondApprovalDecisionTable(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	service, _, _ := newServiceWithFakeAppServer(t, defaultAppServerHandle)
	cases := []struct {
		method   string
		decision string
		want     string
	}{
		{"item/commandExecution/requestApproval", "once", "accept"},
		{"item/commandExecution/requestApproval", "session", "acceptForSession"},
		{"item/commandExecution/requestApproval", "reject", "decline"},
		{"item/commandExecution/requestApproval", "cancel", "cancel"},
		{"execCommandApproval", "once", "approved"},
		{"execCommandApproval", "session", "approved_for_session"},
		{"execCommandApproval", "reject", "denied"},
		{"execCommandApproval", "cancel", "abort"},
		{"applyPatchApproval", "once", "approved"},
		{"item/fileChange/requestApproval", "session", "acceptForSession"},
	}
	for _, tc := range cases {
		pending := &codexAppServerInteraction{
			method: tc.method,
			client: &codexAppServerClient{pending: map[int64]chan *codexAppServerMessage{}, exited: make(chan struct{})},
		}
		// A closed stub client fails writes; capture the marshalled intent
		// instead via a pipe-backed fake.
		fake := newFakeAppServer(t, nil)
		pending.client = fake.client
		if err := service.respondCodexApproval(pending, tc.decision); err != nil {
			t.Fatalf("%s/%s: respond failed: %v", tc.method, tc.decision, err)
		}
		response := waitForChannel(t, fake.responses, time.Second)
		if !strings.Contains(string(response.result), `"`+tc.want+`"`) {
			t.Fatalf("%s/%s: want decision %q got %s", tc.method, tc.decision, tc.want, response.result)
		}
		fake.close()
	}
	// Unknown decisions fail typed instead of silently approving.
	pending := &codexAppServerInteraction{method: "item/commandExecution/requestApproval"}
	fake := newFakeAppServer(t, nil)
	pending.client = fake.client
	if err := service.respondCodexApproval(pending, "yolo"); err == nil {
		t.Fatalf("unknown decision must fail typed")
	}
	fake.close()
}

func TestCodexAppServerLegacyConversationIdRouting(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	service, fake, recorder := newServiceWithFakeAppServer(t, defaultAppServerHandle)
	if _, handled := service.streamViaCodexAppServer(streamRequest("req-1", "chat-1", "confirm"), "codex-fake", "go", nil, func() {}); !handled {
		t.Fatalf("stream setup failed")
	}

	// Legacy execCommandApproval uses conversationId instead of threadId.
	fake.serverRequest(t, 41, "execCommandApproval", map[string]any{
		"conversationId": "thread-1", "callId": "call-7", "command": []any{"git", "status"}, "cwd": "C:\\work",
	})
	interaction := waitForCodexEvent(t, recorder, codexAppServerInteractionEvent, 2*time.Second)
	if interaction.payload["kind"] != "command" || interaction.payload["chatSessionId"] != "chat-1" {
		t.Fatalf("legacy approval not routed by conversationId: %v", interaction.payload)
	}
	if interaction.payload["itemId"] != "call-7" {
		t.Fatalf("legacy callId not used as itemId: %v", interaction.payload)
	}
	if result := service.RespondCodexAppServerInteraction(map[string]any{"interactionId": interaction.payload["interactionId"].(string), "decision": "reject"}); !result["ok"].(bool) {
		t.Fatalf("respond failed: %v", result)
	}
	response := waitForChannel(t, fake.responses, 2*time.Second)
	if !strings.Contains(string(response.result), "denied") {
		t.Fatalf("legacy decision vocabulary expected: %s", response.result)
	}
}

func TestCodexAppServerUnsupportedServerRequestAnswersTyped(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	service, fake, recorder := newServiceWithFakeAppServer(t, defaultAppServerHandle)
	if _, handled := service.streamViaCodexAppServer(streamRequest("req-1", "chat-1", "confirm"), "codex-fake", "go", nil, func() {}); !handled {
		t.Fatalf("stream setup failed")
	}

	fake.serverRequest(t, 51, "item/tool/call", map[string]any{"threadId": "thread-1"})
	response := waitForChannel(t, fake.responses, 2*time.Second)
	if response.id != 51 || response.errMsg == "" {
		t.Fatalf("unsupported server call must fail typed: %+v", response)
	}
	// And the stream stays healthy: no interaction event was emitted.
	for _, event := range recorder.all() {
		if event.name == codexAppServerInteractionEvent {
			t.Fatalf("no interaction should be created for unsupported calls")
		}
	}
}

// TestCodexAppServerRealCLISmoke exercises the client against the real
// `codex app-server` binary when LEMONSSH_CODEX_SMOKE_EXE points at it.
// NETCATTY_CODEX_SMOKE_ARGS appends extra CLI arguments (e.g. config
// overrides like `-c service_tier=fast` for machines whose config.toml uses
// newer enum values than the CLI understands). Skipped by default (no codex
// dependency in CI); it only runs the local protocol steps (handshake,
// thread/start, model/list) — no billed model turn. Verified live against
// codex-cli 0.130.0-alpha.5 on Windows.
func TestCodexAppServerRealCLISmoke(t *testing.T) {
	executable := os.Getenv("LEMONSSH_CODEX_SMOKE_EXE")
	if executable == "" {
		t.Skip("LEMONSSH_CODEX_SMOKE_EXE not set; skipping real CLI smoke")
	}
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 30 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	// Status probe: plain launch args, exactly like production.
	service := newExternalAgentService("", t.TempDir())
	if result := service.CodexAppServerStatus(executable, nil); !result.OK {
		t.Fatalf("real handshake failed: %+v", result)
	}

	// Protocol flow: same client code path with optional config overrides.
	args := []string{"app-server"}
	if extra := strings.Fields(os.Getenv("NETCATTY_CODEX_SMOKE_ARGS")); len(extra) > 0 {
		args = append(args, extra...)
	}
	transport, err := launchCodexAppServerTransport(executable, args, sanitizeExternalEnv(nil), "")
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	client := newCodexAppServerClient(transport)
	client.Start()
	defer client.Close()
	if err := service.handshakeCodexAppServer(client); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	started, err := client.Call("thread/start", map[string]any{
		"approvalPolicy": "on-request",
		"sandbox":        "read-only",
	}, 30*time.Second)
	if err != nil {
		t.Fatalf("thread/start: %v", err)
	}
	threadID := stringAt(jsonPayload(started), "thread", "id")
	if threadID == "" {
		t.Fatalf("thread/start returned no thread id: %s", started)
	}
	list, err := client.Call("model/list", map[string]any{}, 30*time.Second)
	if err != nil {
		t.Fatalf("model/list: %v", err)
	}
	payload := jsonPayload(list)
	entries, _ := payload["data"].([]any)
	t.Logf("smoke ok: thread=%s models=%d", threadID, len(entries))
	if len(entries) == 0 {
		t.Fatalf("model/list returned an empty catalog: %s", list)
	}
}

// --- Regression tests for the independent review findings ---

// newServiceWithFakeServerPool launches a fresh fake per transportFactory
// call, for tests that exercise acquire recycling.
func newServiceWithFakeServerPool(t *testing.T, handle func(fake *fakeAppServer, req fakeRequest)) (*ExternalAgentService, func() []*fakeAppServer, *codexEventRecorder) {
	t.Helper()
	var mu sync.Mutex
	var launched []*fakeAppServer
	service := newExternalAgentService("", t.TempDir())
	service.transportFactory = func(executable string, args []string, env []string, dir string) (codexAppServerTransport, error) {
		var wrapped *fakeAppServer
		wrapped = newFakeAppServer(t, func(req fakeRequest) {
			if handle != nil {
				handle(wrapped, req)
			}
		})
		mu.Lock()
		launched = append(launched, wrapped)
		mu.Unlock()
		return wrapped.transport, nil
	}
	recorder := newCodexEventRecorder()
	service.setEventEmitter(recorder.emit)
	return service, func() []*fakeAppServer {
		mu.Lock()
		defer mu.Unlock()
		return append([]*fakeAppServer(nil), launched...)
	}, recorder
}

// Finding 1 (blocking): the TTL recycle branch must not call Close while
// holding appServerMu — Close runs onExit synchronously and onExit re-enters
// appServerMu. Before the fix this acquisition deadlocked permanently.
func TestCodexAppServerIdleTTLRecycleDoesNotDeadlock(t *testing.T) {
	originalTTL := codexAppServerIdleTTL
	codexAppServerIdleTTL = 50 * time.Millisecond
	t.Cleanup(func() { codexAppServerIdleTTL = originalTTL })

	service, launched, _ := newServiceWithFakeServerPool(t, defaultAppServerHandle)
	base := time.Now()
	current := base
	service.now = func() time.Time { return current }

	if _, err := service.acquireCodexAppServer("codex-fake", nil); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if got := len(launched()); got != 1 {
		t.Fatalf("expected 1 launch after first acquire, got %d", got)
	}

	current = base.Add(500 * time.Millisecond) // TTL elapsed
	acquired := make(chan error, 1)
	go func() {
		_, err := service.acquireCodexAppServer("codex-fake", nil)
		acquired <- err
	}()
	select {
	case err := <-acquired:
		if err != nil {
			t.Fatalf("second acquire failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("acquire deadlocked on the TTL recycle path")
	}
	if got := len(launched()); got != 2 {
		t.Fatalf("TTL recycle must launch a fresh process, launched=%d", got)
	}
	service.appServerMu.Lock()
	stillCached := service.appServerState.client != nil
	service.appServerMu.Unlock()
	if !stillCached {
		t.Fatalf("the fresh client should be cached")
	}
}

// Finding 2: finishCodexAppServerRun must be idempotent — the server's
// turn/completed{interrupted} can arrive while Cancel is still inside the
// turn/interrupt call, and the terminal stream event must be emitted once.
func TestCodexAppServerFinishIsIdempotent(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	service, _, recorder := newServiceWithFakeAppServer(t, func(fake *fakeAppServer, req fakeRequest) {
		if req.method == "turn/interrupt" {
			defaultAppServerHandle(fake, req)
			// Simulate the server's async turn/completed{interrupted} landing
			// while Cancel is still inside the interrupt call.
			fake.notify(t, "turn/completed", map[string]any{"threadId": "thread-1", "turn": map[string]any{"id": "turn-1", "status": "interrupted"}})
			return
		}
		defaultAppServerHandle(fake, req)
	})
	if _, handled := service.streamViaCodexAppServer(streamRequest("req-1", "chat-1", "confirm"), "codex-fake", "go", nil, func() {}); !handled {
		t.Fatalf("stream setup failed")
	}

	if result := service.Cancel("req-1", "chat-1"); !result.OK {
		t.Fatalf("cancel failed: %+v", result)
	}
	countDone := func() int {
		total := 0
		for _, event := range recorder.all() {
			if event.name == "ai:sdk-agent:done" {
				total++
			}
		}
		return total
	}
	deadline := time.After(2 * time.Second)
	for countDone() == 0 {
		select {
		case event := <-recorder.ch:
			_ = event
		case <-deadline:
			t.Fatalf("no done event after cancel")
		}
	}
	time.Sleep(100 * time.Millisecond)
	if done := countDone(); done != 1 {
		t.Fatalf("terminal done event must be emitted exactly once, got %d", done)
	}
}

// Finding 3: re-registering the same requestID over an app-server run must
// not nil-panic (protocol runs have no context cancel).
func TestCodexAppServerSupersededRunDoesNotPanic(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	var interrupts int
	service, _, recorder := newServiceWithFakeServerPool(t, func(fake *fakeAppServer, req fakeRequest) {
		if req.method == "turn/interrupt" {
			interrupts++
		}
		defaultAppServerHandle(fake, req)
	})
	if _, handled := service.streamViaCodexAppServer(streamRequest("req-1", "chat-1", "confirm"), "codex-fake", "go", nil, func() {}); !handled {
		t.Fatalf("first stream setup failed")
	}
	// Same requestID, new chat session: the registry's previous run is a
	// protocol run whose cancel field is nil.
	if _, handled := service.streamViaCodexAppServer(streamRequest("req-1", "chat-2", "confirm"), "codex-fake", "go again", nil, func() {}); !handled {
		t.Fatalf("second stream setup failed")
	}
	if interrupts != 1 {
		t.Fatalf("the superseded run's turn should be interrupted once, got %d", interrupts)
	}
	doneForOldRequest := false
	for _, event := range recorder.all() {
		if event.name == "ai:sdk-agent:done" && event.payload["requestId"] == "req-1" {
			doneForOldRequest = true
		}
	}
	if !doneForOldRequest {
		t.Fatalf("the superseded run's stream should settle with done")
	}
	service.mu.Lock()
	active := service.active["req-1"]
	service.mu.Unlock()
	if active == nil || active.chatSessionID != "chat-2" {
		t.Fatalf("the registry should hold the new run: %+v", active)
	}
}

// Finding 4: a cached client must only be reused when its executable matches;
// switching the codex CLI path must replace the process.
func TestCodexAppServerAcquirePrefersMatchingExecutable(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	service, launched, _ := newServiceWithFakeServerPool(t, defaultAppServerHandle)

	if _, err := service.acquireCodexAppServer("codex-a", nil); err != nil {
		t.Fatalf("acquire a: %v", err)
	}
	if _, err := service.acquireCodexAppServer("codex-b", nil); err != nil {
		t.Fatalf("acquire b: %v", err)
	}
	clients := launched()
	if len(clients) != 2 {
		t.Fatalf("executable switch must launch a new process, launched=%d", len(clients))
	}
	if clients[0].transport.killCalls() != 1 {
		t.Fatalf("the incumbent process must be closed exactly once, killCalls=%d", clients[0].transport.killCalls())
	}
	service.appServerMu.Lock()
	executable := service.appServerState.executable
	service.appServerMu.Unlock()
	if executable != "codex-b" {
		t.Fatalf("cache should track the new executable, got %q", executable)
	}
	// Re-acquiring the same executable must reuse, not relaunch.
	if _, err := service.acquireCodexAppServer("codex-b", nil); err != nil {
		t.Fatalf("re-acquire b: %v", err)
	}
	if len(launched()) != 2 {
		t.Fatalf("same executable must reuse the cached process")
	}
}

// Finding 5: the cached process must only be reused when the launch
// environment matches, so per-session AgentEnv is never silently ignored.
func TestCodexAppServerDifferentChatCredentialsKeepActiveTurnsIsolated(t *testing.T) {
	service, launched, recorder := newServiceWithFakeServerPool(t, defaultAppServerHandle)
	a := streamRequest("req-a", "chat-a", "confirm")
	a.AgentEnv = map[string]string{"LEMONSSH_TOOL_CLI_DISCOVERY_FILE": "chat-a.json"}
	b := streamRequest("req-b", "chat-b", "confirm")
	b.AgentEnv = map[string]string{"LEMONSSH_TOOL_CLI_DISCOVERY_FILE": "chat-b.json"}
	if result, _ := service.streamViaCodexAppServer(a, "codex", "a", nil, func() {}); !result.OK {
		t.Fatal(result)
	}
	if result, _ := service.streamViaCodexAppServer(b, "codex", "b", nil, func() {}); !result.OK {
		t.Fatal(result)
	}
	clients := launched()
	if len(clients) != 2 || clients[0].transport.killCalls() != 0 {
		t.Fatal("new chat killed the active previous chat")
	}
	service.appServerMu.Lock()
	runs := len(service.appServerState.runs)
	service.appServerMu.Unlock()
	if runs != 2 {
		t.Fatalf("same thread ids on different processes collided: %d", runs)
	}
	clients[0].notify(t, "item/agentMessage/delta", map[string]any{"threadId": "thread-1", "delta": "only-a"})
	deadline := time.Now().Add(time.Second)
	for {
		matched := false
		for _, event := range recorder.all() {
			if stringAt(event.payload, "event", "textDelta") == "only-a" {
				if event.payload["requestId"] != "req-a" {
					t.Fatal("text crossed chats")
				}
				matched = true
			}
		}
		if matched {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no delta received")
		}
		time.Sleep(time.Millisecond)
	}
	service.Cancel("req-a", "chat-a")
	if clients[0].transport.killCalls() != 1 || clients[1].transport.killCalls() != 0 {
		t.Fatal("inactive client cleanup touched another chat")
	}
	service.Cancel("req-b", "chat-b")
}

func TestCodexAppServerAcquireKeysOnAgentEnv(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	service, launched, _ := newServiceWithFakeServerPool(t, defaultAppServerHandle)

	if _, err := service.acquireCodexAppServer("codex-fake", map[string]string{"SESSION_TOKEN": "a"}); err != nil {
		t.Fatalf("acquire 1: %v", err)
	}
	if _, err := service.acquireCodexAppServer("codex-fake", map[string]string{"SESSION_TOKEN": "a"}); err != nil {
		t.Fatalf("acquire 2: %v", err)
	}
	if len(launched()) != 1 {
		t.Fatalf("identical env must reuse the cached process, launched=%d", len(launched()))
	}
	if _, err := service.acquireCodexAppServer("codex-fake", map[string]string{"SESSION_TOKEN": "b"}); err != nil {
		t.Fatalf("acquire 3: %v", err)
	}
	if len(launched()) != 2 {
		t.Fatalf("changed env must relaunch, launched=%d", len(launched()))
	}
}

// Finding 6: ListModels must only ride the protocol in app-server mode; sdk
// mode keeps the preset fallback without spawning a subprocess.
func TestCodexAppServerListModelsRespectsRuntime(t *testing.T) {
	original := codexAppServerHandshakeTimeout
	codexAppServerHandshakeTimeout = 5 * time.Second
	t.Cleanup(func() { codexAppServerHandshakeTimeout = original })

	service, launched, _ := newServiceWithFakeServerPool(t, defaultAppServerHandle)
	executable := fakeExecutablePath(t)

	sdkResult := service.ListModels("codex", "", "", "models_codex", nil, executable, "sdk")
	if len(launched()) != 0 {
		t.Fatalf("sdk mode must not launch the app-server protocol, launched=%d", len(launched()))
	}
	if !sdkResult.OK || sdkResult.Warning == "" || len(sdkResult.Models) != 0 {
		t.Fatalf("sdk mode must keep the preset fallback: %+v", sdkResult)
	}

	appServerResult := service.ListModels("codex", "", "", "models_codex", nil, executable, "app-server")
	if len(launched()) != 1 {
		t.Fatalf("app-server mode must query the protocol once, launched=%d", len(launched()))
	}
	if !appServerResult.OK || len(appServerResult.Models) != 2 || appServerResult.CurrentModelID != "gpt-test" {
		t.Fatalf("app-server mode must return the catalog: %+v", appServerResult)
	}
}
