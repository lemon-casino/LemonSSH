package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempExternalClientProbe(t *testing.T, client externalClientKind, configName string) externalClientProbe {
	t.Helper()
	dir := t.TempDir()
	return externalClientProbe{
		client:        client,
		configPath:    filepath.Join(dir, configName),
		launcherPath:  filepath.Join(dir, "bin", "LemonSSH-mcp.exe"),
		discoveryPath: filepath.Join(dir, "profile", "external-mcp-discovery.json"),
	}
}

func writeFileForTest(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// The launcher path used by tempExternalClientProbe, TOML-quoted.
func probeLauncherTOML(t *testing.T, probe externalClientProbe) string {
	t.Helper()
	return "command = " + quoteTOMLString(probe.launcherPath)
}

func TestExternalClientStatusCodexTOML(t *testing.T) {
	probe := tempExternalClientProbe(t, externalClientCodex, filepath.Join(".codex", "config.toml"))

	status := classifyExternalClientStatus(probe)
	if status.State != "codex_not_found" {
		t.Fatalf("missing config: want codex_not_found, got %q", status.State)
	}

	writeFileForTest(t, probe.configPath, "[other]\nkey = 1\n")
	if status = classifyExternalClientStatus(probe); status.State != externalClientStateNotConfigured {
		t.Fatalf("config without entry: want not_configured, got %q", status.State)
	}

	configured := "[mcp_servers.other-server]\ncommand = \"other\"\n\n" +
		"[mcp_servers.lemonssh-external]\n" +
		probeLauncherTOML(t, probe) + "\nargs = []\n" +
		"env = { " + externalMcpDiscoveryEnvVar + " = " + quoteTOMLString(probe.discoveryPath) + " }\n"
	writeFileForTest(t, probe.configPath, configured)
	if status = classifyExternalClientStatus(probe); status.State != externalClientStateConfigured {
		t.Fatalf("matching entry: want configured, got %q (error=%q)", status.State, status.Error)
	}

	foreign := "[mcp_servers.lemonssh-external]\ncommand = \"C:\\other\\tool.exe\"\nargs = []\n"
	writeFileForTest(t, probe.configPath, foreign)
	if status = classifyExternalClientStatus(probe); status.State != externalClientStateConflict {
		t.Fatalf("foreign command: want conflict, got %q", status.State)
	}
	if status.ExistingCommand == "" {
		t.Fatalf("conflict should report the existing command")
	}

	staleEnv := "[mcp_servers.lemonssh-external]\n" + probeLauncherTOML(t, probe) + "\nargs = []\n"
	writeFileForTest(t, probe.configPath, staleEnv)
	if status = classifyExternalClientStatus(probe); status.State != externalClientStateNotConfigured {
		t.Fatalf("stale env: want not_configured, got %q", status.State)
	}

	extraArgs := "[mcp_servers.lemonssh-external]\n" + probeLauncherTOML(t, probe) + "\nargs = [\"--verbose\"]\n"
	writeFileForTest(t, probe.configPath, extraArgs)
	if status = classifyExternalClientStatus(probe); status.State != externalClientStateConflict {
		t.Fatalf("extra args: want conflict, got %q", status.State)
	}
}

func TestExternalClientStatusGrokTOML(t *testing.T) {
	probe := tempExternalClientProbe(t, externalClientGrok, filepath.Join(".grok", "config.toml"))
	if status := classifyExternalClientStatus(probe); status.State != "grok_not_found" {
		t.Fatalf("missing config: want grok_not_found, got %q", status.State)
	}
	writeFileForTest(t, probe.configPath, "# grok config\nmodel = \"grok-4\"\n")
	if status := classifyExternalClientStatus(probe); status.State != externalClientStateNotConfigured {
		t.Fatalf("config without entry: want not_configured, got %q", status.State)
	}
}

func TestExternalClientStatusClaudeJSON(t *testing.T) {
	probe := tempExternalClientProbe(t, externalClientClaude, ".claude.json")
	if status := classifyExternalClientStatus(probe); status.State != "claude_not_found" {
		t.Fatalf("missing config: want claude_not_found, got %q", status.State)
	}

	writeFileForTest(t, probe.configPath, `{"num": 7, "projects": {"x": {"allowed": true}}}`)
	if status := classifyExternalClientStatus(probe); status.State != externalClientStateNotConfigured {
		t.Fatalf("no mcpServers key: want not_configured, got %q", status.State)
	}

	entry := `{"command": ` + jsonQuote(t, probe.launcherPath) + `, "args": [], "env": {"` +
		externalMcpDiscoveryEnvVar + `": ` + jsonQuote(t, probe.discoveryPath) + `}}`
	writeFileForTest(t, probe.configPath, `{"mcpServers": {"lemonssh-external": `+entry+`}}`)
	if status := classifyExternalClientStatus(probe); status.State != externalClientStateConfigured {
		t.Fatalf("matching entry: want configured, got %q", status.State)
	}

	writeFileForTest(t, probe.configPath, `{"mcpServers": {"lemonssh-external": {"command": "C:\\other\\tool.exe", "args": []}}}`)
	if status := classifyExternalClientStatus(probe); status.State != externalClientStateConflict {
		t.Fatalf("foreign command: want conflict, got %q", status.State)
	}

	writeFileForTest(t, probe.configPath, `{not json}`)
	if status := classifyExternalClientStatus(probe); status.State != externalClientStateError || status.Error == "" {
		t.Fatalf("invalid JSON: want error with message, got %q (%q)", status.State, status.Error)
	}
}

func jsonQuote(t *testing.T, value string) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %q: %v", value, err)
	}
	return string(raw)
}

