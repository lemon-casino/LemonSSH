package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Per-client external MCP setup for the Codex CLI, Claude Code and the Grok
// CLI. The retired Electron sidecar spawned each CLI to probe and register
// the MCP server (electron/bridges/externalMcp/*.cjs, removed in bcd94770);
// the Wails shell instead reads and merges the same registration directly
// into each client's config file, so no CLI process ever runs inside the
// desktop app. Writes are incremental merges: the original content is read
// first, only the lemonssh-external entry is replaced, and a same-directory
// .bak backup is written before the file is modified.

const (
	externalMcpServerName      = "lemonssh-external"
	externalMcpDiscoveryEnvVar = "LEMONSSH_EXTERNAL_MCP_DISCOVERY_FILE"
)

type externalClientKind string

const (
	externalClientCodex  externalClientKind = "codex"
	externalClientClaude externalClientKind = "claude"
	externalClientGrok   externalClientKind = "grok"
)

// Client setup states consumed by ExternalMcpCard (ClientSetupStatus.state).
const (
	externalClientStateConfigured    = "configured"
	externalClientStateNotConfigured = "not_configured"
	externalClientStateConflict      = "conflict"
	externalClientStateError         = "error"
)

// ExternalClientSetupStatus mirrors the ClientSetupStatus shape rendered by
// components/settings/tabs/ai/ExternalMcpCard.tsx.
type ExternalClientSetupStatus struct {
	OK              bool   `json:"ok"`
	State           string `json:"state"`
	LauncherPath    string `json:"launcherPath"`
	ConfigPath      string `json:"configPath"`
	Command         string `json:"command"`
	ExistingCommand string `json:"existingCommand"`
	Error           string `json:"error,omitempty"`
}

func (s ExternalClientSetupStatus) toMap() map[string]any {
	m := map[string]any{
		"ok":              s.OK,
		"state":           s.State,
		"launcherPath":    s.LauncherPath,
		"configPath":      s.ConfigPath,
		"command":         s.Command,
		"existingCommand": s.ExistingCommand,
	}
	if s.Error != "" {
		m["error"] = s.Error
	}
	return m
}

// externalClientProbe carries resolved paths for one CLI client. Pure tests
// pass explicit paths; the AgentService surface resolves them from the real
// environment.
type externalClientProbe struct {
	client        externalClientKind
	configPath    string
	launcherPath  string
	discoveryPath string
}

// notFoundState mirrors the per-client *_not_found states the card maps to
// install hints.
func (p externalClientProbe) notFoundState() string {
	switch p.client {
	case externalClientClaude:
		return "claude_not_found"
	case externalClientGrok:
		return "grok_not_found"
	default:
		return "codex_not_found"
	}
}

// configPathForClient resolves the user-scope config file location. Env
// overrides match the CLIs' own resolution (CODEX_HOME, GROK_CONFIG_DIR).
func configPathForClient(client externalClientKind, homeDir string) string {
	switch client {
	case externalClientCodex:
		if dir := strings.TrimSpace(os.Getenv("CODEX_HOME")); dir != "" {
			return filepath.Join(dir, "config.toml")
		}
		return filepath.Join(homeDir, ".codex", "config.toml")
	case externalClientClaude:
		return filepath.Join(homeDir, ".claude.json")
	case externalClientGrok:
		if dir := strings.TrimSpace(os.Getenv("GROK_CONFIG_DIR")); dir != "" {
			return filepath.Join(dir, "config.toml")
		}
		return filepath.Join(homeDir, ".grok", "config.toml")
	}
	return ""
}

// newExternalClientProbe resolves paths from the real environment.
func newExternalClientProbe(client externalClientKind, launcherPath, discoveryPath string) (externalClientProbe, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return externalClientProbe{}, fmt.Errorf("resolve home directory: %w", err)
	}
	return externalClientProbe{
		client:        client,
		configPath:    configPathForClient(client, home),
		launcherPath:  launcherPath,
		discoveryPath: discoveryPath,
	}, nil
}

