package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

type ExternalAgentHistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ExternalAgentImage struct {
	Base64Data string `json:"base64Data"`
	MediaType  string `json:"mediaType"`
	Filename   string `json:"filename,omitempty"`
	FilePath   string `json:"filePath,omitempty"`
}

type ExternalAgentTarget struct {
	SessionID  string `json:"sessionId"`
	Hostname   string `json:"hostname"`
	Label      string `json:"label"`
	OS         string `json:"os,omitempty"`
	Username   string `json:"username,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
	ShellType  string `json:"shellType,omitempty"`
	DeviceType string `json:"deviceType,omitempty"`
	Connected  bool   `json:"connected"`
	Source     string `json:"source"`
}

type ExternalAgentStreamRequest struct {
	RequestID           string                        `json:"requestId"`
	ChatSessionID       string                        `json:"chatSessionId"`
	SDKBackend          string                        `json:"sdkBackend"`
	Prompt              string                        `json:"prompt"`
	CWD                 string                        `json:"cwd,omitempty"`
	ProviderID          string                        `json:"providerId,omitempty"`
	Model               string                        `json:"model,omitempty"`
	ExistingSessionID   string                        `json:"existingSessionId,omitempty"`
	HistoryMessages     []ExternalAgentHistoryMessage `json:"historyMessages,omitempty"`
	Images              []ExternalAgentImage          `json:"images,omitempty"`
	ToolIntegrationMode string                        `json:"toolIntegrationMode,omitempty"`
	DefaultTarget       *ExternalAgentTarget          `json:"defaultTargetSession,omitempty"`
	UserSkillsContext   string                        `json:"userSkillsContext,omitempty"`
	AgentEnv            map[string]string             `json:"agentEnv,omitempty"`
	AgentCommand        string                        `json:"agentCommand,omitempty"`
	CodexRuntime        string                        `json:"codexRuntime,omitempty"`
	PermissionMode      string                        `json:"permissionMode,omitempty"`
	CodebuddyOptions    map[string]any                `json:"codebuddyOptions,omitempty"`
}

type ExternalAgentResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type ExternalAgentSteerResult struct {
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
	TurnKind string `json:"turnKind,omitempty"`
}

type ExternalAgentModelsResult struct {
	OK             bool             `json:"ok"`
	Models         []map[string]any `json:"models,omitempty"`
	CurrentModelID string           `json:"currentModelId,omitempty"`
	Warning        string           `json:"warning,omitempty"`
	Error          string           `json:"error,omitempty"`
}

type externalAgentRun struct {
	cancel        context.CancelFunc
	command       *exec.Cmd
	chatSessionID string
	emittedText   bool
	stderr        strings.Builder
	mu            sync.Mutex
}

type ExternalAgentService struct {
	mu            sync.Mutex
	active        map[string]*externalAgentRun
	emit          func(string, any)
	discoveryPath string
	tempRoot      string
}

func newExternalAgentService(discoveryPath, tempRoot string) *ExternalAgentService {
	return &ExternalAgentService{active: map[string]*externalAgentRun{}, discoveryPath: discoveryPath, tempRoot: tempRoot}
}

func (s *ExternalAgentService) setEventEmitter(emit func(string, any)) { s.emit = emit }

func (s *ExternalAgentService) emitPayload(name, requestID string, payload map[string]any) {
	if s.emit == nil {
		return
	}
	event := map[string]any{"requestId": requestID}
	for key, value := range payload {
		event[key] = value
	}
	s.emit(name, event)
}

func (s *ExternalAgentService) emitEvent(requestID string, event map[string]any) {
	s.emitPayload("ai:sdk-agent:event", requestID, map[string]any{"event": event})
}

func normalizeExternalBackend(value string) (string, bool) {
	backend := strings.ToLower(strings.TrimSpace(value))
	switch backend {
	case "codex", "claude", "copilot", "cursor", "codebuddy", "opencode", "grok":
		return backend, true
	default:
		return "", false
	}
}

func (s *ExternalAgentService) resolveExecutable(backend, configured string) string {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		if info, err := os.Stat(configured); err == nil && info.IsDir() {
			return findExecutableInDirectory(configured, managedAgentExecutables[backend])
		}
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return configured
		}
		if found, err := exec.LookPath(configured); err == nil {
			return found
		}
	}
	return findAgentExecutable(managedAgentExecutables[backend], "")
}

func appendModel(args []string, backend, model string) []string {
	if strings.TrimSpace(model) == "" {
		return args
	}
	switch backend {
	case "codex", "claude", "cursor", "codebuddy", "opencode", "grok":
		return append(args, "--model", model)
	default:
		return args
	}
}

func decodeExternalSessionID(value, backend string) string {
	const prefix = "netcatty-sdk-session:"
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, prefix) {
		return trimmed
	}
	decoded, err := url.PathUnescape(strings.TrimPrefix(trimmed, prefix))
	if err != nil {
		return ""
	}
	var payload struct {
		Version int    `json:"v"`
		ID      string `json:"id"`
		Backend string `json:"backend"`
	}
	if json.Unmarshal([]byte(decoded), &payload) != nil || payload.Version != 1 || payload.Backend != backend {
		return ""
	}
	return payload.ID
}

func externalAgentArgs(backend, prompt, model, permissionMode, existingSessionID string) []string {
	existingSessionID = decodeExternalSessionID(existingSessionID, backend)
	var args []string
	switch backend {
	case "codex":
		// Parent exec flags must precede the resume subcommand. Placing
		// --sandbox after resume is rejected by current Codex CLIs.
		args = []string{"exec", "--json", "--skip-git-repo-check"}
		switch permissionMode {
		case "auto":
			args = append(args, "--dangerously-bypass-approvals-and-sandbox")
		default:
			// The one-shot CLI cannot surface native approval requests. Keep
			// Observer and Confirm fail-closed; LemonSSH tools still provide
			// their own Confirm approval bridge.
			args = append(args, "--sandbox", "read-only")
		}
		args = appendModel(args, backend, model)
		if existingSessionID != "" {
			args = append(args, "resume", existingSessionID)
		}
		args = append(args, prompt)
	case "claude":
		args = []string{"--print", "--verbose", "--output-format", "stream-json", "--include-partial-messages"}
		if permissionMode == "observer" {
			args = append(args, "--permission-mode", "plan")
		}
		if permissionMode == "auto" {
			args = append(args, "--dangerously-skip-permissions")
		}
		args = appendModel(args, backend, model)
		if existingSessionID != "" {
			args = append(args, "--resume", existingSessionID)
		}
		args = append(args, prompt)
	case "cursor":
		args = []string{"-p", "--output-format", "stream-json"}
		if permissionMode == "auto" {
			args = append(args, "--mode", "agent", "--force")
		} else {
			// Cursor's headless agent mode can write without a LemonSSH
			// approval callback, so Confirm remains in ask mode as well.
			args = append(args, "--mode", "ask")
		}
		args = appendModel(args, backend, model)
		if existingSessionID != "" {
			args = append(args, "--resume", existingSessionID)
		}
		args = append(args, prompt)
	case "codebuddy":
		args = []string{"--print", "--verbose", "--output-format", "stream-json"}
		args = appendModel(args, backend, model)
		if existingSessionID != "" {
			args = append(args, "--resume", existingSessionID)
		}
		args = append(args, prompt)
	case "opencode":
		args = []string{"run", "--format", "json"}
		args = appendModel(args, backend, model)
		if existingSessionID != "" {
			args = append(args, "--session", existingSessionID)
		}
		args = append(args, prompt)
	case "grok":
		args = []string{"agent", "--streaming-json"}
		args = appendModel(args, backend, model)
		if existingSessionID != "" {
			args = append(args, "--resume", existingSessionID)
		}
		args = append(args, prompt)
	case "copilot":
		args = []string{"-p", prompt}
	}
	return args
}

func quoteWindowsCmdArg(value string) string {
	if value == "" {
		return `""`
	}
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func streamingCommand(ctx context.Context, executable string, args []string) *exec.Cmd {
	lower := strings.ToLower(executable)
	if runtime.GOOS == "windows" && (strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat")) {
		pieces := []string{"call", quoteWindowsCmdArg(executable)}
		for _, arg := range args {
			pieces = append(pieces, quoteWindowsCmdArg(arg))
		}
		return exec.CommandContext(ctx, "cmd.exe", "/D", "/S", "/C", strings.Join(pieces, " "))
	}
	return exec.CommandContext(ctx, executable, args...)
}

func sanitizeExternalEnv(input map[string]string) []string {
	envMap := map[string]string{}
	for _, item := range os.Environ() {
		if key, _, ok := strings.Cut(item, "="); ok {
			envMap[strings.ToUpper(key)] = item
		}
	}
	blocked := map[string]bool{"NODE_OPTIONS": true, "ELECTRON_RUN_AS_NODE": true, "LD_PRELOAD": true, "DYLD_INSERT_LIBRARIES": true}
	for key, value := range input {
		upper := strings.ToUpper(strings.TrimSpace(key))
		if upper == "" || blocked[upper] || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
			continue
		}
		envMap[upper] = key + "=" + value
	}
	result := make([]string, 0, len(envMap))
	for _, entry := range envMap {
		result = append(result, entry)
	}
	return result
}

const (
	maxExternalAgentImages     = 8
	maxExternalAgentImageBytes = 20 << 20
)

func externalAttachmentDirectory(tempRoot, requestID string) string {
	sum := sha256.Sum256([]byte(requestID))
	return filepath.Join(tempRoot, "agent-attachments", fmt.Sprintf("%x", sum[:]))
}

func (s *ExternalAgentService) stageImages(request ExternalAgentStreamRequest) ([]string, func()) {
	if len(request.Images) == 0 {
		return nil, func() {}
	}
	root := externalAttachmentDirectory(s.tempRoot, request.RequestID)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, func() {}
	}
	paths := []string{}
	for index, image := range request.Images {
		if index >= maxExternalAgentImages {
			break
		}
		if image.FilePath != "" {
			if info, err := os.Stat(image.FilePath); err == nil && info.Mode().IsRegular() {
				paths = append(paths, image.FilePath)
			}
			continue
		}
		if base64.StdEncoding.DecodedLen(len(image.Base64Data)) > maxExternalAgentImageBytes {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(image.Base64Data)
		if err != nil || len(data) > maxExternalAgentImageBytes {
			continue
		}
		name := filepath.Base(image.Filename)
		if name == "." || name == "" {
			name = "attachment.bin"
		}
		name = fmt.Sprintf("%03d-%s", index+1, name)
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, data, 0o600); err == nil {
			paths = append(paths, path)
		}
	}
	return paths, func() { _ = os.RemoveAll(root) }
}

func resolveExternalAgentToolPathFrom(executable, cwd string) string {
	name := "LemonSSH-tool"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	candidates := []string{}
	if executable != "" {
		directory := filepath.Dir(executable)
		candidates = append(candidates, filepath.Join(directory, name), filepath.Join(directory, "bin", name))
	}
	if cwd != "" {
		candidates = append(candidates, filepath.Join(cwd, "bin", name), filepath.Join(cwd, "dist", "wails", name))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate
		}
	}
	return name
}

func resolveExternalAgentToolPath() string {
	executable, _ := os.Executable()
	cwd, _ := os.Getwd()
	return resolveExternalAgentToolPathFrom(executable, cwd)
}

func (s *ExternalAgentService) buildPrompt(request ExternalAgentStreamRequest, attachmentPaths []string) string {
	sections := []string{}
	if request.ExistingSessionID == "" && len(request.HistoryMessages) > 0 {
		lines := []string{"[Conversation context replay. Continue the same conversation and answer the latest request.]"}
		for _, message := range request.HistoryMessages {
			role := strings.ToUpper(strings.TrimSpace(message.Role))
			if role != "USER" && role != "ASSISTANT" {
				continue
			}
			if text := strings.TrimSpace(message.Content); text != "" {
				lines = append(lines, role+": "+text)
			}
		}
		if len(lines) > 1 {
			sections = append(sections, strings.Join(lines, "\n"))
		}
	}
	if request.DefaultTarget != nil {
		target, _ := json.Marshal(request.DefaultTarget)
		sections = append(sections, "[LemonSSH target terminal context]\n"+string(target))
	}
	if request.UserSkillsContext != "" {
		sections = append(sections, request.UserSkillsContext)
	}
	if len(attachmentPaths) > 0 {
		sections = append(sections, "[Attached local files]\n- "+strings.Join(attachmentPaths, "\n- "))
	}
	if request.ToolIntegrationMode == "skills" || request.ToolIntegrationMode == "mcp" {
		toolName := resolveExternalAgentToolPath()
		sections = append(sections, fmt.Sprintf("[LemonSSH native tools]\nUse %q when terminal, SFTP, vault, attachment, or port-forward access is needed. The host enforces Observer/Confirm/Auto permissions. The discovery environment is already configured.", toolName))
	}
	sections = append(sections, request.Prompt)
	return strings.Join(sections, "\n\n")
}

func (s *ExternalAgentService) Stream(request ExternalAgentStreamRequest) ExternalAgentResult {
	if strings.TrimSpace(request.RequestID) == "" || strings.TrimSpace(request.ChatSessionID) == "" {
		return ExternalAgentResult{Error: "requestId and chatSessionId are required"}
	}
	backend, ok := normalizeExternalBackend(request.SDKBackend)
	if !ok {
		return ExternalAgentResult{Error: "unsupported external agent backend"}
	}
	executable := s.resolveExecutable(backend, request.AgentCommand)
	if executable == "" {
		return ExternalAgentResult{Error: fmt.Sprintf("%s executable was not found; choose its installation directory or executable in Agent settings", backend)}
	}
	attachmentPaths, cleanupAttachments := s.stageImages(request)
	prompt := s.buildPrompt(request, attachmentPaths)
	args := externalAgentArgs(backend, prompt, request.Model, request.PermissionMode, request.ExistingSessionID)
	ctx, cancel := context.WithCancel(context.Background())
	command := streamingCommand(ctx, executable, args)
	cwd := strings.TrimSpace(request.CWD)
	if cwd != "" {
		if info, err := os.Stat(cwd); err == nil && info.IsDir() {
			command.Dir = cwd
		}
	}
	env := map[string]string{}
	for key, value := range request.AgentEnv {
		env[key] = value
	}
	if s.discoveryPath != "" {
		env["LEMONSSH_TOOL_CLI_DISCOVERY_FILE"] = s.discoveryPath
		env["NETCATTY_TOOL_CLI_DISCOVERY_FILE"] = s.discoveryPath
	}
	command.Env = sanitizeExternalEnv(env)
	stdout, err := command.StdoutPipe()
	if err != nil {
		cancel()
		cleanupAttachments()
		return ExternalAgentResult{Error: err.Error()}
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		cancel()
		cleanupAttachments()
		return ExternalAgentResult{Error: err.Error()}
	}
	run := &externalAgentRun{cancel: cancel, command: command, chatSessionID: request.ChatSessionID}
	s.mu.Lock()
	if previous := s.active[request.RequestID]; previous != nil {
		previous.cancel()
	}
	s.active[request.RequestID] = run
	s.mu.Unlock()
	if err := command.Start(); err != nil {
		s.mu.Lock()
		if s.active[request.RequestID] == run {
			delete(s.active, request.RequestID)
		}
		s.mu.Unlock()
		cancel()
		cleanupAttachments()
		return ExternalAgentResult{Error: err.Error()}
	}
	s.emitEvent(request.RequestID, map[string]any{"type": "status", "message": fmt.Sprintf("%s agent started", backend)})
	go s.consumeAgentRun(request.RequestID, backend, executable, run, stdout, stderr, cleanupAttachments)
	return ExternalAgentResult{OK: true}
}

func (s *ExternalAgentService) consumeAgentRun(requestID, backend, executable string, run *externalAgentRun, stdout, stderr io.Reader, cleanup func()) {
	defer cleanup()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			s.handleAgentOutputLine(requestID, backend, executable, run, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			run.mu.Lock()
			run.stderr.WriteString(err.Error())
			run.mu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 16*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			run.mu.Lock()
			if run.stderr.Len() < 64*1024 {
				run.stderr.WriteString(line + "\n")
			}
			run.mu.Unlock()
		}
	}()
	wg.Wait()
	waitErr := run.command.Wait()
	run.mu.Lock()
	emittedText := run.emittedText
	stderrText := strings.TrimSpace(run.stderr.String())
	run.mu.Unlock()
	s.mu.Lock()
	if s.active[requestID] == run {
		delete(s.active, requestID)
	}
	s.mu.Unlock()
	if waitErr != nil {
		message := stderrText
		if message == "" {
			message = waitErr.Error()
		}
		if emittedText {
			s.emitEvent(requestID, map[string]any{"type": "warning", "message": "Agent exited after returning partial output: " + message})
			s.emitPayload("ai:sdk-agent:done", requestID, nil)
			return
		}
		s.emitPayload("ai:sdk-agent:error", requestID, map[string]any{"error": message})
		return
	}
	if stderrText != "" {
		s.emitEvent(requestID, map[string]any{"type": "warning", "message": stderrText})
	}
	s.emitPayload("ai:sdk-agent:done", requestID, nil)
}

func stringAt(value any, path ...string) string {
	current := value
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = object[key]
	}
	text, _ := current.(string)
	return text
}

func numberAt(value any, path ...string) float64 {
	current := value
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return 0
		}
		current = object[key]
	}
	switch number := current.(type) {
	case float64:
		return number
	case json.Number:
		result, _ := number.Float64()
		return result
	case string:
		result, _ := strconv.ParseFloat(number, 64)
		return result
	}
	return 0
}

func firstString(value map[string]any, paths ...[]string) string {
	for _, path := range paths {
		if result := stringAt(value, path...); result != "" {
			return result
		}
	}
	return ""
}

func (s *ExternalAgentService) emitText(requestID string, run *externalAgentRun, text string) {
	if text == "" {
		return
	}
	run.mu.Lock()
	run.emittedText = true
	run.mu.Unlock()
	s.emitEvent(requestID, map[string]any{"type": "text-delta", "textDelta": text})
}

func textFromContent(value any) string {
	switch content := value.(type) {
	case string:
		return content
	case []any:
		parts := []string{}
		for _, item := range content {
			if object, ok := item.(map[string]any); ok {
				if text := firstString(object, []string{"text"}, []string{"content"}); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "")
	case map[string]any:
		return firstString(content, []string{"text"}, []string{"content"})
	}
	return ""
}

func (s *ExternalAgentService) handleAgentOutputLine(requestID, backend, executable string, run *externalAgentRun, line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}
	var event map[string]any
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	if decoder.Decode(&event) != nil {
		s.emitText(requestID, run, line+"\n")
		return
	}
	eventType := strings.ToLower(firstString(event, []string{"type"}, []string{"event"}, []string{"kind"}))
	if sessionID := firstString(event, []string{"session_id"}, []string{"sessionId"}, []string{"thread_id"}, []string{"threadId"}, []string{"data", "session_id"}); sessionID != "" {
		s.emitEvent(requestID, map[string]any{"type": "session-id", "sessionId": sessionID, "sdkBackend": backend, "binPath": executable, "runtime": "cli"})
	}
	if strings.Contains(eventType, "usage") || event["usage"] != nil {
		input := numberAt(event, "usage", "input_tokens")
		if input == 0 {
			input = numberAt(event, "input_tokens")
		}
		output := numberAt(event, "usage", "output_tokens")
		if output == 0 {
			output = numberAt(event, "output_tokens")
		}
		if input > 0 || output > 0 {
			s.emitEvent(requestID, map[string]any{"type": "usage", "inputTokens": input, "outputTokens": output, "totalTokens": input + output})
		}
	}
	if strings.Contains(eventType, "tool") && (strings.Contains(eventType, "use") || strings.Contains(eventType, "call") || strings.Contains(eventType, "start")) {
		name := firstString(event, []string{"tool_name"}, []string{"toolName"}, []string{"name"}, []string{"item", "name"})
		id := firstString(event, []string{"tool_call_id"}, []string{"toolCallId"}, []string{"id"}, []string{"item", "id"})
		input := event["input"]
		if input == nil {
			input = event["arguments"]
		}
		if input == nil {
			input = event["item"]
		}
		s.emitEvent(requestID, map[string]any{"type": "tool-call", "toolName": name, "toolCallId": id, "input": input})
		return
	}
	if strings.Contains(eventType, "tool") && (strings.Contains(eventType, "result") || strings.Contains(eventType, "end") || strings.Contains(eventType, "completed")) {
		id := firstString(event, []string{"tool_call_id"}, []string{"toolCallId"}, []string{"id"}, []string{"item", "id"})
		output := event["output"]
		if output == nil {
			output = event["result"]
		}
		if output == nil {
			output = event["item"]
		}
		s.emitEvent(requestID, map[string]any{"type": "tool-result", "toolCallId": id, "output": output})
		return
	}
	if strings.Contains(eventType, "reasoning") || strings.Contains(eventType, "thinking") {
		text := firstString(event, []string{"delta"}, []string{"text"}, []string{"content"}, []string{"data", "text"})
		if text != "" {
			s.emitEvent(requestID, map[string]any{"type": "reasoning-delta", "delta": text})
		}
		return
	}
	text := firstString(event,
		[]string{"textDelta"}, []string{"delta"}, []string{"text"}, []string{"content"}, []string{"result"}, []string{"data", "text"}, []string{"item", "text"}, []string{"item", "content"},
		[]string{"event", "delta", "text"}, []string{"event", "text"}, []string{"message", "text"},
	)
	if text == "" {
		if message, ok := event["message"].(map[string]any); ok {
			text = textFromContent(message["content"])
		}
	}
	if item, ok := event["item"].(map[string]any); ok {
		itemType := strings.ToLower(stringAt(item, "type"))
		if text == "" && (itemType == "agent_message" || itemType == "assistant_message") {
			text = firstString(item, []string{"text"}, []string{"content"})
		}
	}
	if text != "" {
		s.emitText(requestID, run, text)
		return
	}
	if eventType == "error" || strings.Contains(eventType, "failed") {
		message := firstString(event, []string{"error", "message"}, []string{"message"}, []string{"error"})
		if message != "" {
			run.mu.Lock()
			run.stderr.WriteString(message + "\n")
			run.mu.Unlock()
		}
	}
}

func (s *ExternalAgentService) Cancel(requestID, chatSessionID string) ExternalAgentResult {
	s.mu.Lock()
	var runs []*externalAgentRun
	for id, run := range s.active {
		if (requestID != "" && id == requestID) || (requestID == "" && chatSessionID != "" && run.chatSessionID == chatSessionID) {
			runs = append(runs, run)
		}
	}
	s.mu.Unlock()
	for _, run := range runs {
		run.cancel()
		if run.command.Process != nil {
			_ = run.command.Process.Kill()
		}
	}
	return ExternalAgentResult{OK: true}
}

func (s *ExternalAgentService) Cleanup(chatSessionID string) ExternalAgentResult {
	return s.Cancel("", chatSessionID)
}

func (s *ExternalAgentService) Steer(requestID, chatSessionID, prompt string, images []ExternalAgentImage, clientUserMessageID string) ExternalAgentSteerResult {
	s.mu.Lock()
	run := s.active[requestID]
	s.mu.Unlock()
	if run == nil || run.chatSessionID != chatSessionID {
		return ExternalAgentSteerResult{Status: "inactive"}
	}
	return ExternalAgentSteerResult{Status: "unsupported", Message: "This CLI accepts the next instruction as a new turn; the current output continues streaming."}
}

func (s *ExternalAgentService) ListModels(sdkBackend, cwd, providerID, chatSessionID string, agentEnv map[string]string, agentCommand, codexRuntime string) ExternalAgentModelsResult {
	if _, ok := normalizeExternalBackend(sdkBackend); !ok {
		return ExternalAgentModelsResult{Error: "unsupported external agent backend"}
	}
	return ExternalAgentModelsResult{OK: true, Models: []map[string]any{}, Warning: "The CLI does not expose a stable model catalog; LemonSSH presets remain available."}
}

func (s *ExternalAgentService) CodexAppServerStatus(agentCommand string, agentEnv map[string]string) ExternalAgentResult {
	path := s.resolveExecutable("codex", agentCommand)
	if path == "" {
		return ExternalAgentResult{Error: "codex executable was not found"}
	}
	output, err := runAgentCLI(path, "app-server", "--help")
	if err != nil && strings.TrimSpace(output) == "" {
		return ExternalAgentResult{Error: err.Error()}
	}
	return ExternalAgentResult{OK: true}
}

func (s *ExternalAgentService) AccountInfo(agentEnv map[string]string, agentCommand string) map[string]any {
	path := s.resolveExecutable("codex", agentCommand)
	if path == "" {
		return map[string]any{"ok": false, "error": "codex executable was not found"}
	}
	output, err := runAgentCLI(path, "login", "status")
	return map[string]any{"ok": err == nil, "account": map[string]any{"output": strings.TrimSpace(output)}, "error": errorString(err)}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
