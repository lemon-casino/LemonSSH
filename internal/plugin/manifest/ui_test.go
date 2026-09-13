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
