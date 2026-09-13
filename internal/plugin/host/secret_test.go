package host

import (
	"encoding/json"
	"errors"
	"github.com/binaricat/netcatty/internal/platform/credentials"
	"github.com/binaricat/netcatty/internal/plugin/store"
	"strings"
	"testing"
)

type testKeyring map[string]string

func (k testKeyring) Get(s, u string) (string, error) {
	value, ok := k[s+u]
	if !ok {
		return "", errors.New("missing")
	}
	return value, nil
}
func (k testKeyring) Set(s, u, p string) error { k[s+u] = p; return nil }
func (k testKeyring) Delete(s, u string) error { delete(k, s+u); return nil }

func TestSecretSettingSealedAndNeverReturned(t *testing.T) {
	s := store.New()
	raw := json.RawMessage(`{"apiVersion":2,"name":"example","version":"1.0.0","displayName":"Example","entrypoint":{"wasm":"main.wasm","sha256":"` + strings.Repeat("a", 64) + `"},"ui":{"settings":[{"id":"token","type":"password","label":"Token"}]}}`)
	_, _ = s.Install("example", "1.0.0", strings.Repeat("a", 64), raw)
	_ = s.SetState("example", store.StateEnabled)
	h := Host{Store: s, Credentials: credentials.New(testKeyring{})}
	if err := h.SetSetting("example", "token", `"sensitive-value"`); err != nil {
		t.Fatal(err)
	}
	record, _ := s.Get("example")
	bytes, _ := json.Marshal(record)
	if strings.Contains(string(bytes), "sensitive-value") {
		t.Fatal("plaintext stored")
	}
	values, err := h.Settings("example")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := values["token"]; ok {
		t.Fatal("secret returned to UI")
	}
}
