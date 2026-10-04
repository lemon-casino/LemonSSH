package host

import (
	"encoding/json"
	"github.com/lemon-casino/lemonssh/internal/plugin/permissions"
	"github.com/lemon-casino/lemonssh/internal/plugin/store"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsValidatePersistAndRejectUndeclared(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.json")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"apiVersion":2,"name":"example","version":"1.0.0","displayName":"Example","entrypoint":{"wasm":"main.wasm","sha256":"` + strings.Repeat("a", 64) + `"},"ui":{"settings":[{"id":"mode","type":"select","label":"Mode","options":["dark","light"]},{"id":"token","type":"password","label":"Token"}]}}`)
	_, err = s.Install("example", "1.0.0", strings.Repeat("a", 64), raw)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetState("example", store.StateEnabled); err != nil {
		t.Fatal(err)
	}
	h := Host{Store: s, Broker: permissions.NewBroker(nil)}
	if err := h.SetSetting("example", "mode", `"dark"`); err != nil {
		t.Fatal(err)
	}
	if err := h.SetSetting("example", "mode", `"other"`); err == nil {
		t.Fatal("invalid select accepted")
	}
	if err := h.SetSetting("example", "missing", `true`); err == nil {
		t.Fatal("undeclared setting accepted")
	}
	if err := h.SetSetting("example", "token", `"secret"`); err == nil {
		t.Fatal("secret written to plaintext inventory")
	}
	restored, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	values, err := (Host{Store: restored}).Settings("example")
	if err != nil || values["mode"] != "dark" {
		t.Fatal("setting not durable", values, err)
	}
}

func TestUIContributionsAggregatesEnabledPluginsOnly(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "plugins.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifestWithUI := `{"apiVersion":2,"name":"demo","version":"1.0.0","displayName":"Demo","entrypoint":{"wasm":"main.wasm","sha256":"` + strings.Repeat("a", 64) + `"},"contributions":[{"type":"command","id":"demo.run"}],"ui":{"views":[{"id":"status","type":"card","title":"Status","bindings":["demo.greeting"]},{"id":"ghost","type":"card","title":"Ghost","visible":false}],"menus":[{"id":"palette","command":"demo.run","location":"commandPalette","title":"Run"}],"keybindings":[{"command":"demo.run","key":"ctrl+alt+d","args":{"n":1}}]}}`
	if _, err := s.Install("demo", "1.0.0", strings.Repeat("a", 64), json.RawMessage(manifestWithUI)); err != nil {
		t.Fatalf("manifest with UI contributions rejected: %v", err)
	}
	plain := `{"apiVersion":2,"name":"plain","version":"1.0.0","displayName":"Plain","entrypoint":{"wasm":"main.wasm","sha256":"` + strings.Repeat("b", 64) + `"}}`
	if _, err := s.Install("plain", "1.0.0", strings.Repeat("b", 64), json.RawMessage(plain)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetState("plain", store.StateEnabled); err != nil {
		t.Fatal(err)
	}

	h := Host{Store: s, Broker: permissions.NewBroker(nil)}
	// "demo" is still disabled here: only "plain" (no UI block) contributes.
	collected, err := h.UIContributions()
	if err != nil {
		t.Fatal(err)
	}
	if len(collected.Views) != 0 || len(collected.Menus) != 0 || len(collected.Keybindings) != 0 {
		t.Fatalf("disabled plugin contributed UI: %+v", collected)
	}

	if err := s.SetState("demo", store.StateEnabled); err != nil {
		t.Fatal(err)
	}
	collected, err = h.UIContributions()
	if err != nil {
		t.Fatal(err)
	}
	if len(collected.Views) != 2 {
		t.Fatalf("expected 2 views, got %+v", collected.Views)
	}
	status := collected.Views[0]
	if status.PluginID != "demo" || status.ID != "status" || status.Location != "settings" || !status.Visible || len(status.Bindings) != 1 {
		t.Fatalf("view contribution defaults not applied: %+v", status)
	}
	if collected.Views[1].Visible {
		t.Fatal("hidden view surfaced as visible")
	}
	if len(collected.Menus) != 1 || collected.Menus[0].ID != "palette" || collected.Menus[0].Title != "Run" {
		t.Fatalf("menu contributions wrong: %+v", collected.Menus)
	}
	if len(collected.Keybindings) != 1 || collected.Keybindings[0].Command != "demo.run" || !collected.Keybindings[0].Enabled {
		t.Fatalf("keybinding contributions wrong: %+v", collected.Keybindings)
	}
	if string(collected.Keybindings[0].Args) != `{"n":1}` {
		t.Fatalf("keybinding args lost: %s", collected.Keybindings[0].Args)
	}
}

// A manifest whose ui block references an undeclared command must not
// contribute anything: the plugin is skipped wholesale (fail closed) instead
// of surfacing dead menu or keybinding entries.
func TestUIContributionsSkipPluginWithUndeclaredCommandReferences(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "plugins.json"))
	if err != nil {
		t.Fatal(err)
	}
	legacy := `{"apiVersion":2,"name":"legacy","version":"1.0.0","displayName":"Legacy","entrypoint":{"wasm":"main.wasm","sha256":"` + strings.Repeat("c", 64) + `"},"ui":{"menus":[{"id":"dead","command":"missing.command","location":"statusBar"}]}}`
	if _, err := s.Install("legacy", "1.0.0", strings.Repeat("c", 64), json.RawMessage(legacy)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetState("legacy", store.StateEnabled); err != nil {
		t.Fatal(err)
	}
	collected, err := (Host{Store: s, Broker: permissions.NewBroker(nil)}).UIContributions()
	if err != nil {
		t.Fatal(err)
	}
	if len(collected.Menus) != 0 {
		t.Fatalf("plugin with undeclared command reference contributed: %+v", collected.Menus)
	}
}