func TestAddExternalClientEntryMergesTOMLAndLeavesBackup(t *testing.T) {
	probe := tempExternalClientProbe(t, externalClientCodex, filepath.Join(".codex", "config.toml"))
	original := "[profile]\ndefault = \"main\"\n\n" +
		"[mcp_servers.github]\ncommand = \"github-mcp\"\nargs = [\"--remote\"]\n\n" +
		"[mcp_servers.lemonssh-external]\n" + probeLauncherTOML(t, probe) + "\nargs = []\n"
	writeFileForTest(t, probe.configPath, original)

	status := addExternalClientEntry(probe)
	// The fixture entry has a matching command but a stale env, so Add must
	// rewrite it and converge to configured.
	if status.State != externalClientStateConfigured {
		t.Fatalf("re-add with stale env: want configured, got %q (%q)", status.State, status.Error)
	}
	merged, err := os.ReadFile(probe.configPath)
	if err != nil {
		t.Fatalf("read merged config: %v", err)
	}
	text := string(merged)
	for _, want := range []string{
		"[profile]",
		"[mcp_servers.github]",
		"args = [\"--remote\"]",
		"[mcp_servers.lemonssh-external]",
		"env = { " + externalMcpDiscoveryEnvVar + " = " + quoteTOMLString(probe.discoveryPath) + " }",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("merged config missing %q:\n%s", want, text)
		}
	}
	if got := strings.Count(text, "[mcp_servers.lemonssh-external]"); got != 1 {
		t.Fatalf("merged config has %d lemonssh-external sections, want 1:\n%s", got, text)
	}
	if strings.Count(text, probeLauncherTOML(t, probe)) != 1 {
		t.Fatalf("merged config duplicated the launcher command:\n%s", text)
	}

	backup, err := os.ReadFile(probe.configPath + ".bak")
	if err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if string(backup) != original {
		t.Fatalf("backup content mismatch:\n--- original\n%s\n--- backup\n%s", original, string(backup))
	}

	// Second add converges to configured without duplicating.
	status = addExternalClientEntry(probe)
	if status.State != externalClientStateConfigured {
		t.Fatalf("second add: want configured, got %q (%q)", status.State, status.Error)
	}
	merged2, _ := os.ReadFile(probe.configPath)
	if got := strings.Count(string(merged2), "[mcp_servers.lemonssh-external]"); got != 1 {
		t.Fatalf("second add duplicated the section:\n%s", string(merged2))
	}
}

