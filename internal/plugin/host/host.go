// Package host connects installed manifests to the plugin-only permission broker.
package host

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/binaricat/netcatty/internal/platform/credentials"
	"github.com/binaricat/netcatty/internal/plugin/manifest"
	"github.com/binaricat/netcatty/internal/plugin/permissions"
	"github.com/binaricat/netcatty/internal/plugin/store"
	"github.com/binaricat/netcatty/internal/plugin/ui"
)

type Host struct {
	Credentials credentials.Provider
	Store       *store.Store
	Broker      *permissions.Broker
}

func (h Host) Manifest(id string) (manifest.Manifest, error) {
	record, ok := h.Store.Get(id)
	if !ok || record.State != store.StateEnabled {
		return manifest.Manifest{}, permissions.ErrPrincipalUnknown
	}
	var m manifest.Manifest
	if err := json.Unmarshal(record.Manifest, &m); err != nil {
		return m, err
	}
	return m, manifest.Validate(m)
}

func (h Host) UI(id string) (*ui.Schema, error) {
	m, err := h.Manifest(id)
	if err != nil {
		return nil, err
	}
	if m.UI == nil {
		return &ui.Schema{}, nil
	}
	return m.UI, nil
}

func (h Host) Settings(id string) (map[string]any, error) {
	schema, err := h.UI(id)
	if err != nil {
		return nil, err
	}
	record, _ := h.Store.Get(id)
	result := make(map[string]any)
	for _, field := range schema.Settings {
		if field.Type == "password" {
			continue
		}
		if raw, ok := record.Settings[field.ID]; ok {
			var value any
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, err
			}
			result[field.ID] = value
		}
	}
	return result, nil
}

func (h Host) SetSetting(id, key, valueJSON string) error {
	schema, err := h.UI(id)
	if err != nil {
		return err
	}
	var value any
	if err := json.Unmarshal([]byte(valueJSON), &value); err != nil {
		return err
	}
	for _, field := range schema.Settings {
		if field.ID != key {
			continue
		}
		valid := false
		switch field.Type {
		case "password":
			text, ok := value.(string)
			if !ok || (field.Required && text == "") {
				return fmt.Errorf("invalid secret setting")
			}
			if h.Credentials == nil {
				return fmt.Errorf("secret settings require a credential provider; plaintext storage is disabled")
			}
			sealed, err := h.Credentials.Seal([]byte(text), "plugin."+id+"."+key)
			if err != nil {
				return err
			}
			encoded, _ := json.Marshal(map[string]string{"sealed": base64.StdEncoding.EncodeToString(sealed)})
			return h.Store.SetSetting(id, key, encoded)
		case "text":
			_, valid = value.(string)
		case "number":
			_, valid = value.(float64)
		case "boolean":
			_, valid = value.(bool)
		case "select":
			if text, ok := value.(string); ok {
				for _, option := range field.Options {
					if text == option {
						valid = true
					}
				}
			}
		}
		if text, ok := value.(string); field.Required && ok && text == "" {
			valid = false
		}
		if !valid {
			return fmt.Errorf("invalid value for setting %q", key)
		}
		return h.Store.SetSetting(id, key, json.RawMessage(valueJSON))
	}
	return fmt.Errorf("undeclared plugin setting %q", key)
}

func (h Host) resource(id, kind, resource, mode string) (string, error) {
	m, err := h.Manifest(id)
	if err != nil {
		return "", err
	}
	for _, p := range m.Permissions {
		if p.Kind == kind && p.Resource == resource && p.Mode == mode {
			// JSON tuple encoding prevents delimiter collisions between kind and resource.
			key, _ := json.Marshal([]string{kind, resource})
			return string(key) + ":" + mode, nil
		}
	}
	return "", fmt.Errorf("%w: permission is not declared by plugin", permissions.ErrNotGranted)
}

func (h Host) Grant(id, kind, resource, mode, lifetime string) error {
	key, err := h.resource(id, kind, resource, mode)
	if err != nil {
		return err
	}
	if lifetime != string(permissions.LifetimeOnce) && lifetime != string(permissions.LifetimeSession) {
		return fmt.Errorf("unsupported plugin grant lifetime")
	}
	_, err = h.Broker.Grant(id, key, permissions.Lifetime(lifetime), 0)
	return err
}

func (h Host) Authorize(id, kind, resource, mode string) error {
	key, err := h.resource(id, kind, resource, mode)
	if err != nil {
		return err
	}
	return h.Broker.Check(id, key, mode)
}
