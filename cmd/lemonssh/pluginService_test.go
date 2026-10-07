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

	pluginstore "github.com/lemon-casino/lemonssh/internal/plugin/store"
	"github.com/lemon-casino/lemonssh/internal/plugin/v1reject"
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

// PluginService.UIContributions is the Wails query surface behind the renderer
// plugin bridge: it must expose the schema-derived views, menus and keybindings
// of enabled plugins only, with nil-means-default fields resolved.
func TestPluginServiceUIContributions(t *testing.T) {
	s := newPluginService()
	raw := `{"apiVersion":2,"name":"example","version":"1.0.0","displayName":"Example","entrypoint":{"wasm":"main.wasm","sha256":"` + strings.Repeat("a", 64) + `"},"contributions":[{"type":"command","id":"example.run"}],"ui":{"settings":[{"id":"greeting","type":"text","label":"Greeting","default":"hi"}],"views":[{"id":"status","type":"card","title":"Status","bindings":["greeting"]},{"id":"hidden","type":"card","title":"Hidden","visible":false}],"menus":[{"id":"palette","command":"example.run","location":"commandPalette","title":"Run","order":3,"group":"demo"}],"keybindings":[{"command":"example.run","key":"ctrl+alt+e","windows":"ctrl+shift+e","args":{"origin":"accelerator"}}]}}`
	if _, err := s.Install("example", "1.0.0", strings.Repeat("a", 64), raw); err != nil {
		t.Fatal(err)
	}
	if err := s.SetEnabled("example", true); err != nil {
		t.Fatal(err)
	}

	contributions, err := s.UIContributions()
	if err != nil {
		t.Fatal(err)
	}
	if len(contributions.Views) != 2 || len(contributions.Menus) != 1 || len(contributions.Keybindings) != 1 {
		t.Fatalf("unexpected contribution counts: %+v", contributions)
	}
	view := contributions.Views[0]
	if view.PluginID != "example" || view.ID != "status" || view.Location != "settings" || !view.Visible || len(view.Bindings) != 1 || view.Bindings[0] != "greeting" {
		t.Fatalf("view contribution wrong: %+v", view)
	}
	if contributions.Views[1].Visible {
		t.Fatal("hidden view surfaced")
	}
	menu := contributions.Menus[0]
	if menu.Command != "example.run" || menu.Location != "commandPalette" || menu.Title != "Run" || menu.Order != 3 || menu.Group != "demo" || !menu.Visible {
		t.Fatalf("menu contribution wrong: %+v", menu)
	}
	binding := contributions.Keybindings[0]
	if binding.Command != "example.run" || binding.Key != "ctrl+alt+e" || binding.Windows != "ctrl+shift+e" || !binding.Enabled {
		t.Fatalf("keybinding contribution wrong: %+v", binding)
	}
	if string(binding.Args) != `{"origin":"accelerator"}` {
		t.Fatalf("keybinding args wrong: %s", binding.Args)
	}

	if err := s.SetEnabled("example", false); err != nil {
		t.Fatal(err)
	}
	contributions, err = s.UIContributions()
	if err != nil {
		t.Fatal(err)
	}
	if len(contributions.Views) != 0 || len(contributions.Menus) != 0 || len(contributions.Keybindings) != 0 {
		t.Fatalf("disabled plugin still contributed: %+v", contributions)
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

// examplePluginDir resolves the shipped hello-lemonssh example, the repository's
// contract fixture: if the Go host and the example drift apart, this test and
// check:plugin-contract fail.
func examplePluginDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "examples", "plugins", "hello-lemonssh"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "lemonssh.plugin.json")); err != nil {
		t.Fatalf("example plugin missing: %v", err)
	}
	return dir
}

