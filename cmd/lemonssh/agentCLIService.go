package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

var managedAgentExecutables = map[string]string{
	"codex":     "codex",
	"claude":    "claude",
	"copilot":   "copilot",
	"cursor":    "cursor-agent",
	"codebuddy": "codebuddy",
	"opencode":  "opencode",
	"grok":      "grok",
}

type AgentCLIPathInfo struct {
	Path          string  `json:"path"`
	BinPath       string  `json:"binPath"`
	Version       *string `json:"version"`
	Available     bool    `json:"available"`
	Installed     bool    `json:"installed"`
	Authenticated bool    `json:"authenticated"`
	AuthSource    *string `json:"authSource"`
	CLIEmail      string  `json:"cliEmail"`
	CLIBinPath    string  `json:"cliBinPath"`
	CLILoginOK    bool    `json:"cliLoginOk"`
	APIKeyOK      bool    `json:"apiKeyOk"`
	SDKInstalled  bool    `json:"sdkInstalled"`
}

type DiscoveredAgentCLI struct {
	Command string `json:"command"`
	AgentCLIPathInfo
}

type AgentCLIPrewarmResult struct {
	OK        bool   `json:"ok"`
	Refreshed bool   `json:"refreshed"`
	Error     string `json:"error,omitempty"`
}

type AgentCLIService struct {
	mu            sync.Mutex
	loginSessions map[string]*CodexLoginSession
	loginCommands map[string]*exec.Cmd

	// envMu guards the one-shot login-shell environment warm-up (F11):
	// Prewarm, Resolve and Discover share it so the shell probe runs at
	// most once per process and the merged PATH stays sticky.
	envMu             sync.Mutex
	shellEnvRefreshed bool
	shellEnvApplied   bool
	// refreshShellEnvForTest overrides the login-shell probe in tests.
	refreshShellEnvForTest func() (bool, error)
}

func newAgentCLIService() *AgentCLIService {
	return &AgentCLIService{loginSessions: map[string]*CodexLoginSession{}, loginCommands: map[string]*exec.Cmd{}}
}

func (s *AgentCLIService) Resolve(command, customPath string, refreshShellEnv bool, apiKeyPresent bool) AgentCLIPathInfo {
	if refreshShellEnv {
		_, _ = s.refreshShellEnvOnce()
	}
	return resolveAgentCLI(command, customPath, apiKeyPresent)
}

func (s *AgentCLIService) Discover(refreshShellEnv bool, apiKeyPresent bool) []DiscoveredAgentCLI {
	if refreshShellEnv {
		_, _ = s.refreshShellEnvOnce()
	}
	keys := make([]string, 0, len(managedAgentExecutables))
	for key := range managedAgentExecutables {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]DiscoveredAgentCLI, 0, len(keys))
	for _, key := range keys {
		info := resolveAgentCLI(key, "", apiKeyPresent)
		if info.Available {
			result = append(result, DiscoveredAgentCLI{Command: key, AgentCLIPathInfo: info})
		}
	}
	return result
}

// Prewarm warms the process environment from the user's login shell so
// GUI-launched instances (which inherit a minimal PATH — macOS Finder
// launches are the classic case) can still discover shell-configured CLIs.
// Best effort: the call stays ok:true and reports the failure inline, so a
// broken shell init never blocks discovery.
func (s *AgentCLIService) Prewarm() AgentCLIPrewarmResult {
	refreshed, err := s.refreshShellEnvOnce()
	if err != nil {
		return AgentCLIPrewarmResult{OK: true, Refreshed: refreshed, Error: err.Error()}
	}
	return AgentCLIPrewarmResult{OK: true, Refreshed: refreshed}
}

