package main

import (
	"encoding/json"
	"fmt"

	"github.com/binaricat/netcatty/internal/profile/store"
)

// VaultReader reads vault records from the canonical profile store. The
// store holds the renderer-authored JSON; secret fields are redacted at
// this boundary so agents only ever see metadata (capability contract:
// "metadata only — no passwords or keys").
type VaultReader struct {
	store *store.Store
}

func newVaultReader(profileStore *store.Store) *VaultReader {
	return &VaultReader{store: profileStore}
}

// vaultStorageKeys maps catalog read capabilities to the profile-store
// vault-domain key carrying their data.
var vaultStorageKeys = map[string]string{
	"hosts":         "netcatty_hosts_v1",
	"identities":    "netcatty_identities_v1",
	"proxyProfiles": "netcatty_proxy_profiles_v1",
	"groups":        "netcatty_groups_v1",
	"snippets":      "netcatty_snippets_v1",
	"notes":         "netcatty_notes_v1",
}

// secretFieldNames is the recursive redaction deny set. Field NAMES are
// matched, not values, so document text is never touched.
var secretFieldNames = map[string]bool{
	"password":       true,
	"telnetPassword": true,
	"passphrase":     true,
	"secret":         true,
}

// PortForwardingRules returns the redacted port-forwarding rule list from
// the vault domain (netcatty_port_forwarding_v1). Rules carry no secrets
// (hostId references), but the redaction pass still runs for safety.
func (v *VaultReader) PortForwardingRules() ([]any, error) {
	return v.readList("netcatty_port_forwarding_v1")
}

// readList returns the redacted array stored under one vault key. Missing
// keys read as empty arrays — an empty vault is a valid vault.
func (v *VaultReader) readList(vaultKey string) ([]any, error) {
	raw, err := v.store.GetRaw("vault", vaultKey)
	if err != nil || len(raw) == 0 {
		return []any{}, nil
	}
	var array []any
	if err := json.Unmarshal(raw, &array); err != nil {
		return nil, fmt.Errorf("vault read %s: %w", vaultKey, err)
	}
	for i, item := range array {
		array[i] = RedactSecretFields(item)
	}
	return array, nil
}

// findByID returns the redacted record whose id field matches.
func (v *VaultReader) findByID(vaultKey, id string) (any, error) {
	items, err := v.readList(vaultKey)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if obj, ok := item.(map[string]any); ok {
			if obj["id"] == id {
				return obj, nil
			}
		}
	}
	return nil, nil
}

// RedactSecretFields deep-copies a JSON value dropping every field whose
// NAME is in the secret deny set, at any nesting depth. Values are never
// inspected.
func RedactSecretFields(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cleaned := make(map[string]any, len(typed))
		for field, child := range typed {
			if secretFieldNames[field] {
				continue
			}
			cleaned[field] = RedactSecretFields(child)
		}
		return cleaned
	case []any:
		cleaned := make([]any, len(typed))
		for i, child := range typed {
			cleaned[i] = RedactSecretFields(child)
		}
		return cleaned
	default:
		return value
	}
}

// scriptIs filters script-kind entries (snippets carry kind=script for nct
// automation; plain snippets have no kind or kind=snippet).
func scriptIs(item map[string]any, wantScript bool) bool {
	kind := ""
	if raw, ok := item["kind"].(string); ok {
		kind = raw
	}
	isScript := kind == "script"
	return isScript == wantScript
}
