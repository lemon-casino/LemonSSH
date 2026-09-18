// Package profiledata owns the AI side of the W10 profile migration
// (P7-04, AI-03): the renderer localStorage key inventory, its
// classification into profile domains, and the staged migration plan
// builder. Promotion itself reuses internal/profile/store StageProfile /
// PromoteProfile receipts; this package never invents a second store.
package profiledata

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/binaricat/netcatty/internal/profile/store"
)

// KeyClass buckets each renderer AI storage key by what it carries. The
// class decides the target domain and sync behavior:
//   - Config/Preference/History/Grant map to profile domains that sync.
//   - Ephemeral stays device-local forever (T40: runtime tokens, debug
//     state and vendor session grants must never reach cloud sync).
type KeyClass string

const (
	ClassProviderConfig KeyClass = "provider_config"
	ClassPreference     KeyClass = "preference"
	ClassChatHistory    KeyClass = "chat_history"
	ClassGrant          KeyClass = "grant"
	ClassEphemeral      KeyClass = "ephemeral"
)

// KeySpec is one inventoried renderer AI storage key.
type KeySpec struct {
	// StorageKey is the exact renderer localStorage key.
	StorageKey string
	Class      KeyClass
	// SecretBearing marks keys whose values may hold enc:v1 envelopes
	// (API keys). Such values require the origin-aware reseal decision
	// before promotion (T37) and are never readable through the raw
	// profile channel (T38).
	SecretBearing bool
}

// AIStorageKeys is the frozen W10 inventory (storageKeys.ts, AI section).
var AIStorageKeys = []KeySpec{
	{StorageKey: "netcatty_ai_providers_v1", Class: ClassProviderConfig, SecretBearing: true},
	{StorageKey: "netcatty_ai_active_provider_v1", Class: ClassPreference},
	{StorageKey: "netcatty_ai_active_model_v1", Class: ClassPreference},
	{StorageKey: "netcatty_ai_permission_mode_v1", Class: ClassPreference},
	{StorageKey: "netcatty_ai_tool_integration_mode_v1", Class: ClassPreference},
	{StorageKey: "netcatty_ai_host_permissions_v1", Class: ClassGrant},
	{StorageKey: "netcatty_ai_external_agents_v1", Class: ClassProviderConfig},
	{StorageKey: "netcatty_ai_default_agent_v1", Class: ClassPreference},
	{StorageKey: "netcatty_ai_command_blocklist_v1", Class: ClassPreference},
	{StorageKey: "netcatty_ai_command_timeout_v1", Class: ClassPreference},
	{StorageKey: "netcatty_ai_response_idle_timeout_v1", Class: ClassPreference},
	{StorageKey: "netcatty_ai_max_iterations_v1", Class: ClassPreference},
	{StorageKey: "netcatty_ai_sessions_v1", Class: ClassChatHistory},
	{StorageKey: "netcatty_ai_active_session_map_v1", Class: ClassEphemeral},
	{StorageKey: "netcatty_ai_agent_model_map_v1", Class: ClassPreference},
	{StorageKey: "netcatty_ai_agent_provider_map_v1", Class: ClassPreference},
	{StorageKey: "netcatty_ai_agent_thinking_map_v1", Class: ClassPreference},
	{StorageKey: "netcatty_ai_composer_model_prefs_v1", Class: ClassPreference},
	{StorageKey: "netcatty_ai_web_search_v1", Class: ClassProviderConfig, SecretBearing: true},
	{StorageKey: "netcatty_ai_quick_messages_v1", Class: ClassPreference},
	{StorageKey: "netcatty_ai_permission_grants_v1", Class: ClassGrant},
	{StorageKey: "netcatty_ai_show_terminal_selection_action_v1", Class: ClassPreference},
}

// Spec returns the inventory entry for one storage key, or nil.
func Spec(storageKey string) *KeySpec {
	for i := range AIStorageKeys {
		if AIStorageKeys[i].StorageKey == storageKey {
			return &AIStorageKeys[i]
		}
	}
	return nil
}

