package host

import (
	"encoding/json"
	"github.com/binaricat/netcatty/internal/plugin/permissions"
	"github.com/binaricat/netcatty/internal/plugin/store"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsValidatePersistAndRejectUndeclared(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.json")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"apiVersion":2,"name":"example","version":"1.0.0","displayName":"Example","entrypoint":{"wasm":"main.wasm","sha256":"` + strings.Repeat("a", 64) + `"},"ui":{"settings":[{"id":"mode","type":"select","label":"Mode","options":["dark","light"]},{"id":"token","type":"password","label":"Token"}]}}`)
	_, err = s.Install("example", "1.0.0", strings.Repeat("a", 64), raw)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetState("example", store.StateEnabled); err != nil {
		t.Fatal(err)
	}
	h := Host{Store: s, Broker: permissions.NewBroker(nil)}
	if err := h.SetSetting("example", "mode", `"dark"`); err != nil {
		t.Fatal(err)
	}
	if err := h.SetSetting("example", "mode", `"other"`); err == nil {
		t.Fatal("invalid select accepted")
	}
	if err := h.SetSetting("example", "missing", `true`); err == nil {
		t.Fatal("undeclared setting accepted")
	}
	if err := h.SetSetting("example", "token", `"secret"`); err == nil {
		t.Fatal("secret written to plaintext inventory")
	}
	restored, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	values, err := (Host{Store: restored}).Settings("example")
	if err != nil || values["mode"] != "dark" {
		t.Fatal("setting not durable", values, err)
	}
}
