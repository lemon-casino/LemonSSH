package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/lemon-casino/lemonssh/internal/rpc"
)

func externalToolMode(mode string) string {
	if mode == "skills" {
		return "skills"
	}
	return "mcp"
}

func resolveExternalAgentMCPPath() string {
	executable, _ := os.Executable()
	cwd, _ := os.Getwd()
	tool := resolveExternalAgentToolPathFrom(executable, cwd)
	name := "LemonSSH-mcp"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(tool), name)
}

func (s *ExternalAgentService) prepareToolContext(request *ExternalAgentStreamRequest) (func(), error) {
	request.ToolIntegrationMode = externalToolMode(request.ToolIntegrationMode)
	env := make(map[string]string, len(request.AgentEnv)+4)
	for k, v := range request.AgentEnv {
		env[k] = v
	}
	request.AgentEnv = env
	path := s.discoveryPath
	cleanup := func() {}
	if s.host != nil {
		if s.host.tokens == nil {
			return nil, fmt.Errorf("agent tool host is not running")
		}
		if s.tempRoot == "" {
			return nil, fmt.Errorf("managed agent temporary directory is unavailable")
		}
		dir, err := os.MkdirTemp(s.tempRoot, "agent-turn-")
		if err != nil {
			return nil, err
		}
		path = filepath.Join(dir, "discovery.json")
		token, err := s.host.tokens.Issue(rpc.Principal{ID: "agent:" + request.RequestID, Kind: rpc.PrincipalFirstParty, ChatSessionID: request.ChatSessionID})
		if err != nil {
			_ = os.RemoveAll(dir)
			return nil, err
		}
		discovery, err := rpc.LoadDiscovery(s.discoveryPath)
		if err == nil {
			discovery.Token = token
			err = rpc.WriteDiscovery(path, *discovery)
		}
		if err != nil {
			s.host.tokens.Revoke(token)
			_ = os.RemoveAll(dir)
			return nil, err
		}
		var once sync.Once
		cleanup = func() { once.Do(func() { s.host.tokens.Revoke(token); _ = os.RemoveAll(dir) }) }
	}
	if path != "" {
		env["LEMONSSH_TOOL_CLI_DISCOVERY_FILE"] = path
		env["NETCATTY_TOOL_CLI_DISCOVERY_FILE"] = path
	}
	env["LEMONSSH_CHAT_SESSION_ID"] = request.ChatSessionID
	// A managed chat must never inherit an unrelated external-MCP grant.
	env["LEMONSSH_EXTERNAL_MCP_DISCOVERY_FILE"] = ""
	env["NETCATTY_EXTERNAL_MCP_DISCOVERY_FILE"] = ""
	if request.ToolIntegrationMode == "mcp" && request.CWD == "" && (request.SDKBackend == "cursor" || request.SDKBackend == "grok") {
		if s.tempRoot == "" {
			cleanup()
			return nil, fmt.Errorf("managed agent temporary directory is unavailable")
		}
		cwd, err := os.MkdirTemp(s.tempRoot, "agent-workspace-")
		if err != nil {
			cleanup()
			return nil, err
		}
		request.CWD = cwd
		release := cleanup
		cleanup = func() { _ = os.RemoveAll(cwd); release() }
	}
	return cleanup, nil
}

func injectedMCPServer(request ExternalAgentStreamRequest) map[string]any {
	return map[string]any{
		"type": "stdio", "command": resolveExternalAgentMCPPath(), "args": []string{},
		"env": map[string]string{
			"LEMONSSH_TOOL_CLI_DISCOVERY_FILE":     request.AgentEnv["LEMONSSH_TOOL_CLI_DISCOVERY_FILE"],
			"LEMONSSH_CHAT_SESSION_ID":             request.ChatSessionID,
			"LEMONSSH_EXTERNAL_MCP_DISCOVERY_FILE": "", "NETCATTY_EXTERNAL_MCP_DISCOVERY_FILE": "",
		},
	}
}

func externalCodexConfig(request ExternalAgentStreamRequest) map[string]any {
	servers := map[string]any{}
	if externalToolMode(request.ToolIntegrationMode) == "mcp" {
		server := injectedMCPServer(request)
		delete(server, "type")
		server["default_tools_approval_mode"] = "approve"
		servers["lemonssh"] = server
	}
	return map[string]any{"mcp_servers": servers}
}

