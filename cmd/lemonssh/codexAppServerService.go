package main

// Codex App Server session management for ExternalAgentService.
//
// When the frontend selects codexRuntime=="app-server" for the codex backend,
// Stream/Steer/ListModels/CodexAppServerStatus speak the app-server JSON-RPC
// protocol (see codexAppServerClient.go for the protocol evidence) instead of
// the one-shot `codex exec` CLI. One long-lived `codex app-server` child
// process is shared by all turns; turns map to protocol threads.
//
// Approval / user-input requests the server sends mid-turn are re-emitted as
// Wails events (ai:codex-app-server:interaction, cleared via
// ai:codex-app-server:interaction-cleared) and answered through the
// RespondCodexAppServerInteraction / CancelCodexAppServerInteractionTimeout
// bindings. The Wails runtime adapter (infrastructure/runtime/wails/
// externalAgentBridge.ts) maps those onto window.lemonssh for
// infrastructure/ai/shared/codexAppServerInteractions.ts — keep the event
// names in sync with that adapter.

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	// Wails event names consumed by externalAgentBridge.ts.
	codexAppServerInteractionEvent        = "ai:codex-app-server:interaction"
	codexAppServerInteractionClearedEvent = "ai:codex-app-server:interaction-cleared"
)

// Protocol timeouts are vars so tests can shorten them.
var (
	codexAppServerHandshakeTimeout   = 20 * time.Second
	codexAppServerRequestTimeout     = 30 * time.Second
	codexAppServerTurnStartTimeout   = 60 * time.Second
	codexAppServerIdleTTL            = 30 * time.Minute
	codexAppServerInteractionTimeout = 10 * time.Minute
)

// codexAppServerApproval maps the LemonSSH permission modes onto the protocol
// approvalPolicy/sandbox pair. Observer/Confirm stay fail-closed (read-only
// sandbox) exactly like exec mode; Confirm additionally routes approval
// requests to the user via on-request.
func codexAppServerApproval(permissionMode string) (approvalPolicy, sandbox string) {
	switch permissionMode {
	case "observer":
		return "never", "read-only"
	case "auto":
		return "never", "danger-full-access"
	default:
		return "on-request", "read-only"
	}
}

// codexAppServerInteraction is one pending server→client request awaiting a
// renderer decision.
type codexAppServerInteraction struct {
	interactionID string
	rpcID         int64
	method        string
	kind          string // command | file-change | permissions | user-input
	threadID      string
	chatSessionID string
	params        map[string]any
	client        *codexAppServerClient
	timer         *time.Timer
}

// codexAppServerState is the shared app-server runtime owned by the service.
// Everything is guarded by appServerMu; the handshake and protocol calls run
// WITHOUT the lock so inbound dispatch can always make progress.
type codexAppServerState struct {
	client     *codexAppServerClient
	executable string
	envKey     string // canonical AgentEnv snapshot the process was launched with
	lastUsed   time.Time
	runs       map[string]*externalAgentRun // threadID → active run
	pending    map[string]*codexAppServerInteraction
	seq        int
	launchErr  string // last launch/handshake failure, for status reporting
}

// codexAppServerEnvKey renders the request-derived environment into a
// comparable snapshot so a cached process is only reused when its launch
// environment (per-session proxy/credential variables included) matches.
func (s *ExternalAgentService) codexAppServerEnvKey(agentEnv map[string]string) string {
	env := make(map[string]string, len(agentEnv)+2)
	for key, value := range agentEnv {
		env[key] = value
	}
	if s.discoveryPath != "" && env["LEMONSSH_TOOL_CLI_DISCOVERY_FILE"] == "" {
		env["LEMONSSH_TOOL_CLI_DISCOVERY_FILE"] = s.discoveryPath
		env["NETCATTY_TOOL_CLI_DISCOVERY_FILE"] = s.discoveryPath
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, key+"="+env[key])
	}
	return strings.Join(pairs, "\n")
}

