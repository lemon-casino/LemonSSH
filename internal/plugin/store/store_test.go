package store

import (
	"encoding/json"
	"testing"
)

func TestInstallGetAndDuplicate(t *testing.T) {
	s := New()
	manifest := json.RawMessage(`{"apiVersion":2,"name":"demo","version":"1.0.0"}`)
	record, err := s.Install("demo-plugin", "1.0.0", Checksum([]byte("wasm-bytes")), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if record.State != StateInstalled {
		t.Fatal("state must be installed")
	}
	if _, err := s.Install("demo-plugin", "1.0.0", "abc", manifest); err == nil {
		t.Fatal("duplicate install must fail")
	}
	got, ok := s.Get("demo-plugin")
	if !ok || got.Version != "1.0.0" {
		t.Fatalf("get mismatch: %+v", got)
	}
}

func TestUninstallAndSetState(t *testing.T) {
	s := New()
	_, _ = s.Install("p1", "1.0.0", Checksum([]byte("x")), json.RawMessage(`{}`))
	if err := s.SetState("p1", StateEnabled); err != nil {
		t.Fatal(err)
	}
	record, _ := s.Get("p1")
	if record.State != StateEnabled {
		t.Fatal("state mismatch")
	}
	if err := s.Uninstall("p1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("p1"); ok {
		t.Fatal("uninstalled must be gone")
	}
	if err := s.SetState("p1", StateDisabled); err == nil {
		t.Fatal("state change on missing plugin must fail")
	}
}

func TestListSortedByID(t *testing.T) {
	s := New()
	sha := func(s string) string {
		for len(s) < 64 {
			s += "0"
		}
		return s[:64]
	}
	_, _ = s.Install("c-plugin", "1.0.0", sha("c"), json.RawMessage(`{}`))
	_, _ = s.Install("a-plugin", "1.0.0", sha("a"), json.RawMessage(`{}`))
	_, _ = s.Install("b-plugin", "1.0.0", sha("b"), json.RawMessage(`{}`))
	list := s.List()
	if len(list) != 3 {
		t.Fatalf("expected 3, got %d", len(list))
	}
	if list[0].PluginID != "a-plugin" || list[2].PluginID != "c-plugin" {
		t.Fatalf("sort mismatch: %s, %s, %s", list[0].PluginID, list[1].PluginID, list[2].PluginID)
	}
}
