package ui

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateAcceptsWellFormed(t *testing.T) {
	schema := Schema{
		Settings: []SettingField{
			{ID: "theme", Type: "select", Label: "Theme", Options: []string{"dark", "light"}, Default: "dark"},
			{ID: "api-key", Type: "password", Label: "API Key", Required: true},
			{ID: "max-retries", Type: "number", Label: "Max Retries", Default: "3"},
		},
		Views: []ViewDef{
			{ID: "main", Type: "list", Title: "Main View", Columns: []string{"name", "status"}},
		},
	}
	if err := Validate(schema); err != nil {
		t.Fatalf("valid schema rejected: %v", err)
	}
}

func TestValidateRejectsInjection(t *testing.T) {
	cases := []struct{ label, desc string }{
		{"<script>alert(1)</script>", ""},
		{"Click <img onerror=alert(1)>", ""},
		{"{{.Env.SECRET}}", ""},
		{"${process.env.HOME}", ""},
		{"Normal", "<iframe src='evil'>"},
	}
	for _, tc := range cases {
		schema := Schema{Settings: []SettingField{{ID: "x", Type: "text", Label: tc.label, Description: tc.desc}}}
		if err := Validate(schema); err == nil {
			t.Fatalf("injection accepted: %q / %q", tc.label, tc.desc)
		}
	}
}

func TestValidateRejectsBadIDsAndTypes(t *testing.T) {
	badIDs := []string{"", "has space", "has/slash", strings.Repeat("x", 129)}
	for _, id := range badIDs {
		schema := Schema{Settings: []SettingField{{ID: id, Type: "text", Label: "X"}}}
		if err := Validate(schema); err == nil {
			t.Fatalf("bad id %q accepted", id)
		}
	}
	schema := Schema{Settings: []SettingField{{ID: "x", Type: "richtext", Label: "X"}}}
	if err := Validate(schema); err == nil {
		t.Fatal("unknown type accepted")
	}
	selectSchema := Schema{Settings: []SettingField{{ID: "x", Type: "select", Label: "X"}}}
	if err := Validate(selectSchema); err == nil {
		t.Fatal("select without options accepted")
	}
}

func TestValidateDuplicateIDs(t *testing.T) {
	schema := Schema{Settings: []SettingField{
		{ID: "same", Type: "text", Label: "A"},
		{ID: "same", Type: "text", Label: "B"},
	}}
	if err := Validate(schema); err == nil {
		t.Fatal("duplicate setting ID accepted")
	}
}

func validContributionSchema() Schema {
	return Schema{
		Views: []ViewDef{
			{ID: "status", Type: "card", Title: "Status", Bindings: []string{"greeting"}, Location: ViewLocationSettings},
			{ID: "rows", Type: "list", Title: "Rows", Columns: []string{"name", "state"}},
		},
		Menus: []MenuDef{
			{ID: "palette-run", Command: "run-job", Location: "commandPalette", Title: "Run Job", Group: "demo", Order: 2},
		},
		Keybindings: []KeybindingDef{
			{Command: "run-job", Key: "ctrl+alt+r", Args: json.RawMessage(`{"source":"accelerator"}`)},
		},
	}
}

func TestValidateAcceptsMenusAndKeybindings(t *testing.T) {
	if err := Validate(validContributionSchema()); err != nil {
		t.Fatalf("well-formed menus/keybindings rejected: %v", err)
	}
}

func TestValidateRejectsBadMenusAndKeybindings(t *testing.T) {
	cases := map[string]Schema{
		"unknown menu location": {
			Menus: []MenuDef{{ID: "m", Command: "run", Location: "startMenu"}},
		},
		"menu injection title": {
			Menus: []MenuDef{{ID: "m", Command: "run", Location: "statusBar", Title: "<img onerror=x>"}},
		},
		"empty menu command": {
			Menus: []MenuDef{{ID: "m", Command: "", Location: "commandPalette"}},
		},
		"duplicate menu id": {
			Menus: []MenuDef{
				{ID: "m", Command: "a", Location: "commandPalette"},
				{ID: "m", Command: "b", Location: "statusBar"},
			},
		},
		"keybinding bad key": {
			Keybindings: []KeybindingDef{{Command: "run", Key: "ctrl+<script>"}},
		},
		"keybinding empty token": {
			Keybindings: []KeybindingDef{{Command: "run", Key: "ctrl++r"}},
		},
		"keybinding invalid args json": {
			Keybindings: []KeybindingDef{{Command: "run", Key: "ctrl+r", Args: json.RawMessage(`{oops}`)}},
		},
		"view unsupported location": {
			Views: []ViewDef{{ID: "v", Type: "card", Title: "V", Location: "aside"}},
		},
		"view injection binding": {
			Views: []ViewDef{{ID: "v", Type: "list", Title: "V", Columns: []string{"${env}"}}},
		},
	}
	for label, schema := range cases {
		if err := Validate(schema); err == nil {
			t.Fatalf("%s accepted", label)
		}
	}
}

func TestCollectContributionsAppliesDefaults(t *testing.T) {
	hidden := false
	disabled := false
	schema := Schema{
		Views: []ViewDef{
			{ID: "implicit", Type: "card", Title: "Implicit"},
			{ID: "hidden", Type: "card", Title: "Hidden", Visible: &hidden},
		},
		Menus: []MenuDef{
			{ID: "palette", Command: "run", Location: "commandPalette"},
			{ID: "buried", Command: "run", Location: "statusBar", Visible: &hidden},
		},
		Keybindings: []KeybindingDef{
			{Command: "run", Key: "ctrl+r"},
			{Command: "run", Key: "ctrl+shift+r", Enabled: &disabled},
		},
	}
	collected := CollectContributions("demo-plugin", schema)
	if len(collected.Views) != 2 || len(collected.Menus) != 2 || len(collected.Keybindings) != 2 {
		t.Fatalf("unexpected collection sizes: %+v", collected)
	}
	if got := collected.Views[0]; got.PluginID != "demo-plugin" || got.Location != ViewLocationSettings || !got.Visible {
		t.Fatalf("view defaults not applied: %+v", got)
	}
	if collected.Views[1].Visible {
		t.Fatal("explicitly hidden view became visible")
	}
	if got := collected.Menus[0]; !got.Visible || got.Title != "" || got.Group != "" {
		t.Fatalf("menu defaults not applied: %+v", got)
	}
	if collected.Menus[1].Visible {
		t.Fatal("explicitly hidden menu became visible")
	}
	if got := collected.Keybindings[0]; !got.Enabled || got.Key != "ctrl+r" {
		t.Fatalf("keybinding defaults not applied: %+v", got)
	}
	if collected.Keybindings[1].Enabled {
		t.Fatal("explicitly disabled keybinding became enabled")
	}
}