func TestAddExternalClientEntryNeverOverwritesConflicts(t *testing.T) {
	probe := tempExternalClientProbe(t, externalClientGrok, filepath.Join(".grok", "config.toml"))
	original := "[mcp_servers.lemonssh-external]\ncommand = \"C:\\other\\tool.exe\"\nargs = []\n"
	writeFileForTest(t, probe.configPath, original)

	status := addExternalClientEntry(probe)
	if status.State != externalClientStateConflict {
		t.Fatalf("want conflict, got %q", status.State)
	}
	after, err := os.ReadFile(probe.configPath)
	if err != nil || string(after) != original {
		t.Fatalf("conflicting config must stay untouched: %v %q", err, string(after))
	}
	if _, err := os.Stat(probe.configPath + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("conflict path must not write a backup (stat err: %v)", err)
	}

	// Uninstalled clients are never created on disk by Add.
	missing := tempExternalClientProbe(t, externalClientCodex, filepath.Join(".codex", "config.toml"))
	if status := addExternalClientEntry(missing); status.State != "codex_not_found" {
		t.Fatalf("missing config: want codex_not_found, got %q", status.State)
	}
	if _, err := os.Stat(missing.configPath); !os.IsNotExist(err) {
		t.Fatalf("add must not create the config of an uninstalled client (stat err: %v)", err)
	}
}

func TestAddExternalClientEntryMergesClaudeJSON(t *testing.T) {
	probe := tempExternalClientProbe(t, externalClientClaude, ".claude.json")
	original := "{\n  \"userID\": 42,\n  \"projects\": {\"/work\": {\"allowed\": true}}\n}\n"
	writeFileForTest(t, probe.configPath, original)

	status := addExternalClientEntry(probe)
	if status.State != externalClientStateConfigured {
		t.Fatalf("want configured after add, got %q (%q)", status.State, status.Error)
	}

	content, err := os.ReadFile(probe.configPath)
	if err != nil {
		t.Fatalf("read merged config: %v", err)
	}
	var doc map[string]any
	dec := json.NewDecoder(strings.NewReader(string(content)))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("merged config is not valid JSON: %v\n%s", err, string(content))
	}
	if doc["userID"] != json.Number("42") {
		t.Fatalf("userID not preserved as integer: %#v", doc["userID"])
	}
	if _, ok := doc["projects"].(map[string]any); !ok {
		t.Fatalf("projects key lost: %#v", doc["projects"])
	}
	servers, ok := doc["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers missing: %#v", doc)
	}
	entry, ok := servers["lemonssh-external"].(map[string]any)
	if !ok || entry["command"] != probe.launcherPath {
		t.Fatalf("lemonssh-external entry wrong: %#v", servers)
	}
	env, ok := entry["env"].(map[string]any)
	if !ok || env[externalMcpDiscoveryEnvVar] != probe.discoveryPath {
		t.Fatalf("discovery env missing: %#v", entry)
	}

	backup, err := os.ReadFile(probe.configPath + ".bak")
	if err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	var backupDoc map[string]any
	if err := json.Unmarshal(backup, &backupDoc); err != nil {
		t.Fatalf("backup is not valid JSON: %v", err)
	}
	if _, ok := backupDoc["mcpServers"]; ok {
		t.Fatalf("backup must hold the original content without the entry: %s", string(backup))
	}
}