// acquireCodexAppServer returns the shared protocol client, launching and
// initializing a fresh `codex app-server` process when needed. Idle clients
// past the TTL are recycled, and a cached client is only reused when both its
// executable and launch environment match the request.
//
// Locking rule: Close() runs onExit synchronously and onExit re-enters
// appServerMu (handleCodexAppServerExit), so Close must NEVER be called while
// holding appServerMu — stale clients are detached from the state under the
// lock first and closed after unlocking.
func (s *ExternalAgentService) acquireCodexAppServer(executable string, agentEnv map[string]string) (*codexAppServerClient, error) {
	envKey := s.codexAppServerEnvKey(agentEnv)

	var recycle *codexAppServerClient
	s.appServerMu.Lock()
	if client := s.appServerState.client; client != nil && s.appServerState.executable == executable && s.appServerState.envKey == envKey && !client.IsExited() {
		if s.now().Sub(s.appServerState.lastUsed) > codexAppServerIdleTTL {
			// Detach under the lock, close after unlocking (see locking rule).
			recycle = client
			s.resetCodexAppServerLocked()
		} else {
			s.appServerState.lastUsed = s.now()
			s.appServerState.launchErr = ""
			s.appServerMu.Unlock()
			return client, nil
		}
	}
	s.appServerMu.Unlock()
	if recycle != nil {
		recycle.Close()
	}

	client, err := s.launchCodexAppServer(executable, agentEnv)
	if err != nil {
		s.appServerMu.Lock()
		s.appServerState.launchErr = err.Error()
		s.appServerMu.Unlock()
		return nil, err
	}
	if err := s.handshakeCodexAppServer(client); err != nil {
		client.Close()
		message := err.Error()
		if tail := client.StderrText(); tail != "" {
			message = message + ": " + firstLines(tail, 4)
		}
		s.appServerMu.Lock()
		s.appServerState.launchErr = message
		s.appServerMu.Unlock()
		return nil, err
	}

	var stale *codexAppServerClient
	s.appServerMu.Lock()
	existing := s.appServerState.client
	if existing != nil && !existing.IsExited() && s.appServerState.executable == executable && s.appServerState.envKey == envKey {
		// A concurrent acquire won the race; discard our surplus process.
		s.appServerMu.Unlock()
		client.Close()
		return existing, nil
	}
	// Install ours, replacing a stale or mismatched incumbent (executable or
	// env changed). The incumbent is closed after unlocking.
	stale = existing
	s.appServerState.client = client
	s.appServerState.executable = executable
	s.appServerState.envKey = envKey
	s.appServerState.lastUsed = s.now()
	s.appServerState.launchErr = ""
	s.appServerMu.Unlock()
	if stale != nil {
		s.closeIdleCodexClient(stale)
	}
	return client, nil
}

func codexAppServerRunKey(client *codexAppServerClient, threadID string) string {
	return fmt.Sprintf("%p:%s", client, threadID)
}

func (s *ExternalAgentService) closeIdleCodexClient(client *codexAppServerClient) {
	s.appServerMu.Lock()
	inUse := s.appServerState.client == client
	for _, run := range s.appServerState.runs {
		inUse = inUse || run.client == client
	}
	s.appServerMu.Unlock()
	if !inUse {
		client.Close()
	}
}

func (s *ExternalAgentService) resetCodexAppServerLocked() {
	s.appServerState.client = nil
	s.appServerState.executable = ""
}

// launchCodexAppServer spawns the process and installs the dispatch hooks.
// transportFactory is swappable so tests inject an in-process fake
// app-server; nil uses the real `codex app-server` launcher.
func (s *ExternalAgentService) launchCodexAppServer(executable string, agentEnv map[string]string) (*codexAppServerClient, error) {
	env := map[string]string{}
	for key, value := range agentEnv {
		env[key] = value
	}
	if s.discoveryPath != "" && env["LEMONSSH_TOOL_CLI_DISCOVERY_FILE"] == "" {
		env["LEMONSSH_TOOL_CLI_DISCOVERY_FILE"] = s.discoveryPath
		env["NETCATTY_TOOL_CLI_DISCOVERY_FILE"] = s.discoveryPath
	}
	factory := s.transportFactory
	if factory == nil {
		factory = launchCodexAppServerTransport
	}
	transport, err := factory(executable, []string{"app-server"}, sanitizeExternalEnv(env), "")
	if err != nil {
		return nil, err
	}
	client := newCodexAppServerClient(transport)
	client.notify = func(method string, params json.RawMessage) {
		s.handleCodexAppServerNotification(client, method, params)
	}
	client.request = func(rpcID int64, method string, params json.RawMessage) {
		s.handleCodexAppServerServerRequest(client, rpcID, method, params)
	}
	client.onExit = func(exitErr error) {
		s.handleCodexAppServerExit(client, exitErr)
	}
	client.Start()
	return client, nil
}

// handshakeCodexAppServer runs the initialize/initialized exchange.
func (s *ExternalAgentService) handshakeCodexAppServer(client *codexAppServerClient) error {
	result, err := client.Call("initialize", map[string]any{
		"clientInfo": map[string]any{"name": "LemonSSH", "version": version},
	}, codexAppServerHandshakeTimeout)
	if err != nil {
		return err
	}
	if len(result) == 0 {
		return errors.New("codex app-server returned an empty initialize result")
	}
	if err := client.Notify("initialized", nil); err != nil {
		return err
	}
	return nil
}

// handleCodexAppServerExit fails every run riding the dead process.
func (s *ExternalAgentService) handleCodexAppServerExit(client *codexAppServerClient, exitErr error) {
	s.appServerMu.Lock()
	if s.appServerState.client == client {
		s.resetCodexAppServerLocked()
	}
	runs := make([]*externalAgentRun, 0, len(s.appServerState.runs))
	for threadID, run := range s.appServerState.runs {
		if run.client == client {
			runs = append(runs, run)
			delete(s.appServerState.runs, threadID)
		}
	}
	s.appServerMu.Unlock()

	cause := exitErr
	if cause == nil {
		cause = errors.New("codex app-server process exited")
	}
	message := "codex app-server process exited: " + cause.Error()
	if tail := client.StderrText(); tail != "" {
		message = message + ": " + firstLines(tail, 4)
	}
	for _, run := range runs {
		s.finishCodexAppServerRun(run, message)
	}
}