// quoteExternalCliArg matches ExternalMcpCard's quoteShellArg so backend
// commands and frontend fallback markup render identically.
func quoteExternalCliArg(value string) string {
	if value == "" {
		return `""`
	}
	if !strings.ContainsAny(value, " \"'\\") {
		return value
	}
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

// formatExternalClientAddCommand mirrors the copyable commands built by
// ExternalMcpCard (formatCodexAddCommand / formatClaudeAddCommand /
// formatGrokAddCommand) so the backend status command is authoritative.
func formatExternalClientAddCommand(probe externalClientProbe) string {
	cli, envFlag := "", ""
	switch probe.client {
	case externalClientCodex:
		cli, envFlag = "codex", "--env"
	case externalClientClaude:
		cli, envFlag = "claude", "-e"
	case externalClientGrok:
		cli, envFlag = "grok", "-e"
	}
	parts := []string{cli, "mcp", "add"}
	if probe.client == externalClientClaude {
		parts = append(parts, "-s", "user")
	}
	parts = append(parts, externalMcpServerName)
	if probe.discoveryPath != "" {
		parts = append(parts, envFlag, externalMcpDiscoveryEnvVar+"="+quoteExternalCliArg(probe.discoveryPath))
	}
	parts = append(parts, "--", quoteExternalCliArg(probe.launcherPath))
	return strings.Join(parts, " ")
}

// normalizeExternalMcpPath compares entry commands against the launcher the
// way the legacy setups did: trim quotes, ignore .cmd wrappers, Windows case.
func normalizeExternalMcpPath(value string) string {
	normalized := strings.TrimSpace(value)
	normalized = strings.Trim(normalized, `"'`)
	if runtime.GOOS == "windows" {
		normalized = strings.TrimSuffix(normalized, ".cmd")
		normalized = strings.ToLower(normalized)
	}
	return normalized
}

func externalMcpPathsMatch(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	return normalizeExternalMcpPath(a) == normalizeExternalMcpPath(b)
}

// classifyExternalEntryMatch applies the shared rules: the entry counts as
// configured only when it launches exactly our launcher with no extra args
// and carries the current discovery env (when a discovery path is known).
// A stale or missing env, or a disabled entry, stays rewritable
// (not_configured); a foreign command or extra args is a conflict the user
// must resolve, mirroring the legacy CLI-based classification.
func classifyExternalEntryMatch(command string, args []string, argsKnown bool, env map[string]string, envKnown bool, enabled bool, probe externalClientProbe) string {
	if !enabled {
		return externalClientStateNotConfigured
	}
	if command == "" || !argsKnown || len(args) > 0 {
		// Present but not a plain launcher command we can interpret.
		return externalClientStateConflict
	}
	if !externalMcpPathsMatch(command, probe.launcherPath) {
		return externalClientStateConflict
	}
	if probe.discoveryPath != "" {
		if !envKnown || env[externalMcpDiscoveryEnvVar] != probe.discoveryPath {
			return externalClientStateNotConfigured
		}
	}
	return externalClientStateConfigured
}

// classifyExternalClientStatus probes one client's config file without
// modifying it.
func classifyExternalClientStatus(probe externalClientProbe) ExternalClientSetupStatus {
	status := ExternalClientSetupStatus{
		OK:           true,
		LauncherPath: probe.launcherPath,
		ConfigPath:   probe.configPath,
		Command:      formatExternalClientAddCommand(probe),
	}
	content, err := os.ReadFile(probe.configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			status.State = probe.notFoundState()
			return status
		}
		status.State = externalClientStateError
		status.Error = err.Error()
		return status
	}
	if probe.client == externalClientClaude {
		state, existingCommand, err := classifyClaudeConfig(content, probe)
		if err != nil {
			status.State = externalClientStateError
			status.Error = err.Error()
			return status
		}
		status.State = state
		status.ExistingCommand = existingCommand
		return status
	}
	entry := parseTomlMcpEntry(string(content), externalMcpServerName)
	if !entry.found {
		status.State = externalClientStateNotConfigured
		return status
	}
	status.ExistingCommand = strings.TrimSpace(strings.Join(append([]string{entry.command}, entry.args...), " "))
	status.State = classifyExternalEntryMatch(entry.command, entry.args, entry.argsKnown, entry.env, entry.envKnown, entry.enabled, probe)
	return status
}