// packExamplePlugin zips the example directory the same way plugin-cli pack
// does (deflate, forward-slash names) so InstallPackage exercises the real
// archive path end to end.
func packExamplePlugin(t *testing.T) string {
	t.Helper()
	dir := examplePluginDir(t)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entries := []string{"lemonssh.plugin.json", "hello.wasm"}
	for _, name := range entries {
		contents, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
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
	archivePath := filepath.Join(t.TempDir(), "hello-lemonssh.ncpkg")
	if err := os.WriteFile(archivePath, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return archivePath
}

// TestExamplePluginInstallsThroughService is the host-level contract check: the
// official example must install, enable and instantiate through the same Wails
// service methods the frontend plugin manager calls.
func TestExamplePluginInstallsThroughService(t *testing.T) {
	archivePath := packExamplePlugin(t)
	s, err := newPluginServiceAt(filepath.Join(t.TempDir(), "plugins.json"))
	if err != nil {
		t.Fatal(err)
	}
	record, err := s.InstallPackage(archivePath, PluginPackageInstallOptions{Enable: true})
	if err != nil {
		t.Fatalf("example plugin package rejected by the Go host: %v", err)
	}
	if record.PluginID != "hello-lemonssh" || record.State != pluginstore.StateEnabled {
		t.Fatalf("unexpected install result: %+v", record)
	}
	if len(s.List()) != 1 {
		t.Fatalf("expected exactly one installed plugin, got %d", len(s.List()))
	}

	// The installed plugin exposes its declarative settings through the host.
	schema, err := s.UISchema("hello-lemonssh")
	if err != nil {
		t.Fatal(err)
	}
	if len(schema.Settings) != 1 || schema.Settings[0].ID != "com.lemonssh.hello.greeting" {
		t.Fatalf("unexpected ui schema: %+v", schema.Settings)
	}
	if err := s.SetSetting("hello-lemonssh", "com.lemonssh.hello.greeting", `"Modified greeting"`); err != nil {
		t.Fatal(err)
	}
	values, err := s.Settings("hello-lemonssh")
	if err != nil {
		t.Fatal(err)
	}
	if values["com.lemonssh.hello.greeting"] != "Modified greeting" {
		t.Fatalf("setting not stored: %#v", values)
	}

	// CallPlugin drives the lemonssh-wasm-abi v1 dispatch channel end to end.
	// The example answers ping -> pong; the greeting stays empty because the
	// runtime host imports are broker-gated and no grants exist yet.
	ping, err := s.CallPlugin("hello-lemonssh", "ping", `{"from":"test"}`)
	if err != nil {
		t.Fatalf("CallPlugin failed: %v", err)
	}
	if !ping.OK || !strings.Contains(string(ping.Result), `"pong":true`) {
		t.Fatalf("unexpected ping envelope: %+v", ping)
	}
	if !strings.Contains(string(ping.Result), `"greeting":""`) {
		t.Fatalf("greeting must degrade to empty without a settings grant: %s", ping.Result)
	}
	// Unknown methods surface the plugin's own structured in-band error.
	unknown, err := s.CallPlugin("hello-lemonssh", "nope", "")
	if err != nil {
		t.Fatalf("CallPlugin transport failed: %v", err)
	}
	if unknown.OK || unknown.Error == nil || unknown.Error.Code != "not_found" {
		t.Fatalf("unexpected unknown-method envelope: %+v", unknown)
	}
	// Granting the manifest-declared runtime permissions (trusted host UI
	// path) lets the example read its greeting and log the dispatch.
	if err := s.GrantPermission("hello-lemonssh", "runtime", "log", "write", "session"); err != nil {
		t.Fatal(err)
	}
	if err := s.GrantPermission("hello-lemonssh", "runtime", "settings", "read", "session"); err != nil {
		t.Fatal(err)
	}
	granted, err := s.CallPlugin("hello-lemonssh", "ping", "")
	if err != nil {
		t.Fatal(err)
	}
	if !granted.OK || !strings.Contains(string(granted.Result), `"greeting":"Modified greeting"`) {
		t.Fatalf("granted greeting must flow through the channel: %+v", granted)
	}
	if _, err := s.CallPlugin("missing-plugin", "ping", ""); !errors.Is(err, pluginstore.ErrNotInstalled) {
		t.Fatalf("unknown plugin must fail: %v", err)
	}

	// Restart re-instantiates the WASM entrypoint and keeps the plugin enabled.
	restarted, err := s.Restart("hello-lemonssh")
	if err != nil {
		t.Fatal(err)
	}
	if restarted.State != pluginstore.StateEnabled {
		t.Fatalf("restart dropped the plugin out of enabled state: %+v", restarted)
	}

	// Uninstall removes the record and the stored package file.
	storedPath := restarted.Labels["packagePath"]
	if storedPath == "" {
		t.Fatal("installed package path label missing")
	}
	removed, err := s.Uninstall("hello-lemonssh")
	if err != nil || !removed {
		t.Fatalf("uninstall failed: %v %v", removed, err)
	}
	if len(s.List()) != 0 {
		t.Fatal("plugin still listed after uninstall")
	}
	if _, err := os.Stat(storedPath); !os.IsNotExist(err) {
		t.Fatal("stored package file survived uninstall")
	}
}

// TestCallPluginFailsClosed covers the service-level gate: only enabled,
// instantiated plugins accept dispatch requests.
func TestCallPluginFailsClosed(t *testing.T) {
	archivePath := packExamplePlugin(t)
	s, err := newPluginServiceAt(filepath.Join(t.TempDir(), "plugins.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.InstallPackage(archivePath, PluginPackageInstallOptions{Enable: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetEnabled("hello-lemonssh", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CallPlugin("hello-lemonssh", "ping", ""); !errors.Is(err, ErrPluginDisabled) {
		t.Fatalf("disabled plugin must not dispatch: %v", err)
	}
	if err := s.SetEnabled("hello-lemonssh", true); err != nil {
		t.Fatal(err)
	}
	result, err := s.CallPlugin("hello-lemonssh", "ping", "")
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || !strings.Contains(string(result.Result), `"pong":true`) {
		t.Fatalf("re-enabled plugin must dispatch: %+v", result)
	}
}

// TestExamplePluginResumesAfterRestartOfService covers restoreEnabledPackages:
// an enabled package is re-instantiated when the service starts from disk.
func TestExamplePluginResumesAfterRestartOfService(t *testing.T) {
	archivePath := packExamplePlugin(t)
	inventory := filepath.Join(t.TempDir(), "plugins.json")
	s, err := newPluginServiceAt(inventory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.InstallPackage(archivePath, PluginPackageInstallOptions{Enable: true}); err != nil {
		t.Fatal(err)
	}
	resumed, err := newPluginServiceAt(inventory)
	if err != nil {
		t.Fatal(err)
	}
	records := resumed.List()
	if len(records) != 1 || records[0].State != pluginstore.StateEnabled {
		t.Fatalf("enabled example plugin did not survive a service restart: %+v", records)
	}
}