// refreshShellEnvOnce applies the login-shell environment at most once per
// process: the merged PATH is sticky, so repeated Resolve/Discover calls
// are cheap no-ops after the first warm-up.
func (s *AgentCLIService) refreshShellEnvOnce() (bool, error) {
	s.envMu.Lock()
	defer s.envMu.Unlock()
	if s.shellEnvRefreshed {
		return s.shellEnvApplied, nil
	}
	s.shellEnvRefreshed = true
	refresh := s.refreshShellEnvForTest
	if refresh == nil {
		refresh = applyLoginShellEnv
	}
	applied, err := refresh()
	s.shellEnvApplied = applied
	return applied, err
}

const shellEnvPathBegin = "LEMONSSH_PATH_BEGIN"
const shellEnvPathEnd = "LEMONSSH_PATH_END"

// applyLoginShellEnv runs the user's login shell and merges its PATH into
// this process. Login + interactive mode sources the same profile/rc files
// the user's terminal does (nvm, homebrew shellenv, ~/.zprofile, ...), which
// is exactly the PATH slice GUI launches miss. Unix only; Windows keeps its
// registry-built PATH.
func applyLoginShellEnv() (bool, error) {
	if runtime.GOOS == "windows" {
		return false, nil
	}
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		if runtime.GOOS == "darwin" {
			shell = "/bin/zsh"
		} else {
			shell = "/bin/bash"
		}
	}
	if _, err := os.Stat(shell); err != nil {
		return false, fmt.Errorf("login shell %s is not available", shell)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	script := fmt.Sprintf(`printf '\n%s\n%%s\n%s\n' "$PATH"`, shellEnvPathBegin, shellEnvPathEnd)
	command := exec.CommandContext(ctx, shell, "-l", "-i", "-c", script)
	output, err := command.Output()
	if err != nil && ctx.Err() == context.DeadlineExceeded {
		return false, fmt.Errorf("login shell probe timed out")
	}
	shellPath := strings.TrimSpace(extractMarkedSegment(string(output), shellEnvPathBegin, shellEnvPathEnd))
	if shellPath == "" {
		if err != nil {
			return false, fmt.Errorf("login shell probe failed: %w", err)
		}
		return false, fmt.Errorf("login shell returned an empty PATH")
	}
	os.Setenv("PATH", mergePathList(shellPath, os.Getenv("PATH")))
	return true, nil
}

// extractMarkedSegment returns the text between the last begin marker line
// and the first end marker line after it. Login/rc files may print
// arbitrary output, so markers — not line positions — delimit the payload.
func extractMarkedSegment(output, begin, end string) string {
	beginMarker := "\n" + begin + "\n"
	endMarker := "\n" + end + "\n"
	for beginIndex := strings.LastIndex(output, beginMarker); beginIndex >= 0; beginIndex = strings.LastIndex(output[:beginIndex], beginMarker) {
		rest := output[beginIndex+len(beginMarker):]
		if endIndex := strings.Index(rest, endMarker); endIndex >= 0 {
			return rest[:endIndex]
		}
	}
	return ""
}

// mergePathList builds the warmed PATH: the login shell's entries win, the
// original process entries stay as fallback (deduped, order preserved).
func mergePathList(shellPath, currentPath string) string {
	seen := map[string]bool{}
	merged := make([]string, 0, 32)
	add := func(list string) {
		for _, entry := range filepath.SplitList(list) {
			entry = strings.TrimSpace(entry)
			if entry == "" || seen[entry] {
				continue
			}
			seen[entry] = true
			merged = append(merged, entry)
		}
	}
	add(shellPath)
	add(currentPath)
	return strings.Join(merged, string(os.PathListSeparator))
}