// addExternalClientEntry merges the lemonssh-external entry into the client
// config. Legacy semantics are preserved: configs of unidentified clients,
// conflicting entries and already-correct registrations are returned as-is
// and never rewritten.
func addExternalClientEntry(probe externalClientProbe) ExternalClientSetupStatus {
	status := classifyExternalClientStatus(probe)
	switch status.State {
	case probe.notFoundState(), externalClientStateConfigured, externalClientStateConflict, externalClientStateError:
		return status
	}
	content, err := os.ReadFile(probe.configPath)
	if err != nil {
		status.State = externalClientStateError
		status.Error = err.Error()
		return status
	}
	var merged []byte
	if probe.client == externalClientClaude {
		merged, err = mergeClaudeMcpServerEntry(content, probe)
		if err != nil {
			status.State = externalClientStateError
			status.Error = err.Error()
			return status
		}
	} else {
		merged = mergeTomlMcpServerEntry(content, probe)
	}
	if err := writeExternalClientConfigWithBackup(probe.configPath, merged); err != nil {
		status.State = externalClientStateError
		status.Error = err.Error()
		return status
	}
	return classifyExternalClientStatus(probe)
}

// writeExternalClientConfigWithBackup leaves configPath+".bak" with the
// original bytes in the same directory, then replaces the file. The original
// file mode is preserved.
func writeExternalClientConfigWithBackup(configPath string, next []byte) error {
	info, err := os.Stat(configPath)
	if err != nil {
		return err
	}
	original, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	backupPath := configPath + ".bak"
	if err := os.WriteFile(backupPath, original, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write backup %s: %w", backupPath, err)
	}
	return os.WriteFile(configPath, next, info.Mode().Perm())
}

// --- TOML clients (Codex config.toml, Grok config.toml) ---

type tomlMcpEntry struct {
	found     bool
	command   string
	args      []string
	argsKnown bool
	env       map[string]string
	envKnown  bool
	enabled   bool
}

// parseTomlMcpEntry extracts the [mcp_servers.<name>] table. Parsing is
// deliberately line-oriented: only our own entry is interpreted, everything
// else in the file is opaque content that merges must preserve byte for byte.
func parseTomlMcpEntry(content, name string) tomlMcpEntry {
	header := "[mcp_servers." + name + "]"
	entry := tomlMcpEntry{enabled: true}
	inSection := false
	sawSection := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inSection = trimmed == header
			if inSection {
				sawSection = true
			}
			continue
		}
		if !inSection || trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "command":
			entry.command = unquoteTOMLString(value)
		case "args":
			entry.args, entry.argsKnown = parseTOMLStringArray(value)
		case "env":
			entry.env, entry.envKnown = parseTOMLInlineStringTable(value)
		case "enabled":
			entry.enabled = unquoteTOMLString(value) != "false"
		}
	}
	entry.found = sawSection
	return entry
}

// stripTomlMcpServerSection removes the [mcp_servers.<name>] table and its
// nested subsections while leaving every other line untouched.
func stripTomlMcpServerSection(content, name string) string {
	headerExact := "[mcp_servers." + name + "]"
	headerNested := "[mcp_servers." + name + "."
	var kept []string
	skipping := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			skipping = trimmed == headerExact || strings.HasPrefix(trimmed, headerNested)
			if !skipping {
				kept = append(kept, line)
			}
			continue
		}
		if !skipping {
			kept = append(kept, line)
		}
	}
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	return strings.Join(kept, "\n")
}

func buildTomlMcpServerSection(probe externalClientProbe) string {
	var b strings.Builder
	b.WriteString("[mcp_servers." + externalMcpServerName + "]\n")
	b.WriteString("command = " + quoteTOMLString(probe.launcherPath) + "\n")
	b.WriteString("args = []\n")
	if probe.discoveryPath != "" {
		b.WriteString("env = { " + externalMcpDiscoveryEnvVar + " = " + quoteTOMLString(probe.discoveryPath) + " }\n")
	}
	return b.String()
}