// stopRun settles a superseded run when its requestID is being re-registered.
// Protocol runs have no context cancel (the shared process must survive);
// their stream is finalized instead, with a best-effort turn interrupt.
func (s *ExternalAgentService) stopRun(previous *externalAgentRun) {
	if previous == nil {
		return
	}
	if previous.appServer {
		previous.mu.Lock()
		threadID, turnID, client := previous.threadID, previous.turnID, previous.client
		previous.mu.Unlock()
		if client != nil && !client.IsExited() && threadID != "" && turnID != "" {
			_, _ = client.Call("turn/interrupt", map[string]any{"threadId": threadID, "turnId": turnID}, codexAppServerRequestTimeout)
		}
		s.finishCodexAppServerRun(previous, "")
		return
	}
	s.closeCodebuddy(previous)
	if previous.cancel != nil {
		previous.cancel()
	}
}

// registerCodexAppServerRun records an active protocol turn on both the
// request registry (for Cancel/Steer) and the thread registry (for event
// routing).
func (s *ExternalAgentService) registerCodexAppServerRun(run *externalAgentRun, threadID string) {
	s.mu.Lock()
	if previous := s.active[run.requestID]; previous != nil && previous != run {
		s.mu.Unlock()
		s.stopRun(previous)
		s.mu.Lock()
	}
	s.active[run.requestID] = run
	s.mu.Unlock()
	s.appServerMu.Lock()
	s.appServerState.runs[codexAppServerRunKey(run.client, threadID)] = run
	s.appServerMu.Unlock()
}

// codexAppServerRunByRequest finds the protocol run for a steer/cancel.
func (s *ExternalAgentService) codexAppServerRunByRequest(requestID, chatSessionID string) *externalAgentRun {
	s.mu.Lock()
	run := s.active[requestID]
	s.mu.Unlock()
	if run == nil || !run.appServer || run.chatSessionID != chatSessionID {
		return nil
	}
	return run
}

// codexAppServerUserInputs converts the staged prompt and attachments into
// protocol input items (text + localImage, per UserInput in the protocol
// types).
func codexAppServerUserInputs(prompt string, attachmentPaths []string) []map[string]any {
	input := []map[string]any{{"type": "text", "text": prompt}}
	for _, path := range attachmentPaths {
		input = append(input, map[string]any{"type": "localImage", "path": path})
	}
	return input
}

// streamViaCodexAppServer runs one turn over the protocol channel. handled
// reports whether the app-server path owned the request (success OR a
// reported failure); false means the caller must fall back to exec mode.
func (s *ExternalAgentService) streamViaCodexAppServer(request ExternalAgentStreamRequest, executable, prompt string, attachmentPaths []string, cleanupAttachments func()) (ExternalAgentResult, bool) {
	client, err := s.acquireCodexAppServer(executable, request.AgentEnv)
	if err != nil {
		// Graceful degradation: report the protocol failure on the stream and
		// fall back to the one-shot CLI exec path. Never a silent fake success.
		s.emitEvent(request.RequestID, map[string]any{
			"type":    "status",
			"message": fmt.Sprintf("Codex App Server unavailable (%s); falling back to CLI exec mode.", err.Error()),
		})
		return ExternalAgentResult{}, false
	}

	approvalPolicy, sandbox := codexAppServerApproval(request.PermissionMode)
	model := strings.TrimSpace(request.Model)
	cwd := strings.TrimSpace(request.CWD)
	threadParams := map[string]any{
		"approvalPolicy": approvalPolicy,
		"sandbox":        sandbox,
	}
	if cwd != "" {
		threadParams["cwd"] = cwd
	}
	if model != "" {
		threadParams["model"] = model
	}
	threadParams["config"] = externalCodexConfig(request)

	threadID := decodeExternalSessionID(request.ExistingSessionID, "codex")
	if threadID != "" {
		resumeParams := map[string]any{"threadId": threadID}
		for key, value := range threadParams {
			resumeParams[key] = value
		}
		if _, resumeErr := client.Call("thread/resume", resumeParams, codexAppServerRequestTimeout); resumeErr != nil {
			s.emitEvent(request.RequestID, map[string]any{
				"type":    "warning",
				"message": fmt.Sprintf("Could not resume Codex thread %s (%s); starting a new thread.", threadID, resumeErr.Error()),
			})
			threadID = ""
		}
	}
	if threadID == "" {
		started, startErr := client.Call("thread/start", threadParams, codexAppServerRequestTimeout)
		if startErr != nil {
			cleanupAttachments()
			message := startErr.Error()
			s.emitPayload("ai:sdk-agent:error", request.RequestID, map[string]any{"error": message})
			return ExternalAgentResult{OK: false, Error: message}, true
		}
		threadID = stringAt(jsonPayload(started), "thread", "id")
	}
	if threadID == "" {
		cleanupAttachments()
		message := "codex app-server thread/start returned no thread id"
		s.emitPayload("ai:sdk-agent:error", request.RequestID, map[string]any{"error": message})
		return ExternalAgentResult{OK: false, Error: message}, true
	}

	run := &externalAgentRun{
		chatSessionID:      request.ChatSessionID,
		requestID:          request.RequestID,
		appServer:          true,
		client:             client,
		threadID:           threadID,
		cleanupAttachments: cleanupAttachments,
	}
	s.registerCodexAppServerRun(run, threadID)
	s.emitEvent(request.RequestID, map[string]any{
		"type": "session-id", "sessionId": threadID, "sdkBackend": "codex", "binPath": executable, "runtime": "app-server",
	})

	turnParams := map[string]any{
		"threadId": threadID,
		"input":    codexAppServerUserInputs(prompt, attachmentPaths),
	}
	if model != "" {
		turnParams["model"] = model
	}
	turnResult, turnErr := client.Call("turn/start", turnParams, codexAppServerTurnStartTimeout)
	if turnErr != nil {
		message := turnErr.Error()
		s.finishCodexAppServerRun(run, message)
		return ExternalAgentResult{OK: false, Error: message}, true
	}
	if turnID := stringAt(jsonPayload(turnResult), "turn", "id"); turnID != "" {
		run.mu.Lock()
		run.turnID = turnID
		run.mu.Unlock()
	}
	s.emitEvent(request.RequestID, map[string]any{"type": "status", "message": "codex agent started (app-server)"})
	return ExternalAgentResult{OK: true}, true
}

