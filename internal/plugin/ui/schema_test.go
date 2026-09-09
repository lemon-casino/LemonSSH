package ui

import (
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