func resolveAgentCLI(command, customPath string, apiKeyPresent bool) AgentCLIPathInfo {
	executable, allowed := managedAgentExecutables[strings.ToLower(strings.TrimSpace(command))]
	if !allowed {
		return AgentCLIPathInfo{}
	}

	candidate := findAgentExecutable(executable, strings.TrimSpace(customPath))
	if candidate == "" {
		return AgentCLIPathInfo{Path: strings.TrimSpace(customPath), APIKeyOK: apiKeyPresent}
	}
	candidate, _ = filepath.Abs(candidate)
	versionOutput, err := runAgentCLI(candidate, "--version")
	if err != nil {
		return AgentCLIPathInfo{Path: candidate, BinPath: candidate, APIKeyOK: apiKeyPresent}
	}
	version := firstOutputLine(versionOutput)
	info := AgentCLIPathInfo{
		Path:         candidate,
		BinPath:      candidate,
		Available:    true,
		Installed:    true,
		APIKeyOK:     apiKeyPresent,
		SDKInstalled: true,
	}
	if version != "" {
		info.Version = &version
	}
	if command == "cursor" {
		info.CLIBinPath = candidate
		info.CLILoginOK, info.CLIEmail = probeCursorLogin(candidate)
		info.Authenticated = info.CLILoginOK || apiKeyPresent
		if info.CLILoginOK {
			source := "cli-login"
			info.AuthSource = &source
		} else if apiKeyPresent {
			source := "settings"
			info.AuthSource = &source
		}
	}
	return info
}

func findAgentExecutable(executable, customPath string) string {
	if customPath != "" {
		if info, err := os.Stat(customPath); err == nil && info.IsDir() {
			return findExecutableInDirectory(customPath, executable)
		}
		if isManagedExecutablePath(customPath, executable) {
			if info, err := os.Stat(customPath); err == nil && !info.IsDir() {
				return customPath
			}
		}
		return ""
	}

	if resolved, err := exec.LookPath(executable); err == nil {
		return resolved
	}
	for _, directory := range commonAgentDirectories() {
		if found := findExecutableInDirectory(directory, executable); found != "" {
			return found
		}
	}
	return ""
}

func findExecutableInDirectory(directory, executable string) string {
	for _, name := range executableNames(executable) {
		candidate := filepath.Join(directory, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func executableNames(executable string) []string {
	if runtime.GOOS != "windows" {
		return []string{executable}
	}
	return []string{executable + ".exe", executable + ".cmd", executable + ".bat", executable + ".com", executable}
}

func isManagedExecutablePath(candidate, executable string) bool {
	base := strings.ToLower(filepath.Base(candidate))
	for _, name := range executableNames(executable) {
		if base == strings.ToLower(name) {
			return true
		}
	}
	return false
}

func commonAgentDirectories() []string {
	seen := map[string]bool{}
	var directories []string
	add := func(directory string) {
		directory = strings.TrimSpace(directory)
		if directory == "" || seen[directory] {
			return
		}
		seen[directory] = true
		directories = append(directories, directory)
	}
	for _, directory := range filepath.SplitList(os.Getenv("PATH")) {
		add(directory)
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, ".local", "bin"))
		add(filepath.Join(home, "bin"))
	}
	if runtime.GOOS == "windows" {
		add(filepath.Join(os.Getenv("APPDATA"), "npm"))
		add(filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs"))
	} else {
		add("/opt/homebrew/bin")
		add("/usr/local/bin")
	}
	return directories
}

func runAgentCLI(path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var command *exec.Cmd
	lower := strings.ToLower(path)
	if runtime.GOOS == "windows" && (strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat")) {
		command = exec.CommandContext(ctx, "cmd.exe", append([]string{"/D", "/S", "/C", "call", path}, args...)...)
	} else {
		command = exec.CommandContext(ctx, path, args...)
	}
	output, err := command.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return string(output), ctx.Err()
	}
	return string(output), err
}

func firstOutputLine(output string) string {
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			if len(trimmed) > 200 {
				return trimmed[:200]
			}
			return trimmed
		}
	}
	return ""
}

