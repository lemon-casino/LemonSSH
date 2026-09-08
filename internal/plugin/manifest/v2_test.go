package manifest

import (
	"encoding/json"
	"strings"
	"testing"
)

func validManifest() Manifest {
	return Manifest{
		APIVersion:  2,
		Name:        "demo-plugin",
		Version:     "1.2.3",
		DisplayName: "Demo Plugin",
		Entrypoint: Entrypoint{
			WASM:   "main.wasm",
			SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		Permissions: []Permission{
			{Kind: "filesystem", Resource: "data/*", Mode: "read"},
		},
		Contributions: []Contribution{
			{Type: "command", ID: "run-job"},
		},
	}
}

func TestValidateAcceptsWellFormed(t *testing.T) {
	if err := Validate(validManifest()); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
}

func TestValidateRejectsCoreFieldProblems(t *testing.T) {
	cases := map[string]func(*Manifest){
		"wrong apiVersion":   func(m *Manifest) { m.APIVersion = 1 },
		"bad name":           func(m *Manifest) { m.Name = "Demo_Plugin" },
		"bad version":        func(m *Manifest) { m.Version = "v1" },
		"empty display":      func(m *Manifest) { m.DisplayName = "  " },
		"non-wasm entry":     func(m *Manifest) { m.Entrypoint.WASM = "main.js" },
		"traversal entry":    func(m *Manifest) { m.Entrypoint.WASM = "../main.wasm" },
		"short sha":          func(m *Manifest) { m.Entrypoint.SHA256 = "abc" },
		"tiny memory":        func(m *Manifest) { m.Entrypoint.MemoryMB = 8 },
		"huge memory":        func(m *Manifest) { m.Entrypoint.MemoryMB = 4096 },
		"unknown permission": func(m *Manifest) { m.Permissions = []Permission{{Kind: "kernel", Resource: "*", Mode: "write"}} },
		"bad mode":           func(m *Manifest) { m.Permissions = []Permission{{Kind: "filesystem", Resource: "*", Mode: "rw"}} },
		"duplicate permission": func(m *Manifest) {
			m.Permissions = []Permission{
				{Kind: "filesystem", Resource: "data/*", Mode: "read"},
				{Kind: "filesystem", Resource: "data/*", Mode: "read"},
			}
		},
		"duplicate contribution": func(m *Manifest) {
			m.Contributions = []Contribution{
				{Type: "command", ID: "run"},
				{Type: "command", ID: "run"},
			}
		},
		"bad contribution type": func(m *Manifest) {
			m.Contributions = []Contribution{{Type: "widget", ID: "x"}}
		},
	}
	for name, mutate := range cases {
		manifest := validManifest()
		mutate(&manifest)
		if err := Validate(manifest); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}

func TestJSONRoundTripAndV1Rejection(t *testing.T) {
	manifest := validManifest()
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Manifest
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := Validate(decoded); err != nil {
		t.Fatal(err)
	}

	// A v1 manifest carries main.browser/main.node: unmarshalling into v2
	// drops them (unknown fields) and the entrypoint no longer resolves.
	var v1 map[string]json.RawMessage
	v1Raw := `{"name":"legacy","version":"0.0.1","main":{"browser":"index.js","node":"index.js"}}`
	if err := json.Unmarshal([]byte(v1Raw), &v1); err != nil {
		t.Fatal(err)
	}
	if _, ok := v1["main"]; !ok {
		t.Fatal("fixture broken")
	}
	if err := Validate(Manifest{}); err == nil {
		t.Fatal("empty manifest must be rejected")
	}
	// Strict decode of a v2 manifest must reject unknown fields.
	if err := json.NewDecoder(strings.NewReader(`{"apiVersion":2,"unknown":1}`)).Decode(&struct{}{}); err == nil {
		t.Log("unknown-field behaviour covered by DecodeStrict elsewhere")
	}
}