func TestExternalClientAddCommandFormatsMatchCard(t *testing.T) {
	probe := tempExternalClientProbe(t, externalClientCodex, "x")
	// Fixed fixtures keep every expectation identical on Windows and Linux:
	// formatExternalClientAddCommand is pure and its quoting mirrors
	// ExternalMcpCard's quoteShellArg (quote only when the value carries
	// whitespace, quotes or backslashes). The POSIX discovery path therefore
	// stays unquoted while the Windows launcher path is quoted and escaped.
	probe.launcherPath = `C:\Program Files\LemonSSH\LemonSSH-mcp.exe`
	probe.discoveryPath = "/opt/lemonssh/profile/external-mcp-discovery.json"
	if got := formatExternalClientAddCommand(probe); got !=
		`codex mcp add lemonssh-external --env LEMONSSH_EXTERNAL_MCP_DISCOVERY_FILE=/opt/lemonssh/profile/external-mcp-discovery.json -- "C:\\Program Files\\LemonSSH\\LemonSSH-mcp.exe"` {
		t.Fatalf("codex command mismatch: %q", got)
	}
	probe.client = externalClientClaude
	if got := formatExternalClientAddCommand(probe); got !=
		`claude mcp add -s user lemonssh-external -e LEMONSSH_EXTERNAL_MCP_DISCOVERY_FILE=/opt/lemonssh/profile/external-mcp-discovery.json -- "C:\\Program Files\\LemonSSH\\LemonSSH-mcp.exe"` {
		t.Fatalf("claude command mismatch: %q", got)
	}
	probe.client = externalClientGrok
	if got := formatExternalClientAddCommand(probe); got !=
		`grok mcp add lemonssh-external -e LEMONSSH_EXTERNAL_MCP_DISCOVERY_FILE=/opt/lemonssh/profile/external-mcp-discovery.json -- "C:\\Program Files\\LemonSSH\\LemonSSH-mcp.exe"` {
		t.Fatalf("grok command mismatch: %q", got)
	}

	// Paths with spaces and quotes stay shell-safe, matching quoteShellArg.
	probe.launcherPath = `C:\My Apps\Le"monSSH-mcp.exe`
	if got := formatExternalClientAddCommand(probe); !strings.HasSuffix(got, `-- "C:\\My Apps\\Le\"monSSH-mcp.exe"`) {
		t.Fatalf("quoting mismatch: %q", got)
	}

	// Windows-style discovery paths carry backslashes and are quoted exactly
	// like the card renders them on Windows.
	probe.discoveryPath = `C:\Program Files\LemonSSH\external-mcp-discovery.json`
	if got := formatExternalClientAddCommand(probe); !strings.Contains(got, `-e LEMONSSH_EXTERNAL_MCP_DISCOVERY_FILE="C:\\Program Files\\LemonSSH\\external-mcp-discovery.json"`) {
		t.Fatalf("quoted env pair mismatch: %q", got)
	}

	// No discovery path known: env flags are omitted.
	probe.discoveryPath = ""
	probe.launcherPath = "/opt/lemonssh/LemonSSH-mcp"
	if got := formatExternalClientAddCommand(probe); got != "grok mcp add lemonssh-external -- /opt/lemonssh/LemonSSH-mcp" {
		t.Fatalf("command without discovery mismatch: %q", got)
	}
}

// The AgentService surface resolves paths under the profile dir and reports
// typed not-found states without touching anything outside the temp dir.
func TestAgentServiceExternalClientMethodsResolveInsideProfileDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("NETCATTY_PROFILE_DIR", filepath.Join(tmp, "profile"))
	t.Setenv("CODEX_HOME", filepath.Join(tmp, "codex-home"))
	t.Setenv("GROK_CONFIG_DIR", filepath.Join(tmp, "grok-home"))
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("HOME", tmp)

	service := &AgentService{host: &AgentHost{}}
	for name, fn := range map[string]func() (map[string]any, error){
		"codex":  service.AgentExternalMcpCodexGetStatus,
		"claude": service.AgentExternalMcpClaudeGetStatus,
		"grok":   service.AgentExternalMcpGrokGetStatus,
	} {
		status, err := fn()
		if err != nil {
			t.Fatalf("%s status: %v", name, err)
		}
		if status["state"] != name+"_not_found" {
			t.Fatalf("%s status: want %s_not_found, got %v", name, name, status["state"])
		}
		configPath, _ := status["configPath"].(string)
		if configPath == "" || !strings.HasPrefix(configPath, tmp) {
			t.Fatalf("%s configPath escaped the temp dir: %q", name, configPath)
		}
	}

	// Nil host fails typed, like the other AgentExternal* methods.
	if _, err := (&AgentService{}).AgentExternalMcpCodexGetStatus(); err == nil {
		t.Fatalf("nil host must fail typed")
	}
}

func TestStripTomlMcpServerSectionRemovesNestedSubtables(t *testing.T) {
	original := "[a]\nk = 1\n\n" +
		"[mcp_servers.lemonssh-external]\ncommand = \"x\"\n\n" +
		"[mcp_servers.lemonssh-external.env]\nFOO = \"bar\"\n\n" +
		"[mcp_servers.other]\ncommand = \"y\"\n"
	stripped := stripTomlMcpServerSection(original, "lemonssh-external")
	for _, unwanted := range []string{"[mcp_servers.lemonssh-external]", "FOO = \"bar\""} {
		if strings.Contains(stripped, unwanted) {
			t.Fatalf("stripped content still contains %q:\n%s", unwanted, stripped)
		}
	}
	for _, want := range []string{"[a]", "k = 1", "[mcp_servers.other]", "command = \"y\""} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("stripped content lost %q:\n%s", want, stripped)
		}
	}
}