func probeCursorLogin(path string) (bool, string) {
	output, err := runAgentCLI(path, "status", "--format", "json")
	if err != nil {
		return false, ""
	}
	var payload map[string]any
	if json.Unmarshal([]byte(output), &payload) != nil {
		return false, ""
	}
	authenticated, _ := payload["isAuthenticated"].(bool)
	if !authenticated {
		status := strings.ToLower(stringValue(payload["status"]))
		authenticated = status == "authenticated" || status == "logged_in" || status == "logged-in"
	}
	email := stringValue(payload["email"])
	if userInfo, ok := payload["userInfo"].(map[string]any); ok && email == "" {
		email = stringValue(userInfo["email"])
	}
	return authenticated, email
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

type CodexIntegrationOptions struct {
	RefreshShellEnv     bool   `json:"refreshShellEnv,omitempty"`
	ValidateChatGPTAuth bool   `json:"validateChatGptAuth,omitempty"`
	CodexPath           string `json:"codexPath,omitempty"`
}

type CodexIntegrationStatus struct {
	State        string                 `json:"state"`
	IsConnected  bool                   `json:"isConnected"`
	RawOutput    string                 `json:"rawOutput"`
	ExitCode     *int                   `json:"exitCode"`
	CustomConfig map[string]interface{} `json:"customConfig,omitempty"`
}

type CodexLoginSession struct {
	SessionID string `json:"sessionId"`
	State     string `json:"state"`
	URL       string `json:"url,omitempty"`
	Output    string `json:"output"`
	Error     string `json:"error,omitempty"`
	ExitCode  *int   `json:"exitCode"`
	CodexPath string `json:"codexPath,omitempty"`
}

type CodexLoginResult struct {
	OK      bool               `json:"ok"`
	Found   bool               `json:"found,omitempty"`
	Session *CodexLoginSession `json:"session,omitempty"`
	Error   string             `json:"error,omitempty"`
}

type CodexLogoutResult struct {
	OK           bool   `json:"ok"`
	State        string `json:"state,omitempty"`
	IsConnected  bool   `json:"isConnected,omitempty"`
	RawOutput    string `json:"rawOutput,omitempty"`
	LogoutOutput string `json:"logoutOutput,omitempty"`
	Error        string `json:"error,omitempty"`
}

func exitCode(err error) *int {
	if err == nil {
		code := 0
		return &code
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		return &code
	}
	return nil
}

func classifyCodexIntegration(output string, err error) CodexIntegrationStatus {
	lower := strings.ToLower(output)
	state := "unknown"
	connected := false
	switch {
	case strings.Contains(lower, "chatgpt") && (strings.Contains(lower, "logged in") || strings.Contains(lower, "connected")):
		state, connected = "connected_chatgpt", true
	case strings.Contains(lower, "api key") && (strings.Contains(lower, "logged in") || strings.Contains(lower, "connected")):
		state, connected = "connected_api_key", true
	case strings.Contains(lower, "not logged in") || strings.Contains(lower, "not authenticated"):
		state = "not_logged_in"
	case err == nil && strings.TrimSpace(output) != "":
		state, connected = "connected_custom_config", true
	}
	return CodexIntegrationStatus{State: state, IsConnected: connected, RawOutput: strings.TrimSpace(output), ExitCode: exitCode(err)}
}

func (s *AgentCLIService) resolveCodex(customPath string) string {
	return findAgentExecutable(managedAgentExecutables["codex"], strings.TrimSpace(customPath))
}

func (s *AgentCLIService) CodexGetIntegration(options CodexIntegrationOptions) CodexIntegrationStatus {
	path := s.resolveCodex(options.CodexPath)
	if path == "" {
		code := -1
		return CodexIntegrationStatus{State: "not_logged_in", RawOutput: "codex executable was not found", ExitCode: &code}
	}
	output, err := runAgentCLI(path, "login", "status")
	return classifyCodexIntegration(output, err)
}

func newLoginSessionID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return "codex-login-" + hex.EncodeToString(bytes[:])
	}
	return fmt.Sprintf("codex-login-%d", time.Now().UnixNano())
}

