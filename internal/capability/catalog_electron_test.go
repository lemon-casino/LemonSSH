package capability

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixturePolicy struct {
	Write                 bool `json:"write"`
	SensitiveRead         bool `json:"sensitiveRead"`
	LongRunning           bool `json:"longRunning"`
	RequiresChatSession   bool `json:"requiresChatSession"`
	BypassesObserverBlock bool `json:"bypassesObserverBlock"`
	BypassesApproval      bool `json:"bypassesApproval"`
	BypassesChatCancel    bool `json:"bypassesChatCancel"`
}

type fixtureBinding struct {
	RPCMethod            *string  `json:"rpcMethod"`
	MCPTool              *string  `json:"mcpTool"`
	ToolName             *string  `json:"toolName"`
	Command              []string `json:"command"`
	ConfirmInConfirmMode *bool    `json:"confirmInConfirmMode"`
}

type fixtureCapability struct {
	ID          string                    `json:"id"`
	Domain      string                    `json:"domain"`
	Status      string                    `json:"status"`
	Description string                    `json:"description"`
	Policy      fixturePolicy             `json:"policy"`
	Surfaces    map[string]fixtureBinding `json:"surfaces"`
	AgentKinds  []string                  `json:"agentKinds"`
}

type fixtureCatalog struct {
	Capabilities []fixtureCapability `json:"capabilities"`
}

func loadElectronFixture(t *testing.T) fixtureCatalog {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ai", "catalog", "electron-catalog.json"))
	if err != nil {
		t.Fatalf("read electron catalog fixture: %v (regenerate with: node scripts/dump-capability-catalog.cjs)", err)
	}
	var parsed fixtureCatalog
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse electron catalog fixture: %v", err)
	}
	return parsed
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// fixtureString normalizes an absent fixture field to the empty string used
// by the Go SurfaceBinding representation.
func fixtureString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func commandString(parts []string) string {
	return strings.Join(parts, " ")
}

func compareBinding(t *testing.T, id, surface string, got SurfaceBinding, want fixtureBinding) {
	t.Helper()
	if got.RPCMethod != fixtureString(want.RPCMethod) {
		t.Errorf("%s/%s rpcMethod: got %q want %q", id, surface, got.RPCMethod, fixtureString(want.RPCMethod))
	}
	if got.MCPTool != fixtureString(want.MCPTool) {
		t.Errorf("%s/%s mcpTool: got %q want %q", id, surface, got.MCPTool, fixtureString(want.MCPTool))
	}
	if got.ToolName != fixtureString(want.ToolName) {
		t.Errorf("%s/%s toolName: got %q want %q", id, surface, got.ToolName, fixtureString(want.ToolName))
	}
	if commandString(got.Command) != commandString(want.Command) {
		t.Errorf("%s/%s command: got %q want %q", id, surface, commandString(got.Command), commandString(want.Command))
	}
	if (got.ConfirmInConfirmMode == nil) != (want.ConfirmInConfirmMode == nil) {
		t.Errorf("%s/%s confirmInConfirmMode presence: got %v want %v", id, surface, got.ConfirmInConfirmMode != nil, want.ConfirmInConfirmMode != nil)
	} else if got.ConfirmInConfirmMode != nil && *got.ConfirmInConfirmMode != *want.ConfirmInConfirmMode {
		t.Errorf("%s/%s confirmInConfirmMode: got %s want %s", id, surface, boolString(*got.ConfirmInConfirmMode), boolString(*want.ConfirmInConfirmMode))
	}
}

// TestCatalogMatchesElectronAuthority pins the Go catalog per ID against the
// fixture dumped from electron/capabilities/catalog, including row order
// (registry lookups resolve same-surface duplicate methods last-wins).
func TestCatalogMatchesElectronAuthority(t *testing.T) {
	fixture := loadElectronFixture(t)
	if len(fixture.Capabilities) != len(Catalog) {
		t.Fatalf("catalog size drift: Go has %d rows, electron fixture has %d", len(Catalog), len(fixture.Capabilities))
	}
	for i, want := range fixture.Capabilities {
		got := Catalog[i]
		label := fmt.Sprintf("row %d", i)
		if got.ID != want.ID {
			t.Fatalf("%s: Go row %q but fixture row %q (order is load-bearing)", label, got.ID, want.ID)
		}
		label = got.ID
		if got.Domain != want.Domain {
			t.Errorf("%s domain: got %q want %q", label, got.Domain, want.Domain)
		}
		if string(got.Status) != want.Status {
			t.Errorf("%s status: got %q want %q", label, got.Status, want.Status)
		}
		if got.Description != want.Description {
			t.Errorf("%s description: got %q want %q", label, got.Description, want.Description)
		}
		gotPolicy := got.Policy
		wantPolicy := want.Policy
		if gotPolicy != Policy(wantPolicy) {
			t.Errorf("%s policy: got %+v want %+v", label, gotPolicy, wantPolicy)
		}
		if len(got.Surfaces) != len(want.Surfaces) {
			t.Errorf("%s surface count: got %d want %d", label, len(got.Surfaces), len(want.Surfaces))
		}
		for surfaceName, wantBinding := range want.Surfaces {
			gotBinding, ok := got.Surfaces[Surface(surfaceName)]
			if !ok {
				t.Errorf("%s missing surface %q", label, surfaceName)
				continue
			}
			compareBinding(t, label, surfaceName, gotBinding, wantBinding)
		}
		if want.AgentKinds == nil && len(got.AgentKinds) != 0 {
			t.Errorf("%s agentKinds: got %v want none", label, got.AgentKinds)
		}
		if want.AgentKinds != nil {
			if len(got.AgentKinds) != len(want.AgentKinds) {
				t.Errorf("%s agentKinds count: got %v want %v", label, got.AgentKinds, want.AgentKinds)
				continue
			}
			for j, kind := range want.AgentKinds {
				if string(got.AgentKinds[j]) != kind {
					t.Errorf("%s agentKinds[%d]: got %q want %q", label, j, got.AgentKinds[j], kind)
				}
			}
		}
	}
}