// mergeTomlMcpServerEntry strips any previous lemonssh-external table and
// appends a fresh one; all other content survives unchanged.
func mergeTomlMcpServerEntry(original []byte, probe externalClientProbe) []byte {
	body := strings.TrimRight(stripTomlMcpServerSection(string(original), externalMcpServerName), "\r\n \t")
	section := strings.TrimRight(buildTomlMcpServerSection(probe), "\n")
	if body == "" {
		return []byte(section + "\n")
	}
	return []byte(body + "\n\n" + section + "\n")
}

func quoteTOMLString(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

func unquoteTOMLString(value string) string {
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1] // TOML literal string: no escapes.
	}
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return value
	}
	var b strings.Builder
	inner := value[1 : len(value)-1]
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if c != '\\' || i+1 >= len(inner) {
			b.WriteByte(c)
			continue
		}
		i++
		switch inner[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '"', '\\':
			b.WriteByte(inner[i])
		default:
			b.WriteByte(inner[i])
		}
	}
	return b.String()
}

// parseTOMLStringArray parses single-line arrays of basic strings, e.g.
// `args = []` or `args = ["--flag"]`. Multi-line arrays are reported as
// unknown so classification treats them conservatively.
func parseTOMLStringArray(value string) ([]string, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return nil, false
	}
	inner := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if inner == "" {
		return []string{}, true
	}
	var out []string
	for _, part := range strings.Split(inner, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if len(part) < 2 || (part[0] != '"' && part[0] != '\'') {
			return nil, false
		}
		out = append(out, unquoteTOMLString(part))
	}
	return out, true
}

// parseTOMLInlineStringTable parses `env = { KEY = "VALUE", ... }`.
func parseTOMLInlineStringTable(value string) (map[string]string, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return nil, false
	}
	inner := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	env := map[string]string{}
	if inner == "" {
		return env, true
	}
	for _, part := range strings.Split(inner, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, raw, ok := strings.Cut(part, "=")
		if !ok {
			return nil, false
		}
		key = strings.TrimSpace(key)
		raw = strings.TrimSpace(raw)
		if key == "" || len(raw) < 2 || (raw[0] != '"' && raw[0] != '\'') {
			return nil, false
		}
		env[unquoteTOMLString(key)] = unquoteTOMLString(raw)
	}
	return env, true
}

// --- Claude Code (~/.claude.json) ---

