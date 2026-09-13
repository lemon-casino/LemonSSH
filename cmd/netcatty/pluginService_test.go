package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/binaricat/netcatty/internal/plugin/v1reject"
)

func TestPluginRejectsLegacyHybrid(t *testing.T) {
	raw := `{"apiVersion":2,"name":"example","version":"1.0.0","displayName":"Example","main":{"browser":"old.js"},"entrypoint":{"wasm":"main.wasm","sha256":"` + strings.Repeat("a", 64) + `"}}`
	if _, err := parseManifestV2(raw); !errors.Is(err, v1reject.ErrV1NotSupported) {
		t.Fatalf("legacy entrypoint accepted: %v", err)
	}
}

func TestPluginPermissionsBoundToDeclaredManifest(t *testing.T) {
	s := newPluginService()
	raw := `{"apiVersion":2,"name":"example","version":"1.0.0","displayName":"Example","entrypoint":{"wasm":"main.wasm","sha256":"` + strings.Repeat("a", 64) + `"},"permissions":[{"kind":"terminal","resource":"session-1","mode":"read"}]}`
	if _, err := s.Install("example", "1.0.0", strings.Repeat("a", 64), raw); err != nil {
		t.Fatal(err)
	}
	if err := s.GrantPermission("example", "terminal", "session-1", "read", "once"); err == nil {
		t.Fatal("disabled plugin granted permission")
	}
	if err := s.SetEnabled("example", true); err != nil {
		t.Fatal(err)
	}
	if err := s.GrantPermission("example", "terminal", "session-1", "write", "once"); err == nil {
		t.Fatal("undeclared write granted")
	}
	if err := s.GrantPermission("example", "terminal", "session-1", "read", "once"); err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizePermission("example", "terminal", "session-1", "read"); err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizePermission("example", "terminal", "session-1", "read"); err == nil {
		t.Fatal("once reused")
	}
}

func TestPluginServiceDurableInstall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.json")
	s, err := newPluginServiceAt(path)
	if err != nil {
		t.Fatal(err)
	}
	raw := `{"apiVersion":2,"name":"example","version":"1.0.0","displayName":"Example","entrypoint":{"wasm":"main.wasm","sha256":"` + strings.Repeat("a", 64) + `"}}`
	if _, err := s.Install("example", "1.0.0", strings.Repeat("a", 64), raw); err != nil {
		t.Fatal(err)
	}
	other, err := newPluginServiceAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(other.List()) != 1 {
		t.Fatal("service inventory was not loaded from disk")
	}
	if _, err := s.Install("different", "1.0.0", strings.Repeat("a", 64), raw); err == nil {
		t.Fatal("manifest identity mismatch accepted")
	}
}