// jsonPayload decodes a raw response into a generic map for path lookups.
func jsonPayload(raw json.RawMessage) map[string]any {
	payload := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &payload)
	}
	return payload
}

// firstLines keeps at most n lines of a diagnostic tail.
func firstLines(text string, n int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, " | ")
}

// handleCodexAppServerNotification routes one server notification to the run
// owning its thread and maps it onto the shared ai:sdk-agent:event stream.
func (s *ExternalAgentService) handleCodexAppServerNotification(client *codexAppServerClient, method string, raw json.RawMessage) {
	params := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &params)
	}
	threadID := stringAt(params, "threadId")
	s.appServerMu.Lock()
	run := s.appServerState.runs[codexAppServerRunKey(client, threadID)]
	s.appServerMu.Unlock()

	switch method {
	case "turn/started":
		if run == nil {
			return
		}
		if turnID := stringAt(params, "turn", "id"); turnID != "" {
			run.mu.Lock()
			run.turnID = turnID
			run.mu.Unlock()
		}
	case "item/agentMessage/delta":
		if run == nil {
			return
		}
		run.mu.Lock()
		run.appServerAgentDeltaSeen = true
		run.mu.Unlock()
		s.emitText(run.requestID, run, stringAt(params, "delta"))
	case "item/reasoning/textDelta", "item/reasoning/summaryTextDelta":
		if run == nil {
			return
		}
		if delta := stringAt(params, "delta"); delta != "" {
			s.emitEvent(run.requestID, map[string]any{"type": "reasoning-delta", "delta": delta})
		}
	case "item/started":
		if run == nil {
			return
		}
		item, _ := params["item"].(map[string]any)
		if name, id, ok := codexAppServerToolIdentity(item); ok {
			s.emitEvent(run.requestID, map[string]any{"type": "tool-call", "toolName": name, "toolCallId": id, "input": item})
		}
	case "item/completed":
		if run == nil {
			return
		}
		item, _ := params["item"].(map[string]any)
		switch codexAppServerItemType(item) {
		case codexItemCommandExecution, codexItemMcpToolCall, codexItemDynamicToolCall, codexItemFileChange, codexItemWebSearch:
			_, id, _ := codexAppServerToolIdentity(item)
			s.emitEvent(run.requestID, map[string]any{"type": "tool-result", "toolCallId": id, "output": codexAppServerToolOutput(item)})
		case codexItemAgentMessage:
			run.mu.Lock()
			seenDelta := run.appServerAgentDeltaSeen
			run.mu.Unlock()
			if !seenDelta {
				s.emitText(run.requestID, run, stringAt(item, "text"))
			}
		}
	case "thread/tokenUsage/updated":
		if run == nil {
			return
		}
		usage, _ := params["tokenUsage"].(map[string]any)
		total, _ := usage["total"].(map[string]any)
		inputTokens := numberAt(total, "inputTokens")
		outputTokens := numberAt(total, "outputTokens")
		totalTokens := numberAt(total, "totalTokens")
		if inputTokens > 0 || outputTokens > 0 || totalTokens > 0 {
			s.emitEvent(run.requestID, map[string]any{
				"type": "usage", "inputTokens": inputTokens, "outputTokens": outputTokens, "totalTokens": totalTokens,
			})
		}
	case "warning":
		message := stringAt(params, "message")
		if run != nil && message != "" {
			s.emitEvent(run.requestID, map[string]any{"type": "warning", "message": message})
		}
	case "configWarning":
		if run != nil {
			summary := stringAt(params, "summary")
			details := stringAt(params, "details")
			message := strings.TrimSpace(summary + " " + details)
			if message != "" {
				s.emitEvent(run.requestID, map[string]any{"type": "warning", "message": message})
			}
		}
	case "error":
		if run == nil {
			return
		}
		message := stringAt(params, "error", "message")
		if message == "" {
			return
		}
		run.mu.Lock()
		if run.stderr.Len() < 64*1024 {
			run.stderr.WriteString(message + "\n")
		}
		run.mu.Unlock()
		willRetry, _ := params["willRetry"].(bool)
		if !willRetry {
			s.emitEvent(run.requestID, map[string]any{"type": "warning", "message": message})
		}
	case "thread/status/changed":
		if run == nil {
			return
		}
		if strings.EqualFold(stringAt(params, "status", "type"), "systemError") {
			s.emitEvent(run.requestID, map[string]any{"type": "warning", "message": "codex thread entered systemError state"})
		}
	case "turn/completed":
		if run == nil {
			return
		}
		status := strings.ToLower(stringAt(params, "turn", "status"))
		switch status {
		case "completed", "interrupted":
			s.finishCodexAppServerRun(run, "")
		case "failed":
			message := stringAt(params, "turn", "error", "message")
			if message == "" {
				message = "codex turn failed"
			}
			s.finishCodexAppServerRun(run, message)
		}
	}
}