var loginURLPattern = regexp.MustCompile(`https?://[^\s]+`)

type codexLoginWriter struct {
	service   *AgentCLIService
	sessionID string
}

func (w codexLoginWriter) Write(data []byte) (int, error) {
	w.service.mu.Lock()
	defer w.service.mu.Unlock()
	if session := w.service.loginSessions[w.sessionID]; session != nil {
		if len(session.Output) < 256*1024 {
			session.Output += string(data)
		}
		if session.URL == "" {
			match := loginURLPattern.FindString(session.Output)
			session.URL = strings.TrimRight(match, ".,;)")
		}
	}
	return len(data), nil
}

func cloneCodexLoginSession(value *CodexLoginSession) *CodexLoginSession {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (s *AgentCLIService) CodexStartLogin(options CodexIntegrationOptions) CodexLoginResult {
	path := s.resolveCodex(options.CodexPath)
	if path == "" {
		return CodexLoginResult{Error: "codex executable was not found"}
	}
	id := newLoginSessionID()
	ctx := context.Background()
	command := streamingCommand(ctx, path, []string{"login", "--device-auth"})
	writer := codexLoginWriter{service: s, sessionID: id}
	command.Stdout, command.Stderr = writer, writer
	session := &CodexLoginSession{SessionID: id, State: "running", CodexPath: path}
	s.mu.Lock()
	s.loginSessions[id] = session
	s.loginCommands[id] = command
	s.mu.Unlock()
	if err := command.Start(); err != nil {
		s.mu.Lock()
		session.State, session.Error = "error", err.Error()
		delete(s.loginCommands, id)
		result := cloneCodexLoginSession(session)
		s.mu.Unlock()
		return CodexLoginResult{Error: err.Error(), Session: result}
	}
	go func() {
		err := command.Wait()
		s.mu.Lock()
		defer s.mu.Unlock()
		current := s.loginSessions[id]
		delete(s.loginCommands, id)
		if current == nil || current.State == "cancelled" {
			return
		}
		current.ExitCode = exitCode(err)
		if err != nil {
			current.State = "error"
			current.Error = err.Error()
		} else {
			current.State = "success"
		}
	}()
	s.mu.Lock()
	result := cloneCodexLoginSession(session)
	s.mu.Unlock()
	return CodexLoginResult{OK: true, Session: result}
}

func (s *AgentCLIService) CodexGetLoginSession(sessionID string) CodexLoginResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := cloneCodexLoginSession(s.loginSessions[sessionID])
	if session == nil {
		return CodexLoginResult{OK: true, Found: false}
	}
	return CodexLoginResult{OK: true, Found: true, Session: session}
}

func (s *AgentCLIService) CodexCancelLogin(sessionID string) CodexLoginResult {
	s.mu.Lock()
	session := s.loginSessions[sessionID]
	command := s.loginCommands[sessionID]
	if session != nil && session.State == "running" {
		session.State = "cancelled"
		if command != nil && command.Process != nil {
			_ = command.Process.Kill()
		}
	}
	copy := cloneCodexLoginSession(session)
	delete(s.loginCommands, sessionID)
	s.mu.Unlock()
	if copy == nil {
		return CodexLoginResult{OK: true, Found: false}
	}
	return CodexLoginResult{OK: true, Found: true, Session: copy}
}

func (s *AgentCLIService) CodexLogout(options CodexIntegrationOptions) CodexLogoutResult {
	path := s.resolveCodex(options.CodexPath)
	if path == "" {
		return CodexLogoutResult{Error: "codex executable was not found"}
	}
	output, err := runAgentCLI(path, "logout")
	status := s.CodexGetIntegration(options)
	return CodexLogoutResult{OK: err == nil, State: status.State, IsConnected: status.IsConnected, RawOutput: status.RawOutput, LogoutOutput: strings.TrimSpace(output), Error: errorString(err)}
}
