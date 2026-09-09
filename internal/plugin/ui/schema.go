// Package ui owns declarative plugin UI schema validation (P5-04): host
// renders settings forms, list views and cards from a validated schema. The
// schema is the only path for plugin-provided UI; no arbitrary HTML/JS/CSS
// can be injected.
package ui

import (
	"errors"
	"fmt"
	"strings"
)

var ErrSchemaInvalid = errors.New("plugin UI schema invalid")

// Schema is the root declarative UI definition for one plugin.
type Schema struct {
	Settings []SettingField `json:"settings"`
	Views    []ViewDef      `json:"views"`
}

// SettingField is one form field.
type SettingField struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"` // "text" | "number" | "boolean" | "select" | "password"
	Label       string   `json:"label"`
	Default     string   `json:"default,omitempty"`
	Options     []string `json:"options,omitempty"` // for select
	Required    bool     `json:"required,omitempty"`
	Description string   `json:"description,omitempty"`
}

// ViewDef is one declarative view.
type ViewDef struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"` // "list" | "card"
	Title    string   `json:"title"`
	Columns  []string `json:"columns,omitempty"`  // for list
	Bindings []string `json:"bindings,omitempty"` // data bindings
}

// Validate enforces the UI schema contract: safe IDs, known types, no
// injection vectors (HTML/JS/CSS in labels or descriptions).
func Validate(schema Schema) error {
	seenSettings := make(map[string]bool)
	for _, field := range schema.Settings {
		if err := validateID(field.ID); err != nil {
			return err
		}
		if seenSettings[field.ID] {
			return fmt.Errorf("%w: duplicate setting %q", ErrSchemaInvalid, field.ID)
		}
		seenSettings[field.ID] = true
		switch field.Type {
		case "text", "number", "boolean", "password":
		case "select":
			if len(field.Options) == 0 {
				return fmt.Errorf("%w: select %q has no options", ErrSchemaInvalid, field.ID)
			}
		default:
			return fmt.Errorf("%w: unknown setting type %q", ErrSchemaInvalid, field.Type)
		}
		if err := validateText(field.Label, "label"); err != nil {
			return err
		}
		if err := validateText(field.Description, "description"); err != nil {
			return err
		}
	}
	seenViews := make(map[string]bool)
	for _, view := range schema.Views {
		if err := validateID(view.ID); err != nil {
			return err
		}
		if seenViews[view.ID] {
			return fmt.Errorf("%w: duplicate view %q", ErrSchemaInvalid, view.ID)
		}
		seenViews[view.ID] = true
		switch view.Type {
		case "list", "card":
		default:
			return fmt.Errorf("%w: unknown view type %q", ErrSchemaInvalid, view.Type)
		}
		if err := validateText(view.Title, "title"); err != nil {
			return err
		}
	}
	return nil
}

func validateID(id string) error {
	if id == "" || len(id) > 128 {
		return fmt.Errorf("%w: invalid id %q", ErrSchemaInvalid, id)
	}
	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '.' || r == '_' {
			continue
		}
		return fmt.Errorf("%w: invalid char in id %q", ErrSchemaInvalid, id)
	}
	return nil
}

// validateText rejects HTML/script injection vectors in user-visible text.
func validateText(value, field string) error {
	if len(value) > 4096 {
		return fmt.Errorf("%w: %s too long", ErrSchemaInvalid, field)
	}
	lower := strings.ToLower(value)
	for _, pattern := range []string{"<script", "</script", "<img ", "<iframe", "javascript:", "onerror=", "onload=", "{{", "${"} {
		if strings.Contains(lower, pattern) {
			return fmt.Errorf("%w: %s contains injection vector %q", ErrSchemaInvalid, field, pattern)
		}
	}
	return nil
}