// ThreadItem type discriminators, lowercased once (strings.ToLower of the
// protocol names) so the notification mapping cannot drift per call site.
const (
	codexItemCommandExecution = "commandexecution"
	codexItemMcpToolCall      = "mcptoollcall"
	codexItemDynamicToolCall  = "dynamictoolcall"
	codexItemFileChange       = "filechange"
	codexItemWebSearch        = "websearch"
	codexItemAgentMessage     = "agentmessage"
)

func codexAppServerItemType(item map[string]any) string {
	return strings.ToLower(stringAt(item, "type"))
}

// codexAppServerToolIdentity extracts the tool-call event fields for an item.
func codexAppServerToolIdentity(item map[string]any) (name, id string, ok bool) {
	if item == nil {
		return "", "", false
	}
	id = stringAt(item, "id")
	switch codexAppServerItemType(item) {
	case codexItemCommandExecution:
		name = "commandExecution"
		if command := strings.TrimSpace(stringAt(item, "command")); command != "" {
			if lines := strings.Split(command, "\n"); len(lines) > 0 && len(lines[0]) <= 120 {
				name = lines[0]
			}
		}
	case codexItemMcpToolCall:
		name = strings.TrimSpace(stringAt(item, "server") + ":" + stringAt(item, "tool"))
	case codexItemDynamicToolCall:
		name = stringAt(item, "tool")
	case codexItemFileChange:
		name = "fileChange"
	case codexItemWebSearch:
		name = "webSearch"
	default:
		return "", "", false
	}
	return name, id, true
}

// codexAppServerToolOutput picks the best output payload of a completed item.
func codexAppServerToolOutput(item map[string]any) any {
	if item == nil {
		return nil
	}
	switch codexAppServerItemType(item) {
	case codexItemCommandExecution:
		if output := item["aggregatedOutput"]; output != nil {
			return output
		}
	case codexItemMcpToolCall:
		if result := item["result"]; result != nil {
			return result
		}
		if failure := item["error"]; failure != nil {
			return failure
		}
	case codexItemDynamicToolCall:
		if content := item["contentItems"]; content != nil {
			return content
		}
	case codexItemFileChange:
		if changes := item["changes"]; changes != nil {
			return changes
		}
	}
	return item
}

