package manifest

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDeclarativeUIValidatedAtManifestBoundary(t *testing.T) {
	raw := `{"apiVersion":2,"name":"example","version":"1.0.0","displayName":"Example","entrypoint":{"wasm":"main.wasm","sha256":"` + strings.Repeat("a", 64) + `"},"ui":{"settings":[{"id":"mode","type":"javascript","label":"Mode"}]}}`
	var m Manifest
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if err := Validate(m); err == nil {
		t.Fatal("unsafe UI schema accepted at install boundary")
	}
}

func TestUICommandsMustReferenceDeclaredContributions(t *testing.T) {
	uiBlock := `"ui":{"views":[{"id":"status","type":"card","title":"Status"}],"menus":[{"id":"palette","command":"run-job","location":"commandPalette"}],"keybindings":[{"command":"run-job","key":"ctrl+alt+r"}]}`
	good := `{"apiVersion":2,"name":"example","version":"1.0.0","displayName":"Example","entrypoint":{"wasm":"main.wasm","sha256":"` + strings.Repeat("a", 64) + `"},"contributions":[{"type":"command","id":"run-job"}],` + uiBlock + `}`
	var m Manifest
	if err := json.Unmarshal([]byte(good), &m); err != nil {
		t.Fatal(err)
	}
	if err := Validate(m); err != nil {
		t.Fatalf("manifest with declared command references rejected: %v", err)
	}

	bad := `{"apiVersion":2,"name":"example","version":"1.0.0","displayName":"Example","entrypoint":{"wasm":"main.wasm","sha256":"` + strings.Repeat("a", 64) + `"},"contributions":[{"type":"setting","id":"mode"}],` + uiBlock + `}`
	var broken Manifest
	if err := json.Unmarshal([]byte(bad), &broken); err != nil {
		t.Fatal(err)
	}
	if err := Validate(broken); err == nil {
		t.Fatal("menu/keybinding referencing an undeclared command accepted")
	}
}