// classifyClaudeConfig reads the lemonssh-external entry from the Claude
// user-scope config JSON.
func classifyClaudeConfig(content []byte, probe externalClientProbe) (state string, existingCommand string, err error) {
	doc, err := decodeClaudeConfig(content)
	if err != nil {
		return "", "", err
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	raw, found := servers[externalMcpServerName]
	if !found {
		return externalClientStateNotConfigured, "", nil
	}
	entry, ok := raw.(map[string]any)
	if !ok {
		return externalClientStateConflict, "", nil
	}
	enabled := true
	if v, ok := entry["enabled"].(bool); ok {
		enabled = v
	}
	command, _ := entry["command"].(string)
	var args []string
	argsKnown := false
	switch parsed := entry["args"].(type) {
	case []any:
		argsKnown = true
		for _, a := range parsed {
			if s, ok := a.(string); ok {
				args = append(args, s)
			}
		}
	case nil:
		argsKnown = true
	}
	env, envKnown := map[string]string{}, false
	switch parsed := entry["env"].(type) {
	case map[string]any:
		envKnown = true
		for k, v := range parsed {
			if s, ok := v.(string); ok {
				env[k] = s
			}
		}
	case nil:
		envKnown = true
	}
	existingCommand = strings.TrimSpace(strings.Join(append([]string{command}, args...), " "))
	return classifyExternalEntryMatch(command, args, argsKnown, env, envKnown, enabled, probe), existingCommand, nil
}

func decodeClaudeConfig(content []byte) (map[string]any, error) {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return map[string]any{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("claude config is not valid JSON: %w", err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, nil
}

// mergeClaudeMcpServerEntry re-serializes the Claude config with only the
// lemonssh-external entry replaced; every other key (and integer literals,
// via UseNumber) survives the round trip.
func mergeClaudeMcpServerEntry(original []byte, probe externalClientProbe) ([]byte, error) {
	doc, err := decodeClaudeConfig(original)
	if err != nil {
		return nil, err
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	entry := map[string]any{
		"command": probe.launcherPath,
		"args":    []any{},
		"type":    "stdio",
	}
	if probe.discoveryPath != "" {
		entry["env"] = map[string]any{externalMcpDiscoveryEnvVar: probe.discoveryPath}
	}
	servers[externalMcpServerName] = entry
	doc["mcpServers"] = servers
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// --- AgentHost glue ---

// externalLauncherPath resolves the LemonSSH-mcp launcher shipped next to the
// app executable; the same resolution externalStatus reports.
func (h *AgentHost) externalLauncherPath() string {
	exe, _ := os.Executable()
	name := "LemonSSH-mcp"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	launcher := filepath.Join(filepath.Dir(exe), name)
	if _, err := os.Stat(launcher); err != nil {
		if cwd, err := os.Getwd(); err == nil {
			for _, dir := range []string{"bin", filepath.Join("dist", "wails")} {
				candidate := filepath.Join(cwd, dir, name)
				if _, err := os.Stat(candidate); err == nil {
					return candidate
				}
			}
		}
	}
	return launcher
}

// externalDiscoveryPathForClients is the deterministic discovery file the
// client entries reference. It matches setExternalEnabled's e.path so a
// written entry survives enable/disable cycles and host restarts.
func (h *AgentHost) externalDiscoveryPathForClients() string {
	if h.discoveryPath != "" {
		return filepath.Join(filepath.Dir(h.discoveryPath), "external-mcp-discovery.json")
	}
	return filepath.Join(baseProfileDir(), "external-mcp-discovery.json")
}

func (s *AgentService) agentExternalClientStatus(client externalClientKind) (map[string]any, error) {
	if s.host == nil {
		return nil, fmt.Errorf("agent host is unavailable")
	}
	probe, err := newExternalClientProbe(client, s.host.externalLauncherPath(), s.host.externalDiscoveryPathForClients())
	if err != nil {
		return nil, err
	}
	return classifyExternalClientStatus(probe).toMap(), nil
}

func (s *AgentService) agentExternalClientAdd(client externalClientKind) (map[string]any, error) {
	if s.host == nil {
		return nil, fmt.Errorf("agent host is unavailable")
	}
	probe, err := newExternalClientProbe(client, s.host.externalLauncherPath(), s.host.externalDiscoveryPathForClients())
	if err != nil {
		return nil, err
	}
	return addExternalClientEntry(probe).toMap(), nil
}

// AgentExternalMcpCodexGetStatus probes ~/.codex/config.toml (or $CODEX_HOME)
// for the lemonssh-external MCP server entry.
func (s *AgentService) AgentExternalMcpCodexGetStatus() (map[string]any, error) {
	return s.agentExternalClientStatus(externalClientCodex)
}

// AgentExternalMcpCodexAdd merges the entry into codex config.toml.
func (s *AgentService) AgentExternalMcpCodexAdd() (map[string]any, error) {
	return s.agentExternalClientAdd(externalClientCodex)
}

// AgentExternalMcpClaudeGetStatus probes ~/.claude.json for the entry.
func (s *AgentService) AgentExternalMcpClaudeGetStatus() (map[string]any, error) {
	return s.agentExternalClientStatus(externalClientClaude)
}

// AgentExternalMcpClaudeAdd merges the entry into the Claude user-scope
// config (~/.claude.json mcpServers).
func (s *AgentService) AgentExternalMcpClaudeAdd() (map[string]any, error) {
	return s.agentExternalClientAdd(externalClientClaude)
}

// AgentExternalMcpGrokGetStatus probes ~/.grok/config.toml (or
// $GROK_CONFIG_DIR) for the entry.
func (s *AgentService) AgentExternalMcpGrokGetStatus() (map[string]any, error) {
	return s.agentExternalClientStatus(externalClientGrok)
}

// AgentExternalMcpGrokAdd merges the entry into the Grok CLI config.
func (s *AgentService) AgentExternalMcpGrokAdd() (map[string]any, error) {
	return s.agentExternalClientAdd(externalClientGrok)
}