func (s *ExternalAgentService) configureAgentTools(request ExternalAgentStreamRequest, backend string, args []string, cwd string) ([]string, map[string]string, func(), error) {
	env := make(map[string]string, len(request.AgentEnv))
	for k, v := range request.AgentEnv {
		env[k] = v
	}
	cleanup := func() {}
	mode := externalToolMode(request.ToolIntegrationMode)
	if backend == "codebuddy" {
		env["CODEBUDDY_CODE_DISABLE_BACKGROUND_TASKS"] = "1"
	}
	servers := map[string]any{}
	if mode == "mcp" {
		servers["lemonssh"] = injectedMCPServer(request)
	}
	config, _ := json.Marshal(map[string]any{"mcpServers": servers})
	// Arguments precede the prompt/subcommand so CLIs cannot parse them as text.
	switch backend {
	case "claude", "codebuddy":
		flags := []string{"--mcp-config", string(config)}
		if backend == "claude" {
			tools := ""
			if mode == "skills" {
				tools = "Bash"
			}
			flags = append(flags, "--strict-mcp-config", "--tools", tools, "--disallowedTools", "AskUserQuestion")
			if mode == "mcp" {
				flags = append(flags, "--allowedTools", "mcp__lemonssh__*")
			}
		}
		args = append(flags, args...)
	case "codex":
		server := externalCodexConfig(request)["mcp_servers"].(map[string]any)
		keys := make([]string, 0, len(server))
		for k := range server {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		flags := []string{}
		if mode == "skills" {
			flags = append(flags, "-c", "mcp_servers={}")
		}
		for _, name := range keys {
			entry := server[name].(map[string]any)
			flags = append(flags, "-c", "mcp_servers."+name+".command="+quoteTOMLString(entry["command"].(string)), "-c", "mcp_servers."+name+".args=[]")
			vars := entry["env"].(map[string]string)
			names := make([]string, 0, len(vars))
			for key := range vars {
				names = append(names, key)
			}
			sort.Strings(names)
			for _, key := range names {
				flags = append(flags, "-c", "mcp_servers."+name+".env."+key+"="+quoteTOMLString(vars[key]))
			}
			flags = append(flags, "-c", "mcp_servers."+name+".default_tools_approval_mode=\"approve\"")
		}
		args = append(flags, args...)
	case "opencode":
		mcp := map[string]any{}
		if mode == "mcp" {
			server := injectedMCPServer(request)
			mcp["lemonssh"] = map[string]any{"type": "local", "command": []string{server["command"].(string)}, "environment": server["env"], "enabled": true}
		}
		bash := any("deny")
		if mode == "skills" {
			bash = map[string]string{"*": "deny", shellQuote(resolveExternalAgentToolPath()) + " *": "allow", resolveExternalAgentToolPath() + " *": "allow"}
		}
		data, _ := json.Marshal(map[string]any{"autoupdate": false, "share": "disabled", "mcp": mcp, "permission": map[string]any{"edit": "deny", "bash": bash, "question": "deny", "webfetch": "deny"}})
		env["OPENCODE_CONFIG_CONTENT"] = string(data)
	case "copilot":
		args = append([]string{"--additional-mcp-config", string(config)}, args...)
		if mode == "mcp" {
			args = append([]string{"--allow-tool", "lemonssh", "--deny-tool", "shell", "--deny-tool", "write"}, args...)
		}
	case "cursor", "grok":
		if mode == "mcp" {
			var err error
			cleanup, err = s.acquireWorkspaceMCP(cwd, backend, request)
			if err != nil {
				return nil, nil, nil, err
			}
			if backend == "cursor" {
				args = append([]string{"--approve-mcps"}, args...)
			}
			if backend == "grok" {
				args = append(args, "--disallowed-tools", "run_terminal_command,run_terminal_cmd,search_replace,write,Agent")
			}
		}
	}
	return args, env, cleanup, nil
}

func shellQuote(value string) string {
	if runtime.GOOS == "windows" {
		value = filepath.ToSlash(value)
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// The CLI host accepts arbitrary remote commands, but its local shell wrapper
// must remain one invocation. Expansion and shell operators outside quotes are
// refused even after the remote-command separator.
func isLemonSSHCLICommand(command, executable, chat string) bool {
	var tokens []string
	var token strings.Builder
	quote := rune(0)
	for _, r := range strings.TrimSpace(command) {
		if r == '\n' || r == '\r' || r == '\x00' || r == '`' || (r == '$' && quote != '\'') || r == '%' || r == '!' || r == '\\' {
			return false
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				token.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case ';', '|', '&', '<', '>', '(', ')':
			return false
		case ' ', '\t':
			if token.Len() > 0 {
				tokens = append(tokens, token.String())
				token.Reset()
			}
		default:
			token.WriteRune(r)
		}
	}
	if quote != 0 {
		return false
	}
	if token.Len() > 0 {
		tokens = append(tokens, token.String())
	}
	if len(tokens) < 2 {
		return false
	}
	if filepath.Clean(tokens[0]) != filepath.Clean(executable) && tokens[0] != strings.ReplaceAll(executable, "\\", "\\\\") {
		return false
	}
	if len(tokens) == 2 && (tokens[1] == "capabilities" || tokens[1] == "help" || tokens[1] == "--help") {
		return true
	}
	found := false
	for i := 1; i < len(tokens); i++ {
		if tokens[i] == "--" {
			break
		}
		if tokens[i] == "--chat-session" {
			if found || i+1 >= len(tokens) || tokens[i+1] != chat {
				return false
			}
			found = true
			i++
		} else if strings.HasPrefix(tokens[i], "--chat-session=") {
			if found || strings.TrimPrefix(tokens[i], "--chat-session=") != chat {
				return false
			}
			found = true
		}
	}
	return found
}

type workspaceMCPEntry struct {
	original []byte
	existed  bool
	written  []byte
	mode     os.FileMode
	key      string
	refs     int
}

var workspaceMCPFiles = struct {
	sync.Mutex
	entries map[string]*workspaceMCPEntry
}{entries: make(map[string]*workspaceMCPEntry)}

func (s *ExternalAgentService) acquireWorkspaceMCP(cwd, backend string, request ExternalAgentStreamRequest) (func(), error) {
	if cwd == "" {
		return nil, fmt.Errorf("%s MCP mode requires a working directory", backend)
	}
	name := filepath.Join(cwd, ".cursor", "mcp.json")
	if backend == "grok" {
		name = filepath.Join(cwd, ".grok", "config.toml")
	}
	name, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}
	server := injectedMCPServer(request)
	encoded, _ := json.Marshal(server)
	key := fmt.Sprintf("%x", sha256.Sum256(encoded))
	workspaceMCPFiles.Lock()
	defer workspaceMCPFiles.Unlock()
	if entry := workspaceMCPFiles.entries[name]; entry != nil {
		if entry.key != key {
			return nil, fmt.Errorf("%s workspace already has an active MCP chat; use another working directory or Skills mode", backend)
		}
		entry.refs++
		return workspaceMCPCleanup(name, entry), nil
	}
	original, readErr := os.ReadFile(name)
	if readErr != nil && !os.IsNotExist(readErr) {
		return nil, readErr
	}
	entry := &workspaceMCPEntry{original: original, existed: readErr == nil, key: key, refs: 1, mode: 0o600}
	if info, err := os.Lstat(name); err == nil {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("MCP configuration is not a regular file")
		}
		entry.mode = info.Mode().Perm()
	}
	if backend == "cursor" {
		doc := map[string]any{}
		if entry.existed && len(original) > 0 {
			if err := json.Unmarshal(original, &doc); err != nil {
				return nil, fmt.Errorf("invalid workspace MCP configuration: %w", err)
			}
		}
		if doc == nil {
			return nil, fmt.Errorf("workspace MCP configuration must be an object")
		}
		servers, ok := doc["mcpServers"].(map[string]any)
		if !ok && doc["mcpServers"] != nil {
			return nil, fmt.Errorf("mcpServers must be an object")
		}
		if servers == nil {
			servers = map[string]any{}
		}
		if servers["lemonssh"] != nil {
			return nil, fmt.Errorf("workspace already defines a lemonssh MCP server; refusing to overwrite it")
		}
		servers["lemonssh"] = server
		doc["mcpServers"] = servers
		entry.written, _ = json.MarshalIndent(doc, "", "  ")
	} else {
		if strings.Contains(string(original), "[mcp_servers.lemonssh]") {
			return nil, fmt.Errorf("workspace already defines a lemonssh MCP server; refusing to overwrite it")
		}
		vars := server["env"].(map[string]string)
		keys := make([]string, 0, len(vars))
		for key := range vars {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		pairs := []string{}
		for _, key := range keys {
			pairs = append(pairs, key+" = "+quoteTOMLString(vars[key]))
		}
		entry.written = []byte(string(original) + "\n[mcp_servers.lemonssh]\ncommand = " + quoteTOMLString(server["command"].(string)) + "\nargs = []\nenv = { " + strings.Join(pairs, ", ") + " }\nenabled = true\n")
	}
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(name, entry.written, entry.mode); err != nil {
		return nil, err
	}
	workspaceMCPFiles.entries[name] = entry
	return workspaceMCPCleanup(name, entry), nil
}

func workspaceMCPCleanup(name string, entry *workspaceMCPEntry) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			workspaceMCPFiles.Lock()
			defer workspaceMCPFiles.Unlock()
			if workspaceMCPFiles.entries[name] != entry {
				return
			}
			entry.refs--
			if entry.refs > 0 {
				return
			}
			delete(workspaceMCPFiles.entries, name)
			current, err := os.ReadFile(name)
			// Do not overwrite edits made while the agent was running.
			if err != nil || string(current) != string(entry.written) {
				return
			}
			if entry.existed {
				_ = os.WriteFile(name, entry.original, entry.mode)
			} else {
				_ = os.Remove(name)
			}
		})
	}
}
