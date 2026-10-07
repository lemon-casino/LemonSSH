// Package ui owns declarative plugin UI schema validation (P5-04): host
// renders settings forms, list views and cards from a validated schema. The
// schema is the only path for plugin-provided UI; no arbitrary HTML/JS/CSS
// can be injected.
package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrSchemaInvalid = errors.New("plugin UI schema invalid")

// ViewLocationSettings is the only view location the host renders today:
// declarative views surface inside Settings → Plugins (and host-owned
// overlays) through components/plugins.DeclarativePluginHost.
const ViewLocationSettings = "settings"

// Schema is the root declarative UI definition for one plugin.
type Schema struct {
	Settings    []SettingField  `json:"settings"`
	Views       []ViewDef       `json:"views"`
	Menus       []MenuDef       `json:"menus,omitempty"`
	Keybindings []KeybindingDef `json:"keybindings,omitempty"`
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

// ViewDef is one declarative view. Location is restricted to
// ViewLocationSettings for now; Visible defaults to true when nil so existing
// manifests keep rendering their views.
type ViewDef struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"` // "list" | "card"
	Title    string   `json:"title"`
	Location string   `json:"location,omitempty"` // only "settings" is supported today
	Columns  []string `json:"columns,omitempty"`  // for list
	Bindings []string `json:"bindings,omitempty"` // data bindings
	Visible  *bool    `json:"visible,omitempty"`  // nil means visible
}

// MenuDef declares one menu entry that surfaces a plugin command. The
// referenced command must exist in the manifest's contributions (validated by
// the manifest package, which sees both blocks).
type MenuDef struct {
	ID       string `json:"id"`
	Command  string `json:"command"`
	Alt      string `json:"alt,omitempty"`      // alternate command id
	Location string `json:"location"`           // commandPalette | application | host/context | terminal/context | terminal/toolbar | statusBar
	Title    string `json:"title,omitempty"`    // label override; falls back to the command id
	Group    string `json:"group,omitempty"`    // sort group
	Order    int    `json:"order,omitempty"`    // sort order inside the group
	Visible  *bool  `json:"visible,omitempty"`  // nil means visible
}

// KeybindingDef binds one plugin command to an accelerator. Platform-specific
// overrides are optional; Enabled defaults to true when nil.
type KeybindingDef struct {
	Command string          `json:"command"`
	Key     string          `json:"key"`
	Mac     string          `json:"mac,omitempty"`
	Linux   string          `json:"linux,omitempty"`
	Windows string          `json:"windows,omitempty"`
	Args    json.RawMessage `json:"args,omitempty"` // pre-validated JSON payload for the command
	Enabled *bool           `json:"enabled,omitempty"`
}

// ContributionView/Menu/Keybinding are the host-facing contribution records
// derived from a validated Schema; CollectContributions applies the nil-means
// defaults so consumers (bridge, Wails renderer) never re-derive them.
type ContributionView struct {
	PluginID string   `json:"pluginId"`
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Title    string   `json:"title"`
	Location string   `json:"location"`
	Columns  []string `json:"columns,omitempty"`
	Bindings []string `json:"bindings,omitempty"`
	Visible  bool     `json:"visible"`
}

type ContributionMenu struct {
	PluginID string `json:"pluginId"`
	ID       string `json:"id"`
	Command  string `json:"command"`
	Alt      string `json:"alt,omitempty"`
	Location string `json:"location"`
	Title    string `json:"title"`
	Group    string `json:"group,omitempty"`
	Order    int    `json:"order,omitempty"`
	Visible  bool   `json:"visible"`
}

type ContributionKeybinding struct {
	PluginID string          `json:"pluginId"`
	Command  string          `json:"command"`
	Key      string          `json:"key"`
	Mac      string          `json:"mac,omitempty"`
	Linux    string          `json:"linux,omitempty"`
	Windows  string          `json:"windows,omitempty"`
	Args     json.RawMessage `json:"args,omitempty"`
	Enabled  bool            `json:"enabled"`
}

