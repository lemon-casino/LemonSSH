package main

import (
	"encoding/json"
	"fmt"

	"github.com/lemon-casino/lemonssh/internal/profile/store"
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
// vault-domain key carrying their data. Reads fall back to the legacy
// netcatty_* rows (see vaultLegacyKeys) until the storage-key migration has
// copied them, so a pre-rename profile keeps answering vault tools.
var vaultStorageKeys = map[string]string{
	"hosts":         "lemonssh_hosts_v1",
	"identities":    "lemonssh_identities_v1",
	"proxyProfiles": "lemonssh_proxy_profiles_v1",
	"groups":        "lemonssh_groups_v1",
	"snippets":      "lemonssh_snippets_v1",
	"notes":         "lemonssh_notes_v1",
}

// vaultLegacyKeys maps each current vault key to its pre-rename name.
var vaultLegacyKeys = map[string]string{
	"lemonssh_hosts_v1":           "netcatty_hosts_v1",
	"lemonssh_identities_v1":      "netcatty_identities_v1",
	"lemonssh_proxy_profiles_v1":  "netcatty_proxy_profiles_v1",
	"lemonssh_groups_v1":          "netcatty_groups_v1",
	"lemonssh_snippets_v1":        "netcatty_snippets_v1",
	"lemonssh_notes_v1":           "netcatty_notes_v1",
	"lemonssh_port_forwarding_v1": "netcatty_port_forwarding_v1",
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
// the vault domain (lemonssh_port_forwarding_v1, legacy netcatty fallback).
// Rules carry no secrets (hostId references), but the redaction pass still
// runs for safety.
func (v *VaultReader) PortForwardingRules() ([]any, error) {
	return v.readList("lemonssh_port_forwarding_v1")
}

// readList returns the redacted array stored under one vault key, falling
// back to the pre-rename key when the current one is absent. Missing keys
// read as empty arrays — an empty vault is a valid vault.
func (v *VaultReader) readList(vaultKey string) ([]any, error) {
	raw, err := v.store.GetRaw("vault", vaultKey)
	if (err != nil || len(raw) == 0) && vaultLegacyKeys[vaultKey] != "" {
		if legacy, legacyErr := v.store.GetRaw("vault", vaultLegacyKeys[vaultKey]); legacyErr == nil && len(legacy) > 0 {
			raw, err = legacy, nil
		}
	}
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
