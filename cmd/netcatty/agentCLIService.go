package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
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
	OK bool `json:"ok"`
}

type AgentCLIService struct{}

func newAgentCLIService() *AgentCLIService { return &AgentCLIService{} }

func (s *AgentCLIService) Resolve(command, customPath string, refreshShellEnv bool, apiKeyPresent bool) AgentCLIPathInfo {
	_ = refreshShellEnv
	return resolveAgentCLI(command, customPath, apiKeyPresent)
}

func (s *AgentCLIService) Discover(refreshShellEnv bool, apiKeyPresent bool) []DiscoveredAgentCLI {
	_ = refreshShellEnv
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

func (s *AgentCLIService) Prewarm() AgentCLIPrewarmResult {
	return AgentCLIPrewarmResult{OK: true}
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