// finishCodexAppServerRun tears down one protocol turn: cancel any pending
// interactions on the thread (answering the server so it never wedges), clean
// staged attachments and emit the terminal stream event. errText=="" emits
// done; otherwise a partial-output warning or a hard error, matching exec
// mode semantics. Idempotent: the server's turn/completed{interrupted} and a
// user Cancel can race for the same run; the first finish wins and the
// terminal stream event is emitted exactly once.
func (s *ExternalAgentService) finishCodexAppServerRun(run *externalAgentRun, errText string) {
	run.mu.Lock()
	if run.appServerFinished {
		run.mu.Unlock()
		return
	}
	run.appServerFinished = true
	run.mu.Unlock()

	s.appServerMu.Lock()
	key := codexAppServerRunKey(run.client, run.threadID)
	if existing := s.appServerState.runs[key]; existing == run {
		delete(s.appServerState.runs, key)
	}
	cancelled := make([]*codexAppServerInteraction, 0)
	for interactionID, pending := range s.appServerState.pending {
		if pending.threadID == run.threadID && pending.client == run.client {
			cancelled = append(cancelled, pending)
			delete(s.appServerState.pending, interactionID)
		}
	}
	s.appServerMu.Unlock()

	for _, pending := range cancelled {
		if pending.timer != nil {
			pending.timer.Stop()
		}
		// Deny the server-side request so the turn/teardown is not blocked.
		_ = pending.client.RespondError(pending.rpcID, -32001, "LemonSSH turn ended before the interaction was answered")
	}
	if len(cancelled) > 0 {
		ids := make([]string, 0, len(cancelled))
		for _, pending := range cancelled {
			ids = append(ids, pending.interactionID)
		}
		s.emitPayload(codexAppServerInteractionClearedEvent, "", map[string]any{
			"interactionIds": ids, "chatSessionId": run.chatSessionID,
		})
	}

	s.closeIdleCodexClient(run.client)
	if run.cleanupAttachments != nil {
		run.cleanupAttachments()
		run.cleanupAttachments = nil
	}
	s.mu.Lock()
	if s.active[run.requestID] == run {
		delete(s.active, run.requestID)
	}
	run.mu.Lock()
	emittedText := run.emittedText
	stderrText := strings.TrimSpace(run.stderr.String())
	run.mu.Unlock()
	s.mu.Unlock()

	if errText != "" {
		if emittedText {
			s.emitEvent(run.requestID, map[string]any{"type": "warning", "message": "Agent ended after returning partial output: " + errText})
			if stderrText != "" {
				s.emitEvent(run.requestID, map[string]any{"type": "warning", "message": stderrText})
			}
			s.emitPayload("ai:sdk-agent:done", run.requestID, nil)
			return
		}
		s.emitPayload("ai:sdk-agent:error", run.requestID, map[string]any{"error": errText})
		return
	}
	if stderrText != "" {
		s.emitEvent(run.requestID, map[string]any{"type": "warning", "message": stderrText})
	}
	s.emitPayload("ai:sdk-agent:done", run.requestID, nil)
}

// steerViaCodexAppServer injects a mid-turn instruction through turn/steer.
func (s *ExternalAgentService) steerViaCodexAppServer(requestID, chatSessionID, prompt string, images []ExternalAgentImage) (ExternalAgentSteerResult, bool) {
	run := s.codexAppServerRunByRequest(requestID, chatSessionID)
	if run == nil {
		return ExternalAgentSteerResult{}, false
	}
	run.mu.Lock()
	threadID, turnID, client := run.threadID, run.turnID, run.client
	run.mu.Unlock()
	if threadID == "" || turnID == "" || client == nil || client.IsExited() {
		return ExternalAgentSteerResult{Status: "unsupported", Message: "Codex App Server turn is not steering-capable right now."}, true
	}
	paths, cleanup := s.stageImages(ExternalAgentStreamRequest{RequestID: requestID, Images: images})
	_, err := client.Call("turn/steer", map[string]any{
		"threadId":       threadID,
		"expectedTurnId": turnID,
		"input":          codexAppServerUserInputs(prompt, paths),
	}, codexAppServerRequestTimeout)
	cleanup()
	if err != nil {
		return ExternalAgentSteerResult{Status: "unsupported", Message: "Codex App Server rejected steer: " + err.Error()}, true
	}
	return ExternalAgentSteerResult{Status: "accepted"}, true
}