// domainFor maps a class to its profile domain. Ephemeral data is
// device-local only (T40).
func (k KeySpec) domainFor() string {
	switch k.Class {
	case ClassChatHistory:
		return "sessions"
	case ClassEphemeral:
		return "device"
	default:
		return "settings"
	}
}

// Syncable reports whether the migrated value may participate in cloud
// sync. Ephemeral and secret-bearing keys stay device-local until their
// dedicated handling lands (T37/T40).
func (k KeySpec) Syncable() bool {
	return !k.SecretBearing && k.Class != ClassEphemeral
}

// ProfileKey is the store key a storage key migrates to: the full
// netcatty_ai_ prefix is replaced by the ai/ namespace.
func (k KeySpec) ProfileKey() string {
	return "ai/" + strings.TrimPrefix(k.StorageKey, "netcatty_ai_")
}

// encV1Prefix marks renderer-encrypted envelope values.
var encV1Prefix = []byte("enc:v1:")

// PlanReport summarizes one planning run; UnknownKeys fails closed so an
// unrecognized renderer key can never silently vanish in a promotion.
type PlanReport struct {
	Mutated       []string
	SecretBearing []string
	Ephemeral     []string
	UnknownKeys   []string
	EmptyKeys     []string
}

// PlanMigrations turns a renderer localStorage snapshot into profile store
// mutations. Rules:
//   - known keys map to their classified domain and profile key;
//   - values must be valid JSON (or empty, which is recorded and skipped);
//   - unknown "netcatty_ai_*" keys fail closed and block the plan;
//   - secret-bearing values are planned but flagged: promotion requires the
//     origin-aware reseal decision before this package's caller stages them
//     (T37/T38).
func PlanMigrations(snapshot map[string]json.RawMessage) ([]store.Mutation, *PlanReport, error) {
	report := &PlanReport{}
	var mutations []store.Mutation

	for storageKey, value := range snapshot {
		spec := Spec(storageKey)
		if spec == nil {
			if strings.HasPrefix(storageKey, "netcatty_ai_") {
				report.UnknownKeys = append(report.UnknownKeys, storageKey)
			}
			continue
		}
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) == 0 || string(trimmed) == "null" {
			report.EmptyKeys = append(report.EmptyKeys, storageKey)
			continue
		}
		if !json.Valid(trimmed) {
			return nil, nil, &InvalidValueError{StorageKey: storageKey}
		}
		// The envelope lives inside a JSON string value, so detection is a
		// containment check on the serialized form.
		if spec.SecretBearing && bytes.Contains(trimmed, encV1Prefix) {
			report.SecretBearing = append(report.SecretBearing, storageKey)
		}
		if spec.Class == ClassEphemeral {
			report.Ephemeral = append(report.Ephemeral, storageKey)
		}
		report.Mutated = append(report.Mutated, storageKey)
		mutations = append(mutations, store.Mutation{
			Domain: spec.domainFor(),
			Key:    spec.ProfileKey(),
			Value:  trimmed,
		})
	}

	if len(report.UnknownKeys) > 0 {
		return nil, report, &UnknownKeyError{Keys: report.UnknownKeys}
	}
	return mutations, report, nil
}

// InvalidValueError marks a snapshot value that is not valid JSON; the
// migration must not half-promote unreadable state (T36).
type InvalidValueError struct {
	StorageKey string
}

func (e *InvalidValueError) Error() string {
	return "profiledata: value for " + e.StorageKey + " is not valid JSON"
}

// UnknownKeyError fails the plan closed when the renderer carries an AI
// key this inventory does not know (T36/T40 classification drift guard).
type UnknownKeyError struct {
	Keys []string
}

func (e *UnknownKeyError) Error() string {
	return "profiledata: unknown AI storage keys present: " + strings.Join(e.Keys, ", ")
}