// Contributions is the aggregate declarative UI contribution set of one
// plugin (or of every enabled plugin when the host aggregates).
type Contributions struct {
	Views       []ContributionView       `json:"views"`
	Menus       []ContributionMenu       `json:"menus"`
	Keybindings []ContributionKeybinding `json:"keybindings"`
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
		// Only the settings surface exists today; anything else must wait for
		// a host rendering path instead of silently hiding the view.
		if view.Location != "" && view.Location != ViewLocationSettings {
			return fmt.Errorf("%w: view %q location %q is not supported", ErrSchemaInvalid, view.ID, view.Location)
		}
		if len(view.Columns) > maxViewKeys || len(view.Bindings) > maxViewKeys {
			return fmt.Errorf("%w: view %q exceeds %d columns/bindings", ErrSchemaInvalid, view.ID, maxViewKeys)
		}
		for _, key := range append(append([]string{}, view.Columns...), view.Bindings...) {
			if err := validateText(key, "column/binding"); err != nil {
				return err
			}
		}
	}
	seenMenus := make(map[string]bool)
	for _, menu := range schema.Menus {
		if err := validateID(menu.ID); err != nil {
			return err
		}
		if seenMenus[menu.ID] {
			return fmt.Errorf("%w: duplicate menu %q", ErrSchemaInvalid, menu.ID)
		}
		seenMenus[menu.ID] = true
		if err := validateCommandRef(menu.Command); err != nil {
			return err
		}
		if menu.Alt != "" {
			if err := validateCommandRef(menu.Alt); err != nil {
				return err
			}
		}
		if !validMenuLocations[menu.Location] {
			return fmt.Errorf("%w: menu %q has unknown location %q", ErrSchemaInvalid, menu.ID, menu.Location)
		}
		if err := validateText(menu.Title, "menu title"); err != nil {
			return err
		}
		if err := validateText(menu.Group, "menu group"); err != nil {
			return err
		}
	}
	seenKeybindings := make(map[string]bool)
	for _, binding := range schema.Keybindings {
		if err := validateCommandRef(binding.Command); err != nil {
			return err
		}
		ref := binding.Command + ":" + binding.Key
		if seenKeybindings[ref] {
			return fmt.Errorf("%w: duplicate keybinding %q", ErrSchemaInvalid, ref)
		}
		seenKeybindings[ref] = true
		for _, key := range []string{binding.Key, binding.Mac, binding.Linux, binding.Windows} {
			if key == "" {
				continue
			}
			if err := validateAccelerator(key); err != nil {
				return err
			}
		}
		if len(binding.Args) > 0 {
			var probe any
			if err := json.Unmarshal(binding.Args, &probe); err != nil {
				return fmt.Errorf("%w: keybinding %q args are not valid JSON", ErrSchemaInvalid, ref)
			}
		}
	}
	return nil
}

const maxViewKeys = 64

var validMenuLocations = map[string]bool{
	"commandPalette":   true,
	"application":      true,
	"host/context":     true,
	"terminal/context": true,
	"terminal/toolbar": true,
	"statusBar":        true,
}

// validateCommandRef bounds a contribution command reference; the manifest
// package additionally cross-checks that the referenced command exists.
func validateCommandRef(command string) error {
	if command == "" || len(command) > 128 {
		return fmt.Errorf("%w: invalid command reference %q", ErrSchemaInvalid, command)
	}
	return validateText(command, "command reference")
}

// validateAccelerator keeps keybinding strings to accelerator-shaped ASCII:
// modifier and key tokens joined by '+', with function/arrow/space keys
// allowed. Anything else (HTML, template syntax, whitespace runs) is rejected.
func validateAccelerator(key string) error {
	if len(key) > 128 {
		return fmt.Errorf("%w: accelerator too long %q", ErrSchemaInvalid, key)
	}
	for _, token := range strings.Split(key, "+") {
		normalized := strings.ToLower(strings.TrimSpace(token))
		if normalized == "" {
			return fmt.Errorf("%w: accelerator %q has an empty token", ErrSchemaInvalid, key)
		}
		if len(normalized) > 32 {
			return fmt.Errorf("%w: accelerator %q token too long", ErrSchemaInvalid, key)
		}
		for _, r := range normalized {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
				continue
			}
			return fmt.Errorf("%w: accelerator %q has invalid char %q", ErrSchemaInvalid, key, string(r))
		}
	}
	return nil
}

// CollectContributions derives the host-facing contribution records from a
// validated schema, applying the nil-means-default rules (views and menus are
// visible, keybindings enabled, views default to the settings location).
func CollectContributions(pluginID string, schema Schema) Contributions {
	var out Contributions
	for _, view := range schema.Views {
		location := view.Location
		if location == "" {
			location = ViewLocationSettings
		}
		out.Views = append(out.Views, ContributionView{
			PluginID: pluginID,
			ID:       view.ID,
			Type:     view.Type,
			Title:    view.Title,
			Location: location,
			Columns:  view.Columns,
			Bindings: view.Bindings,
			Visible:  view.Visible == nil || *view.Visible,
		})
	}
	for _, menu := range schema.Menus {
		out.Menus = append(out.Menus, ContributionMenu{
			PluginID: pluginID,
			ID:       menu.ID,
			Command:  menu.Command,
			Alt:      menu.Alt,
			Location: menu.Location,
			Title:    menu.Title,
			Group:    menu.Group,
			Order:    menu.Order,
			Visible:  menu.Visible == nil || *menu.Visible,
		})
	}
	for _, binding := range schema.Keybindings {
		out.Keybindings = append(out.Keybindings, ContributionKeybinding{
			PluginID: pluginID,
			Command:  binding.Command,
			Key:      binding.Key,
			Mac:      binding.Mac,
			Linux:    binding.Linux,
			Windows:  binding.Windows,
			Args:     binding.Args,
			Enabled:  binding.Enabled == nil || *binding.Enabled,
		})
	}
	return out
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