// listModelsViaCodexAppServer queries the live model catalog. A nil models
// slice with a warning means degraded (presets remain available).
func (s *ExternalAgentService) listModelsViaCodexAppServer(executable string, agentEnv map[string]string) ([]map[string]any, string, string, error) {
	client, err := s.acquireCodexAppServer(executable, agentEnv)
	if err != nil {
		return nil, "", "", err
	}
	result, err := client.Call("model/list", map[string]any{}, codexAppServerRequestTimeout)
	if err != nil {
		return nil, "", "", err
	}
	payload := jsonPayload(result)
	entries, _ := payload["data"].([]any)
	models := make([]map[string]any, 0, len(entries))
	defaultModel := ""
	for _, entry := range entries {
		model, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		id := stringAt(model, "id")
		if id == "" {
			id = stringAt(model, "model")
		}
		item := map[string]any{
			"id":          id,
			"name":        firstNonEmptyString(stringAt(model, "displayName"), id),
			"description": stringAt(model, "description"),
		}
		if defaultItem, _ := model["defaultReasoningEffort"].(string); defaultItem != "" {
			item["defaultThinkingLevel"] = defaultItem
		}
		efforts := []string{}
		if options, ok := model["supportedReasoningEfforts"].([]any); ok {
			for _, option := range options {
				if effort, ok := option.(map[string]any); ok {
					if effortID := stringAt(effort, "effort"); effortID != "" {
						efforts = append(efforts, effortID)
					}
				}
			}
		}
		if len(efforts) > 0 {
			item["thinkingLevels"] = efforts
		}
		if isDefault, _ := model["isDefault"].(bool); isDefault {
			defaultModel = id
		}
		models = append(models, item)
	}
	if len(models) == 0 {
		return nil, "", "", errors.New("codex app-server returned an empty model catalog")
	}
	return models, defaultModel, "", nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// handleCodexAppServerServerRequest turns approval / user-input requests into
// renderer interactions; unsupported server calls fail typed so the server
// never wedges waiting for a response.
func (s *ExternalAgentService) handleCodexAppServerServerRequest(client *codexAppServerClient, rpcID int64, method string, raw json.RawMessage) {
	params := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &params)
	}
	var kind string
	switch method {
	case "item/commandExecution/requestApproval", "execCommandApproval":
		kind = "command"
	case "item/fileChange/requestApproval", "applyPatchApproval":
		kind = "file-change"
	case "item/permissions/requestApproval":
		kind = "permissions"
	case "item/tool/requestUserInput":
		kind = "user-input"
	default:
		_ = client.RespondError(rpcID, -32601, "LemonSSH does not support app-server method "+method)
		return
	}

	threadID := stringAt(params, "threadId")
	if threadID == "" {
		threadID = stringAt(params, "conversationId")
	}
	s.appServerMu.Lock()
	run := s.appServerState.runs[codexAppServerRunKey(client, threadID)]
	s.appServerState.seq++
	interactionID := fmt.Sprintf("codex-ia-%d", s.appServerState.seq)
	interaction := &codexAppServerInteraction{
		interactionID: interactionID,
		rpcID:         rpcID,
		method:        method,
		kind:          kind,
		threadID:      threadID,
		chatSessionID: "",
		params:        params,
		client:        client,
	}
	if run != nil {
		interaction.chatSessionID = run.chatSessionID
	}
	interaction.timer = time.AfterFunc(codexAppServerInteractionTimeout, func() {
		s.timeoutCodexAppServerInteraction(interactionID)
	})
	s.appServerState.pending[interactionID] = interaction
	s.appServerMu.Unlock()

	s.emitPayload(codexAppServerInteractionEvent, "", codexAppServerInteractionPayload(interaction))
}

// codexAppServerInteractionPayload renders the wire shape consumed by
// infrastructure/ai/shared/codexAppServerInteractions.ts (CodexAppServerInteraction).
func codexAppServerInteractionPayload(interaction *codexAppServerInteraction) map[string]any {
	payload := map[string]any{
		"interactionId": interaction.interactionID,
		"source":        "codex-app-server",
		"kind":          interaction.kind,
		"requestId":     fmt.Sprintf("%d", interaction.rpcID),
		"chatSessionId": interaction.chatSessionID,
		"args":          interaction.params,
	}
	switch interaction.kind {
	case "command":
		payload["toolName"] = firstNonEmptyString(strings.TrimSpace(stringAt(interaction.params, "command")), "commandExecution")
		payload["itemId"] = firstNonEmptyString(stringAt(interaction.params, "itemId"), stringAt(interaction.params, "callId"))
		payload["availableDecisions"] = []string{"accept", "acceptForSession", "decline", "cancel"}
	case "file-change":
		payload["toolName"] = "fileChange"
		payload["itemId"] = firstNonEmptyString(stringAt(interaction.params, "itemId"), stringAt(interaction.params, "callId"))
		payload["availableDecisions"] = []string{"accept", "acceptForSession", "decline", "cancel"}
	case "permissions":
		payload["toolName"] = "permissions"
		payload["itemId"] = stringAt(interaction.params, "itemId")
		payload["availableDecisions"] = []string{"accept", "acceptForSession", "decline", "cancel"}
	case "user-input":
		payload["toolName"] = "userInput"
		payload["itemId"] = stringAt(interaction.params, "itemId")
		if questions, ok := interaction.params["questions"]; ok {
			payload["questions"] = questions
		} else {
			payload["questions"] = []any{}
		}
		payload["autoResolutionMs"] = nil
	}
	if reason := stringAt(interaction.params, "reason"); reason != "" {
		payload["reason"] = reason
	}
	return payload
}

// timeoutCodexAppServerInteraction answers an unanswered interaction so the
// codex turn cannot stall forever on a hidden/closed renderer.
func (s *ExternalAgentService) timeoutCodexAppServerInteraction(interactionID string) {
	s.appServerMu.Lock()
	pending := s.appServerState.pending[interactionID]
	delete(s.appServerState.pending, interactionID)
	s.appServerMu.Unlock()
	if pending == nil {
		return
	}
	_ = pending.client.RespondError(pending.rpcID, -32002, "LemonSSH interaction timed out without a user decision")
	s.emitPayload(codexAppServerInteractionClearedEvent, "", map[string]any{
		"interactionIds": []string{interactionID}, "chatSessionId": pending.chatSessionID,
	})
}