// TestCatalogIntegrity ports the CJS catalog integrity guards: unique IDs,
// implemented rows expose at least one executable binding.
func TestCatalogIntegrity(t *testing.T) {
	seen := make(map[string]bool, len(Catalog))
	for i := range Catalog {
		def := &Catalog[i]
		if seen[def.ID] {
			t.Errorf("duplicate capability id %q", def.ID)
		}
		seen[def.ID] = true

		if def.Status != StatusImplemented {
			continue
		}
		if len(def.Surfaces) == 0 {
			t.Errorf("%s has no surfaces", def.ID)
			continue
		}
		executable := false
		for _, binding := range def.Surfaces {
			if binding.RPCMethod != "" || len(binding.Command) > 0 || binding.ToolName != "" || binding.MCPTool != "" {
				executable = true
				break
			}
		}
		if !executable {
			t.Errorf("%s has no rpc/cli/catty/mcp binding", def.ID)
		}
	}
}

// TestRegistryLookupSemantics characterizes CJS registry behavior including
// same-surface duplicate methods resolving to the later catalog row.
func TestRegistryLookupSemantics(t *testing.T) {
	registry := Default()

	if got := len(registry.List(ListOptions{})); got != len(Catalog) {
		t.Fatalf("registry size: got %d want %d", got, len(Catalog))
	}

	// netcatty/getContext is bound by session.environment first and session.get
	// later; the CJS registry resolves it to the later row.
	if def := registry.GetByRPCMethod("netcatty/getContext", SurfaceBuiltin); def == nil || def.ID != "session.get" {
		t.Errorf("builtin netcatty/getContext resolved to %v, want session.get", def)
	}
	// netcatty/setCancelled is bound by session.cancel then session.resume.
	if def := registry.GetByRPCMethod("netcatty/setCancelled", SurfaceBuiltin); def == nil || def.ID != "session.resume" {
		t.Errorf("builtin netcatty/setCancelled resolved to %v, want session.resume", def)
	}

	if def := registry.GetByRPCMethod("netcatty/sftp/list", SurfaceBuiltin); def == nil || def.ID != "sftp.list" {
		t.Errorf("builtin netcatty/sftp/list resolved to %v, want sftp.list", def)
	}
	if def := registry.GetByRPCMethod("public/sftp/list", SurfacePublic); def == nil || def.ID != "sftp.list" {
		t.Errorf("public public/sftp/list resolved to %v, want sftp.list", def)
	}
	if def := registry.GetByRPCMethod("netcatty/sftp/list", SurfacePublic); def != nil {
		t.Errorf("public surface must not see builtin methods, got %v", def)
	}
	if def := registry.GetByRPCMethod("vault/hosts/open", SurfaceGlobal); def == nil || def.ID != "vault.host.open" {
		t.Errorf("global vault/hosts/open resolved to %v, want vault.host.open", def)
	}
	if def := registry.GetByCLICommand([]string{"sftp", "list"}); def == nil || def.ID != "sftp.list" {
		t.Errorf("cli sftp list resolved to %v, want sftp.list", def)
	}
	if def := registry.GetByToolName("tool_output_read"); def == nil || def.ID != "harness.tool_output.read" {
		t.Errorf("tool_output_read resolved to %v, want harness.tool_output.read", def)
	}
	if def := registry.GetByID("vault.host.open"); def == nil || len(def.AgentKinds) != 1 || def.AgentKinds[0] != AgentKindGlobal {
		t.Errorf("vault.host.open agentKinds: got %v, want [global]", def)
	}

	// Legacy write set from policy.test.cjs (builtin surface).
	legacyWrite := []string{
		"netcatty/exec", "netcatty/sftp/write", "netcatty/sftp/download", "netcatty/sftp/upload",
		"netcatty/sftp/mkdir", "netcatty/sftp/delete", "netcatty/sftp/rename", "netcatty/sftp/chmod",
		"netcatty/jobStart", "netcatty/jobStop",
	}
	writeMethods := make(map[string]bool)
	for _, method := range registry.RPCMethodsForSurface(SurfaceBuiltin, RPCFilter{Status: StatusImplemented, Write: true}) {
		writeMethods[method] = true
	}
	if len(writeMethods) != len(legacyWrite) {
		t.Errorf("builtin write set size: got %d want %d (%v)", len(writeMethods), len(legacyWrite), writeMethods)
	}
	for _, method := range legacyWrite {
		if !writeMethods[method] {
			t.Errorf("builtin write set missing %q", method)
		}
	}
}
