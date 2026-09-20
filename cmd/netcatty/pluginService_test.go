package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
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

func writePluginArchive(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, contents := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "plugin.ncpkg")
	if err := os.WriteFile(archivePath, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return archivePath
}

func TestReadPluginArchiveValidatesChecksumAndPaths(t *testing.T) {
	wasmBytes := []byte("test-wasm")
	sum := sha256.Sum256(wasmBytes)
	manifestJSON := `{"apiVersion":2,"name":"example","version":"1.0.0","displayName":"Example","entrypoint":{"wasm":"main.wasm","sha256":"` + hex.EncodeToString(sum[:]) + `"}}`
	archivePath := writePluginArchive(t, map[string][]byte{
		"lemonssh.plugin.json": []byte(manifestJSON),
		"main.wasm":            wasmBytes,
	})
	parsed, _, gotWASM, err := readPluginArchive(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Name != "example" || !bytes.Equal(gotWASM, wasmBytes) {
		t.Fatalf("unexpected package contents: %#v %q", parsed, gotWASM)
	}

	unsafePath := writePluginArchive(t, map[string][]byte{
		"../lemonssh.plugin.json": []byte(manifestJSON),
		"main.wasm":               wasmBytes,
	})
	if _, _, _, err := readPluginArchive(unsafePath); err == nil || !strings.Contains(err.Error(), "unsafe plugin package path") {
		t.Fatalf("unsafe archive path accepted: %v", err)
	}
}