// RespondCodexAppServerInteraction answers a pending approval / user-input
// request. Payload: {interactionId, decision? , answers?}. Decisions follow
// the frontend CodexApprovalDecision values ('once' | 'session' | 'reject' |
// 'cancel') for approvals and pass `answers` through verbatim for user input.
func (s *ExternalAgentService) RespondCodexAppServerInteraction(payload map[string]any) map[string]any {
	interactionID := stringAt(payload, "interactionId")
	if interactionID == "" {
		return map[string]any{"ok": false, "error": "interactionId is required"}
	}
	s.appServerMu.Lock()
	pending := s.appServerState.pending[interactionID]
	delete(s.appServerState.pending, interactionID)
	s.appServerMu.Unlock()
	if pending == nil {
		return map[string]any{"ok": false, "error": "unknown or already resolved interaction"}
	}
	if pending.timer != nil {
		pending.timer.Stop()
	}
	if pending.client.IsExited() {
		return map[string]any{"ok": false, "error": "codex app-server is not running"}
	}

	decision, _ := payload["decision"].(string)
	answers, hasAnswers := payload["answers"].(map[string]any)
	var respondErr error
	switch {
	case pending.kind == "user-input":
		if !hasAnswers {
			respondErr = pending.client.RespondError(pending.rpcID, -32602, "missing answers for requestUserInput")
		} else {
			respondErr = pending.client.Respond(pending.rpcID, map[string]any{"answers": answers})
		}
	case pending.kind == "permissions":
		respondErr = s.respondCodexPermissions(pending, decision)
	default:
		respondErr = s.respondCodexApproval(pending, decision)
	}
	if respondErr != nil {
		return map[string]any{"ok": false, "error": respondErr.Error()}
	}
	s.emitPayload(codexAppServerInteractionClearedEvent, "", map[string]any{
		"interactionIds": []string{interactionID}, "chatSessionId": pending.chatSessionID,
	})
	return map[string]any{"ok": true}
}

// respondCodexApproval maps decisions onto {decision} results (v2 approval
// requests) or the legacy ReviewDecision vocabulary.
func (s *ExternalAgentService) respondCodexApproval(pending *codexAppServerInteraction, decision string) error {
	legacy := pending.method == "execCommandApproval" || pending.method == "applyPatchApproval"
	var result any
	switch decision {
	case "once":
		if legacy {
			result = map[string]any{"decision": "approved"}
		} else {
			result = map[string]any{"decision": "accept"}
		}
	case "session":
		if legacy {
			result = map[string]any{"decision": "approved_for_session"}
		} else {
			result = map[string]any{"decision": "acceptForSession"}
		}
	case "reject":
		if legacy {
			result = map[string]any{"decision": "denied"}
		} else {
			result = map[string]any{"decision": "decline"}
		}
	case "cancel":
		if legacy {
			result = map[string]any{"decision": "abort"}
		} else {
			result = map[string]any{"decision": "cancel"}
		}
	default:
		return fmt.Errorf("unsupported codex approval decision %q", decision)
	}
	return pending.client.Respond(pending.rpcID, result)
}

// respondCodexPermissions grants the requested permission profile for the
// chosen scope, or denies by JSON-RPC error.
func (s *ExternalAgentService) respondCodexPermissions(pending *codexAppServerInteraction, decision string) error {
	switch decision {
	case "once", "session":
		scope := "turn"
		if decision == "session" {
			scope = "session"
		}
		permissions, _ := pending.params["permissions"].(map[string]any)
		if permissions == nil {
			permissions = map[string]any{}
		}
		return pending.client.Respond(pending.rpcID, map[string]any{"permissions": permissions, "scope": scope})
	default:
		return pending.client.RespondError(pending.rpcID, -32001, "permission request denied by user ("+decision+")")
	}
}

// CancelCodexAppServerInteractionTimeout hands interaction ownership to the
// renderer (approvalGate resolves it locally and calls Respond next).
func (s *ExternalAgentService) CancelCodexAppServerInteractionTimeout(interactionID string) map[string]any {
	s.appServerMu.Lock()
	pending := s.appServerState.pending[interactionID]
	if pending != nil && pending.timer != nil {
		pending.timer.Stop()
		pending.timer = nil
	}
	s.appServerMu.Unlock()
	return map[string]any{"ok": true, "cancelled": pending != nil}
}

// codexAppServerAvailable reports whether the shared protocol client is up.
func (s *ExternalAgentService) codexAppServerLaunchError() string {
	s.appServerMu.Lock()
	defer s.appServerMu.Unlock()
	return s.appServerState.launchErr
}

// probeCodexAppServer runs a real protocol handshake for status reporting.
func (s *ExternalAgentService) probeCodexAppServer(executable string, agentEnv map[string]string) error {
	_, err := s.acquireCodexAppServer(executable, agentEnv)
	return err
}
