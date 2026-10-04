package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lemon-casino/lemonssh/internal/plugin/providers"
)

// installEnabledHelloExample packages and enables the shipped hello-lemonssh
// example through the same Wails service methods the frontend calls.
func installEnabledHelloExample(t *testing.T) (*PluginService, func()) {
	t.Helper()
	archivePath := packExamplePlugin(t)
	inventory := filepath.Join(t.TempDir(), "plugins.json")
	s, err := newPluginServiceAt(inventory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.InstallPackage(archivePath, PluginPackageInstallOptions{Enable: true}); err != nil {
		t.Fatalf("example plugin package rejected: %v", err)
	}
	cleanup := func() {
		_, _ = s.Uninstall("hello-lemonssh")
	}
	return s, cleanup
}

// TestPluginCommandExecutesOverWasmDispatch is the command-chain contract:
// the bridge's executePluginCommand falls through to CallPlugin with method
// "command.execute" (infrastructure/runtime/wails/pluginBridge.ts), so the
// shipped example must answer its declared hello.ping command there instead
// of surfacing "no active native command handler".
func TestPluginCommandExecutesOverWasmDispatch(t *testing.T) {
	s, cleanup := installEnabledHelloExample(t)
	defer cleanup()

	result, err := s.CallPlugin("hello-lemonssh", "command.execute", `{"command":"hello.ping","args":null,"context":{"source":"quick-switcher"}}`)
	if err != nil {
		t.Fatalf("command.execute dispatch failed: %v", err)
	}
	if !result.OK {
		t.Fatalf("plugin rejected command.execute: %+v", result)
	}
	if !strings.Contains(string(result.Result), `"command":"hello.ping"`) || !strings.Contains(string(result.Result), `"handled":true`) {
		t.Fatalf("unexpected command.execute result: %s", result.Result)
	}

	// A command the plugin does not own surfaces its structured in-band error
	// (which the renderer renders as "Plugin command ... failed: ...").
	other, err := s.CallPlugin("hello-lemonssh", "command.execute", `{"command":"someone.elses.command"}`)
	if err != nil {
		t.Fatalf("dispatch transport failed: %v", err)
	}
	if other.OK || other.Error == nil || other.Error.Code != "not_found" {
		t.Fatalf("unhandled command must surface the plugin error: %+v", other)
	}
}

// TestTerminalProviderLifecycle is the terminal provider contract: the
// example declares a terminal.theme provider through the providers.list
// dispatch, the host accepts it only while the manifest-derived broker grant
// exists, and provide/invoke flows through provider.invoke.
func TestTerminalProviderLifecycle(t *testing.T) {
	s, cleanup := installEnabledHelloExample(t)
	defer cleanup()

	// Fail-closed: without the grant the example contributes nothing.
	empty, err := s.TerminalProviders("terminal.theme", "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("ungranted provider enumerated: %+v", empty)
	}

	// The trusted host UI grant path (same flow as the runtime permissions).
	if err := s.GrantPermission("hello-lemonssh", "provider", "terminal.theme", "read", "session"); err != nil {
		t.Fatalf("manifest-declared provider permission not grantable: %v", err)
	}

	listed, err := s.TerminalProviders("terminal.theme", "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected exactly one provider declaration, got %+v", listed)
	}
	contribution := listed[0]
	if contribution.PluginID != "hello-lemonssh" || contribution.PluginVersion != "0.4.0" {
		t.Fatalf("contribution identity wrong: %+v", contribution)
	}
	if contribution.Provider.ID != "com.lemonssh.hello.accent" || contribution.Provider.Kind != "terminal.theme" || contribution.Provider.Label != "Hello Accent" {
		t.Fatalf("declaration wrong: %+v", contribution.Provider)
	}

	// Extension surfaces list nothing for a terminal-only plugin.
	extensions, err := s.ExtensionProviders("sync", "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(extensions) != 0 {
		t.Fatalf("terminal plugin leaked into extension kinds: %+v", extensions)
	}

	// Provide fans the request out and the example answers with its accent.
	results, err := s.ProvideTerminal(providers.TerminalRequest{
		RequestID: "terminal-e2e-1",
		Kind:      "terminal.theme",
		Operation: "provideTheme",
		Session: providers.TerminalSessionSnapshot{
			SessionID: "session-e2e", Protocol: "ssh", Status: "connected", CWD: "/tmp",
		},
		Payload:    []byte(`{"reason":"session-state"}`),
		DeadlineMs: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != providers.StatusOK {
		t.Fatalf("unexpected provide results: %+v", results)
	}
	if !strings.Contains(string(results[0].Result), `"cursor":"#34d399"`) {
		t.Fatalf("theme result not delivered: %s", results[0].Result)
	}
	if results[0].PluginID != "hello-lemonssh" || results[0].ProviderID != "com.lemonssh.hello.accent" || results[0].RequestID != "terminal-e2e-1" {
		t.Fatalf("result identity wrong: %+v", results[0])
	}

	// Session events reach contributing plugins.
	deliveries, err := s.PublishTerminalSessionEvent(providers.SessionEvent{
		Type:    "connected",
		Session: providers.TerminalSessionSnapshot{SessionID: "session-e2e", Protocol: "ssh", Status: "connected"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 1 || deliveries[0].PluginID != "hello-lemonssh" || !deliveries[0].Delivered {
		t.Fatalf("session event not delivered: %+v", deliveries)
	}

	// Cancelling an unknown request reports false; cancelling while a dispatch
	// is in flight is covered by the registry unit tests.
	if cancelled, err := s.CancelTerminalRequest("terminal-never-seen"); err != nil || cancelled {
		t.Fatalf("unknown cancel must report false: %v %v", cancelled, err)
	}

	// Disabling the plugin revokes the grant and stops the enumeration.
	if err := s.SetEnabled("hello-lemonssh", false); err != nil {
		t.Fatal(err)
	}
	afterDisable, err := s.TerminalProviders("terminal.theme", "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(afterDisable) != 0 {
		t.Fatalf("disabled plugin still contributes providers: %+v", afterDisable)
	}
	if _, err := s.CallPlugin("hello-lemonssh", "command.execute", `{"command":"hello.ping"}`); !errors.Is(err, ErrPluginDisabled) {
		t.Fatalf("disabled plugin must not execute commands: %v", err)
	}
}

// TestTerminalProvidersRejectInterceptorAndUnknownKinds pins the registry
// boundary: privileged interceptor kinds and unknown kinds are never
// registrable through the service surface.
func TestTerminalProvidersRejectInterceptorAndUnknownKinds(t *testing.T) {
	s, cleanup := installEnabledHelloExample(t)
	defer cleanup()
	if err := s.GrantPermission("hello-lemonssh", "provider", "terminal.theme", "read", "session"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"terminal.interceptor.input", "terminal.interceptor.output", "not-a-kind"} {
		listed, err := s.TerminalProviders(kind, "en")
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", kind, err)
		}
		if len(listed) != 0 {
			t.Fatalf("%s: kind must not be registrable: %+v", kind, listed)
		}
	}
	// Extension enumeration rejects unknown kinds too.
	if extensions, err := s.ExtensionProviders("terminal.theme", "en"); err != nil || len(extensions) != 0 {
		t.Fatalf("terminal kind must not be an extension kind: %+v %v", extensions, err)
	}
}

// TestTerminalProviderProvideDeadlineCap verifies the registry clamps the
// requested deadline to the host's dispatch cap instead of trusting it.
func TestTerminalProviderProvideDeadlineCap(t *testing.T) {
	s, cleanup := installEnabledHelloExample(t)
	defer cleanup()
	if err := s.GrantPermission("hello-lemonssh", "provider", "terminal.theme", "read", "session"); err != nil {
		t.Fatal(err)
	}
	results, err := s.ProvideTerminal(providers.TerminalRequest{
		RequestID:  "terminal-e2e-2",
		Kind:       "terminal.theme",
		Operation:  "provideTheme",
		Session:    providers.TerminalSessionSnapshot{SessionID: "session-e2e", Protocol: "ssh", Status: "connected"},
		DeadlineMs: 1 << 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != providers.StatusOK {
		t.Fatalf("capped deadline must still serve the request: %+v", results)
	}
}
