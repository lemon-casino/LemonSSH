package capability

import "testing"

// TestResolveAgentKinds pins the placement rules used by generated agent tools.
func TestResolveAgentKinds(t *testing.T) {
	registry := Default()

	cases := []struct {
		id   string
		want []AgentKind
	}{
		{"vault.host.open", []AgentKind{AgentKindGlobal}},
		{"harness.tool_output.read", []AgentKind{AgentKindSidebar}},
		{"terminal.execute", []AgentKind{AgentKindSidebar, AgentKindGlobal}},
		{"meta.status", nil},
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
