package capability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func loadFixtureObject(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ai", "catalog", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v (regenerate with: node scripts/dump-capability-catalog.cjs)", name, err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	return parsed
}

func toJSONValue(t *testing.T, value any) any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal Go value: %v", err)
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("re-parse marshaled value: %v", err)
	}
	return parsed
}

// TestToolInputsMatchElectronAuthority pins the generated tool-input data
// against the CJS dump.
func TestToolInputsMatchElectronAuthority(t *testing.T) {
	fixture := loadFixtureObject(t, "electron-tool-inputs.json")
	wantFields := fixture["toolInputFields"]
	wantHints := fixture["modelDescriptionHints"]
	if got := toJSONValue(t, toolInputFields); !reflect.DeepEqual(got, wantFields) {
		t.Errorf("toolInputFields drifted from electron authority; rerun scripts/dump-capability-catalog.cjs")
	}
	if got := toJSONValue(t, modelDescriptionHints); !reflect.DeepEqual(got, wantHints) {
		t.Errorf("modelDescriptionHints drifted from electron authority; rerun scripts/dump-capability-catalog.cjs")
	}
}

// TestAgentToolSpecsMatchElectronAuthority pins the sidebar and global agent
// projections against the CJS listAgentToolSpecs output.
func TestAgentToolSpecsMatchElectronAuthority(t *testing.T) {
	cases := []struct {
		name     string
		kind     AgentKind
		fixture  string
		matchKey string
	}{
		{"sidebar", AgentKindSidebar, "electron-agent-specs-sidebar.json", "specs"},
		{"global", AgentKindGlobal, "electron-agent-specs-global.json", "specs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := loadFixtureObject(t, tc.fixture)
			wantSpecs := fixture[tc.matchKey].([]any)
			gotSpecs := toJSONValue(t, Default().ListAgentToolSpecs(tc.kind)).([]any)
			if len(gotSpecs) != len(wantSpecs) {
				t.Fatalf("spec count: Go %d, electron %d", len(gotSpecs), len(wantSpecs))
			}
			for i := range wantSpecs {
				wantID := wantSpecs[i].(map[string]any)["capabilityId"]
				gotID := gotSpecs[i].(map[string]any)["capabilityId"]
				if wantID != gotID {
					t.Fatalf("spec order drift at %d: Go %v, electron %v (order is load-bearing)", i, gotID, wantID)
				}
				if !reflect.DeepEqual(gotSpecs[i], wantSpecs[i]) {
					gotRaw, _ := json.Marshal(gotSpecs[i])
					wantRaw, _ := json.Marshal(wantSpecs[i])
					t.Errorf("spec %v drifted:\n got: %s\nwant: %s", wantID, gotRaw, wantRaw)
				}
			}
		})
	}
}

// TestMcpToolsMatchElectronAuthority pins the MCP tool list projection.
func TestMcpToolsMatchElectronAuthority(t *testing.T) {
	fixture := loadFixtureObject(t, "electron-mcp-tools.json")
	wantTools := fixture["tools"].([]any)
	gotTools := toJSONValue(t, Default().ListMcpTools()).([]any)
	if len(gotTools) != len(wantTools) {
		t.Fatalf("mcp tool count: Go %d, electron %d", len(gotTools), len(wantTools))
	}
	for i := range wantTools {
		wantID := wantTools[i].(map[string]any)["capabilityId"]
		if !reflect.DeepEqual(gotTools[i], wantTools[i]) {
			gotRaw, _ := json.Marshal(gotTools[i])
			wantRaw, _ := json.Marshal(wantTools[i])
			t.Errorf("mcp tool %v drifted:\n got: %s\nwant: %s", wantID, gotRaw, wantRaw)
		}
	}
}

// TestResolveAgentKinds pins the placement rules from toolSurfaces.cjs.
func TestResolveAgentKinds(t *testing.T) {
	registry := Default()

	cases := []struct {
		id   string
		want []AgentKind
	}{
		{"vault.host.open", []AgentKind{AgentKindGlobal}},                    // explicit placement
		{"harness.tool_output.read", []AgentKind{AgentKindSidebar}},          // sidebar-local only
		{"terminal.execute", []AgentKind{AgentKindSidebar, AgentKindGlobal}}, // shared RPC
		{"meta.status", nil}, // denylist
	}
	for _, tc := range cases {
		def := registry.GetByID(tc.id)
		if def == nil {
			t.Fatalf("missing capability %s", tc.id)
		}
		got := def.ResolveAgentKinds()
		if len(got) != len(tc.want) {
			t.Errorf("%s agent kinds: got %v want %v", tc.id, got, tc.want)
			continue
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("%s agent kinds: got %v want %v", tc.id, got, tc.want)
			}
		}
	}
}
