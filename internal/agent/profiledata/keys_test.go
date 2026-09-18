package profiledata

import (
	"encoding/json"
	"testing"
)

func TestInventoryCoversRendererKeys(t *testing.T) {
	if len(AIStorageKeys) < 20 {
		t.Fatalf("inventory unexpectedly small: %d keys", len(AIStorageKeys))
	}
	seen := map[string]bool{}
	for _, spec := range AIStorageKeys {
		if seen[spec.StorageKey] {
			t.Errorf("duplicate inventory entry %s", spec.StorageKey)
		}
		seen[spec.StorageKey] = true
		if !strings_HasPrefix(spec.StorageKey, "netcatty_ai_") {
			t.Errorf("inventory key %s must carry the netcatty_ai_ prefix", spec.StorageKey)
		}
	}
}

func strings_HasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func TestPlanMigrationsClassifiesKeys(t *testing.T) {
	snapshot := map[string]json.RawMessage{
		"netcatty_ai_providers_v1":          json.RawMessage(`[{"id":"p1"}]`),
		"netcatty_ai_sessions_v1":           json.RawMessage(`{"chats":[]}`),
		"netcatty_ai_permission_mode_v1":    json.RawMessage(`"confirm"`),
		"netcatty_ai_active_session_map_v1": json.RawMessage(`{"w1":"s1"}`),
		"netcatty_ai_permission_grants_v1":  json.RawMessage(`[]`),
	}

	mutations, report, err := PlanMigrations(snapshot)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(mutations) != len(snapshot) {
		t.Fatalf("mutations = %d, want %d", len(mutations), len(snapshot))
	}

	byKey := map[string]storeDomainKey{}
	for _, mutation := range mutations {
		byKey[mutation.Key] = storeDomainKey{mutation.Domain, string(mutation.Value)}
	}
	if got := byKey["ai/providers_v1"]; got.domain != "settings" {
		t.Errorf("providers domain = %q, want settings", got.domain)
	}
	if got := byKey["ai/sessions_v1"]; got.domain != "sessions" {
		t.Errorf("sessions domain = %q, want sessions", got.domain)
	}
	if got := byKey["ai/active_session_map_v1"]; got.domain != "device" {
		t.Errorf("active session map must stay device-local (T40), got %q", got.domain)
	}

	if len(report.Ephemeral) != 1 || report.Ephemeral[0] != "netcatty_ai_active_session_map_v1" {
		t.Errorf("ephemeral report = %v", report.Ephemeral)
	}
	if len(report.UnknownKeys) != 0 || len(report.SecretBearing) != 0 {
		t.Errorf("unexpected report: %+v", report)
	}
}

type storeDomainKey struct {
	domain, value string
}

func TestPlanMigrationsSecretBearingFlagged(t *testing.T) {
	snapshot := map[string]json.RawMessage{
		"netcatty_ai_providers_v1": json.RawMessage(`"enc:v1:1:abcdef"`),
	}
	mutations, report, err := PlanMigrations(snapshot)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("secret-bearing values are planned (flagged, not dropped), got %d", len(mutations))
	}
	if len(report.SecretBearing) != 1 || report.SecretBearing[0] != "netcatty_ai_providers_v1" {
		t.Errorf("secret report = %v", report.SecretBearing)
	}
}

func TestPlanMigrationsFailsClosedOnUnknownAIKey(t *testing.T) {
	snapshot := map[string]json.RawMessage{
		"netcatty_ai_providers_v1":       json.RawMessage(`[]`),
		"netcatty_ai_some_future_key_v9": json.RawMessage(`{}`),
		"netcatty_unrelated_key":         json.RawMessage(`{}`), // not AI: ignored
	}
	mutations, report, err := PlanMigrations(snapshot)
	if err == nil {
		t.Fatal("unknown AI key must fail the plan closed")
	}
	var unknownErr *UnknownKeyError
	if !errorsAs(err, &unknownErr) {
		t.Fatalf("error must be UnknownKeyError, got %v", err)
	}
	if mutations != nil {
		t.Errorf("no mutations may be planned when classification drifts, got %d", len(mutations))
	}
	if len(report.UnknownKeys) != 1 {
		t.Errorf("unknown report = %v", report.UnknownKeys)
	}
}

func TestPlanMigrationsRejectsMalformedJSON(t *testing.T) {
	snapshot := map[string]json.RawMessage{
		"netcatty_ai_sessions_v1": json.RawMessage(`{truncated`),
	}
	mutations, _, err := PlanMigrations(snapshot)
	if err == nil {
		t.Fatal("malformed JSON must fail")
	}
	invalidErr, ok := err.(*InvalidValueError)
	if !ok {
		t.Fatalf("error must be InvalidValueError, got %v", err)
	}
	_ = invalidErr
	if mutations != nil {
		t.Errorf("no mutations on invalid value, got %d", len(mutations))
	}
}

func TestPlanMigrationsSkipsEmptyValues(t *testing.T) {
	snapshot := map[string]json.RawMessage{
		"netcatty_ai_quick_messages_v1":  json.RawMessage(""),
		"netcatty_ai_max_iterations_v1":  json.RawMessage("null"),
		"netcatty_ai_permission_mode_v1": json.RawMessage(`"auto"`),
	}
	mutations, report, err := PlanMigrations(snapshot)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("empty values must be skipped, got %d mutations", len(mutations))
	}
	if len(report.EmptyKeys) != 2 {
		t.Errorf("empty report = %v", report.EmptyKeys)
	}
}

func errorsAs(err error, target **UnknownKeyError) bool {
	if e, ok := err.(*UnknownKeyError); ok {
		*target = e
		return true
	}
	return false
}
