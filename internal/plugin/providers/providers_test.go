package providers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/binaricat/lemonssh/internal/plugin/permissions"
	pluginstore "github.com/binaricat/lemonssh/internal/plugin/store"
	"github.com/binaricat/lemonssh/internal/plugin/wasm"
)

// fakeDispatch records dispatch traffic and answers from a per-plugin table.
type fakeDispatch struct {
	mu       sync.Mutex
	calls    []string // "pluginID|method|payload"
	handlers map[string]func(method, payload string) (*wasm.DispatchResult, error)
}

func (f *fakeDispatch) dispatch(_ context.Context, pluginID, method, payload string) (*wasm.DispatchResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, pluginID+"|"+method+"|"+payload)
	handler := f.handlers[pluginID]
	f.mu.Unlock()
	if handler == nil {
		return nil, wasm.ErrModuleNotFound
	}
	return handler(method, payload)
}

func (f *fakeDispatch) callCount(pluginID, method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, call := range f.calls {
		parts := strings.SplitN(call, "|", 3)
		if len(parts) >= 2 && parts[0] == pluginID && parts[1] == method {
			count++
		}
	}
	return count
}

func okResult(result string) *wasm.DispatchResult {
	return &wasm.DispatchResult{OK: true, Result: json.RawMessage(result)}
}

func failResult(code, message string) *wasm.DispatchResult {
	return &wasm.DispatchResult{OK: false, Error: &wasm.DispatchError{Code: code, Message: message}}
}

// fixtureStore installs one enabled plugin whose manifest declares the given
// provider resources.
func fixtureStore(t *testing.T, pluginID, version string, resources ...string) *pluginstore.Store {
	t.Helper()
	permissionsJSON := make([]string, 0, len(resources))
	for _, resource := range resources {
		permissionsJSON = append(permissionsJSON, `{"kind":"provider","resource":"`+resource+`","mode":"read"}`)
	}
	manifest := `{"apiVersion":2,"name":"` + pluginID + `","version":"` + version + `","displayName":"Demo Plugin","entrypoint":{"wasm":"main.wasm","sha256":"` + strings.Repeat("a", 64) + `"},"permissions":[` + strings.Join(permissionsJSON, ",") + `]}`
	store := pluginstore.New()
	if _, err := store.Install(pluginID, version, strings.Repeat("b", 64), json.RawMessage(manifest)); err != nil {
		t.Fatal(err)
	}
	if err := store.SetState(pluginID, pluginstore.StateEnabled); err != nil {
		t.Fatal(err)
	}
	return store
}

func helloHandlers() map[string]func(method, payload string) (*wasm.DispatchResult, error) {
	return map[string]func(method, payload string) (*wasm.DispatchResult, error){
		"demo": func(method, _ string) (*wasm.DispatchResult, error) {
			switch method {
			case MethodList:
				return okResult(`{"providers":[
					{"id":"com.demo.accent","label":"Demo Accent","kind":"terminal.theme"},
					{"id":"com.demo.rich","label":{"en":"Demo Rich","zh-CN":"演示"},"kind":"terminal.theme","capabilities":["colors"],"description":"Resolved later"},
					{"id":"com.demo.undeclared","label":"Nope","kind":"terminal.completion"},
					{"id":"COM.demo.bad","label":"Bad","kind":"terminal.theme"},
					{"id":"com.demo.interceptor","label":"Nope","kind":"terminal.interceptor.input"},
					{"id":"com.demo.accent","label":"Dup","kind":"terminal.theme"}
				]}`), nil
			case MethodInvoke:
				return okResult(`{"colors":{"cursor":"#34d399"}}`), nil
			default:
				return okResult(`{}`), nil
			}
		},
	}
}

func TestListRequiresGrantAndFiltersDeclarations(t *testing.T) {
	store := fixtureStore(t, "demo", "1.0.0", "terminal.theme")
	broker := permissions.NewBroker(nil)
	fake := &fakeDispatch{handlers: helloHandlers()}
	registry := NewRegistry(store, broker, fake.dispatch)

	// Fail-closed: without the broker grant nothing is enumerated and the
	// plugin is not even asked.
	providers := registry.List(context.Background(), "terminal.theme", "en")
	if len(providers) != 0 {
		t.Fatalf("ungranted plugin listed: %+v", providers)
	}
	if fake.callCount("demo", MethodList) != 0 {
		t.Fatal("ungranted plugin was dispatched")
	}
	if _, err := broker.Grant("demo", permissionKey(KindTerminalTheme), permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}

	providers = registry.List(context.Background(), "terminal.theme", "en")
	if len(providers) != 2 {
		t.Fatalf("expected 2 accepted declarations, got %+v", providers)
	}
	first, second := providers[0].Provider, providers[1].Provider
	if first.ID != "com.demo.accent" || first.Label != "Demo Accent" || first.Kind != KindTerminalTheme {
		t.Fatalf("declaration wrong: %+v", first)
	}
	// The undeclared kind, the uppercase id, the interceptor kind and the
	// duplicate id must all have been dropped.
	if second.ID != "com.demo.rich" {
		t.Fatalf("expected the rich declaration second: %+v", second)
	}
	if second.Label != "Demo Rich" {
		t.Fatalf("localized label not resolved to en: %+v", second)
	}

	// The contribution carries the plugin identity.
	if providers[0].PluginID != "demo" || providers[0].PluginVersion != "1.0.0" || providers[0].PluginDisplayName != "Demo Plugin" {
		t.Fatalf("contribution identity wrong: %+v", providers[0])
	}
}

func TestListResolvesPreferredLocale(t *testing.T) {
	store := fixtureStore(t, "demo", "1.0.0", "terminal.theme")
	broker := permissions.NewBroker(nil)
	if _, err := broker.Grant("demo", permissionKey(KindTerminalTheme), permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}
	fake := &fakeDispatch{handlers: helloHandlers()}
	registry := NewRegistry(store, broker, fake.dispatch)

	providers := registry.List(context.Background(), "terminal.theme", "zh-CN")
	if len(providers) != 2 || providers[1].Provider.Label != "演示" {
		t.Fatalf("zh-CN label not preferred: %+v", providers)
	}
}

func TestListCachesPerVersionAndGrantSet(t *testing.T) {
	store := fixtureStore(t, "demo", "1.0.0", "terminal.theme")
	broker := permissions.NewBroker(nil)
	fake := &fakeDispatch{handlers: helloHandlers()}
	registry := NewRegistry(store, broker, fake.dispatch)
	if _, err := broker.Grant("demo", permissionKey(KindTerminalTheme), permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if providers := registry.List(context.Background(), "terminal.theme", "en"); len(providers) != 2 {
			t.Fatalf("iteration %d listed %+v", i, providers)
		}
	}
	if got := fake.callCount("demo", MethodList); got != 1 {
		t.Fatalf("expected 1 dispatch for repeated lists, got %d", got)
	}

	// A version bump re-queries.
	if err := store.Uninstall("demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install("demo", "1.0.1", strings.Repeat("c", 64), json.RawMessage(`{"apiVersion":2,"name":"demo","version":"1.0.1","displayName":"Demo Plugin","entrypoint":{"wasm":"main.wasm","sha256":"`+strings.Repeat("a", 64)+`"},"permissions":[{"kind":"provider","resource":"terminal.theme","mode":"read"}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := store.SetState("demo", pluginstore.StateEnabled); err != nil {
		t.Fatal(err)
	}
	if providers := registry.List(context.Background(), "terminal.theme", "en"); len(providers) != 2 {
		t.Fatalf("new version listed %+v", providers)
	}
	if got := fake.callCount("demo", MethodList); got != 2 {
		t.Fatalf("expected a re-dispatch after version bump, got %d", got)
	}

	// Disabled plugins contribute nothing and drop their cache entry.
	if err := store.SetState("demo", pluginstore.StateDisabled); err != nil {
		t.Fatal(err)
	}
	if providers := registry.List(context.Background(), "terminal.theme", "en"); len(providers) != 0 {
		t.Fatalf("disabled plugin listed: %+v", providers)
	}
	if got := fake.callCount("demo", MethodList); got != 2 {
		t.Fatalf("disabled plugin was dispatched, calls=%d", got)
	}
}

func TestExtensionKindsEnumerateThroughSameRegistry(t *testing.T) {
	store := fixtureStore(t, "sync-demo", "2.0.0", KindSync)
	broker := permissions.NewBroker(nil)
	fake := &fakeDispatch{handlers: map[string]func(method, payload string) (*wasm.DispatchResult, error){
		"sync-demo": func(method, _ string) (*wasm.DispatchResult, error) {
			if method == MethodList {
				return okResult(`{"providers":[{"id":"com.sync.demo","label":"Sync Demo","kind":"sync"},{"id":"com.sync.bad","label":"Bad","kind":"terminal.theme"}]}`), nil
			}
			return okResult(`{}`), nil
		},
	}}
	registry := NewRegistry(store, broker, fake.dispatch)
	if _, err := broker.Grant("sync-demo", permissionKey(KindSync), permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}

	providers := registry.List(context.Background(), KindSync, "en")
	if len(providers) != 1 || providers[0].Provider.ID != "com.sync.demo" || providers[0].Provider.Kind != KindSync {
		t.Fatalf("sync provider not enumerated: %+v", providers)
	}
	// terminal kinds are not granted for this plugin, so the terminal filter is empty
	if providers := registry.List(context.Background(), KindTerminalTheme, "en"); len(providers) != 0 {
		t.Fatalf("ungranted kind leaked: %+v", providers)
	}
	// Unknown kinds list nothing instead of erroring.
	if providers := registry.List(context.Background(), "terminal.interceptor.input", "en"); len(providers) != 0 {
		t.Fatalf("interceptor kind accepted: %+v", providers)
	}
}

func providerRequest(kind, operation string) TerminalRequest {
	return TerminalRequest{
		RequestID: "terminal-test-1",
		Kind:      kind,
		Operation: operation,
		Session: TerminalSessionSnapshot{
			SessionID: "session-1", Protocol: "ssh", Status: "connected",
			CWD: "/tmp", Title: "demo",
		},
	}
}

func TestProvideMapsSuccessAndPluginErrors(t *testing.T) {
	store := fixtureStore(t, "demo", "1.0.0", "terminal.theme")
	broker := permissions.NewBroker(nil)
	if _, err := broker.Grant("demo", permissionKey(KindTerminalTheme), permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}
	fake := &fakeDispatch{handlers: helloHandlers()}
	registry := NewRegistry(store, broker, fake.dispatch)

	results := registry.Provide(context.Background(), providerRequest(KindTerminalTheme, "provideTheme"))
	if len(results) != 2 {
		t.Fatalf("expected results for 2 providers, got %+v", results)
	}
	ok := results[0]
	if ok.Status != StatusOK || ok.PluginID != "demo" || ok.ProviderID != "com.demo.accent" || ok.RequestID != "terminal-test-1" {
		t.Fatalf("ok result wrong: %+v", ok)
	}
	if !strings.Contains(string(ok.Result), `"cursor":"#34d399"`) {
		t.Fatalf("plugin result not passed through: %s", ok.Result)
	}
	// The invoke payload carries the canonical envelope.
	fake.mu.Lock()
	var invokePayload string
	for _, call := range fake.calls {
		if strings.Contains(call, "|"+MethodInvoke+"|") {
			invokePayload = strings.SplitN(call, "|", 3)[2]
			break // first provider (com.demo.accent)
		}
	}
	fake.mu.Unlock()
	var sent struct {
		ProviderID string                  `json:"providerId"`
		Kind       string                  `json:"kind"`
		Operation  string                  `json:"operation"`
		RequestID  string                  `json:"requestId"`
		Session    TerminalSessionSnapshot `json:"session"`
	}
	if err := json.Unmarshal([]byte(invokePayload), &sent); err != nil {
		t.Fatal(err)
	}
	if sent.ProviderID != "com.demo.accent" || sent.Kind != KindTerminalTheme || sent.Operation != "provideTheme" || sent.RequestID != "terminal-test-1" || sent.Session.SessionID != "session-1" {
		t.Fatalf("invoke envelope wrong: %+v", sent)
	}

	// A plugin-declared failure becomes a structured failed result with the
	// plugin code preserved in data.
	fake.handlers = map[string]func(method, payload string) (*wasm.DispatchResult, error){
		"demo": func(method, _ string) (*wasm.DispatchResult, error) {
			if method == MethodList {
				return okResult(`{"providers":[{"id":"com.demo.accent","label":"Demo Accent","kind":"terminal.theme"}]}`), nil
			}
			return failResult("unsupported", "operation not supported"), nil
		},
	}
	registry.Invalidate("demo")
	results = registry.Provide(context.Background(), providerRequest(KindTerminalTheme, "provideTheme"))
	if len(results) != 1 || results[0].Status != StatusFailed {
		t.Fatalf("expected failed result, got %+v", results)
	}
	if results[0].Error == nil || results[0].Error.Code != CodeUnknown || results[0].Error.Message != "operation not supported" {
		t.Fatalf("plugin error not mapped: %+v", results[0].Error)
	}
	if !strings.Contains(string(results[0].Error.Data), "unsupported") {
		t.Fatalf("plugin code not preserved in data: %s", results[0].Error.Data)
	}
}

func TestProvideMapsTransportFailures(t *testing.T) {
	store := fixtureStore(t, "demo", "1.0.0", "terminal.theme")
	broker := permissions.NewBroker(nil)
	if _, err := broker.Grant("demo", permissionKey(KindTerminalTheme), permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}
	fake := &fakeDispatch{handlers: map[string]func(method, payload string) (*wasm.DispatchResult, error){
		"demo": func(method, _ string) (*wasm.DispatchResult, error) {
			if method == MethodList {
				return okResult(`{"providers":[{"id":"com.demo.accent","label":"Demo Accent","kind":"terminal.theme"}]}`), nil
			}
			return nil, wasm.ErrDispatchTimeout
		},
	}}
	registry := NewRegistry(store, broker, fake.dispatch)
	results := registry.Provide(context.Background(), providerRequest(KindTerminalTheme, "provideTheme"))
	if len(results) != 1 || results[0].Status != StatusFailed {
		t.Fatalf("expected failed result, got %+v", results)
	}
	if results[0].Error == nil || results[0].Error.Code != CodeDeadlineExceeded {
		t.Fatalf("timeout must map to the deadline wire code: %+v", results[0].Error)
	}
}

func TestProvideValidatesRequest(t *testing.T) {
	store := fixtureStore(t, "demo", "1.0.0", "terminal.theme")
	registry := NewRegistry(store, permissions.NewBroker(nil), nil)

	request := providerRequest("terminal.interceptor.input", "intercept")
	results := registry.Provide(context.Background(), request)
	if len(results) != 1 || results[0].Status != StatusFailed || results[0].Error.Code != CodeUnknown {
		t.Fatalf("interceptor kinds must fail validation: %+v", results)
	}
	request = providerRequest(KindTerminalTheme, "provideTheme")
	request.RequestID = ""
	results = registry.Provide(context.Background(), request)
	if len(results) != 1 || results[0].Status != StatusFailed {
		t.Fatalf("missing request id must fail: %+v", results)
	}
	request.Session.SessionID = ""
	results = registry.Provide(context.Background(), request)
	if len(results) != 1 || results[0].Status != StatusFailed {
		t.Fatalf("missing session id must fail: %+v", results)
	}
}

func TestCancelAbortsInFlightDispatch(t *testing.T) {
	store := fixtureStore(t, "demo", "1.0.0", "terminal.theme")
	broker := permissions.NewBroker(nil)
	if _, err := broker.Grant("demo", permissionKey(KindTerminalTheme), permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	fake := &fakeDispatch{handlers: map[string]func(method, payload string) (*wasm.DispatchResult, error){
		"demo": func(method, _ string) (*wasm.DispatchResult, error) {
			if method == MethodList {
				return okResult(`{"providers":[{"id":"com.demo.accent","label":"Demo Accent","kind":"terminal.theme"}]}`), nil
			}
			close(started)
			<-release
			return okResult(`{}`), nil
		},
	}}
	registry := NewRegistry(store, broker, fake.dispatch)

	type provideOutcome struct {
		results []TerminalResult
	}
	outcome := make(chan provideOutcome, 1)
	go func() {
		outcome <- provideOutcome{registry.Provide(context.Background(), providerRequest(KindTerminalTheme, "provideTheme"))}
	}()
	<-started
	if !registry.Cancel("terminal-test-1") {
		t.Fatal("cancel reported an unknown active request")
	}
	close(release)
	results := (<-outcome).results
	if len(results) != 1 || results[0].Status != StatusCancelled {
		t.Fatalf("cancelled dispatch must surface as cancelled: %+v", results)
	}
	// Cancelling an unknown request reports false.
	if registry.Cancel("terminal-never-existed") {
		t.Fatal("unknown cancel must report false")
	}
}

func TestProvideHonorsPreferredProviderIDs(t *testing.T) {
	store := fixtureStore(t, "demo", "1.0.0", "terminal.theme")
	broker := permissions.NewBroker(nil)
	if _, err := broker.Grant("demo", permissionKey(KindTerminalTheme), permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}
	fake := &fakeDispatch{handlers: helloHandlers()}
	registry := NewRegistry(store, broker, fake.dispatch)

	request := providerRequest(KindTerminalTheme, "provideTheme")
	request.PreferredProviderIDs = []string{"com.demo.rich"}
	results := registry.Provide(context.Background(), request)
	if len(results) != 1 || results[0].ProviderID != "com.demo.rich" {
		t.Fatalf("preferred filter not applied: %+v", results)
	}
}

func TestPublishSessionEventDeliversToContributorsOnly(t *testing.T) {
	store := fixtureStore(t, "demo", "1.0.0", "terminal.theme")
	ungranted := fixtureStore(t, "silent", "1.0.0", "terminal.theme")
	broker := permissions.NewBroker(nil)
	if _, err := broker.Grant("demo", permissionKey(KindTerminalTheme), permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}
	fake := &fakeDispatch{handlers: helloHandlers()}
	_ = ungranted
	registry := NewRegistry(store, broker, fake.dispatch)

	exitCode := 0
	deliveries := registry.PublishSessionEvent(context.Background(), SessionEvent{
		Type:     "connected",
		Session:  TerminalSessionSnapshot{SessionID: "session-1", Protocol: "ssh", Status: "connected"},
		ExitCode: &exitCode,
	})
	if len(deliveries) != 1 || deliveries[0].PluginID != "demo" || !deliveries[0].Delivered {
		t.Fatalf("unexpected deliveries: %+v", deliveries)
	}
	if got := fake.callCount("demo", MethodSessionEvent); got != 1 {
		t.Fatalf("session event not dispatched: %d", got)
	}
	// The ungranted plugin must never be contacted.
	if got := fake.callCount("silent", MethodSessionEvent); got != 0 {
		t.Fatalf("ungranted plugin received a session event: %d", got)
	}
	// Invalid events are dropped.
	if deliveries := registry.PublishSessionEvent(context.Background(), SessionEvent{}); len(deliveries) != 0 {
		t.Fatalf("empty event type delivered: %+v", deliveries)
	}
}

func TestGrantRevocationStopsEnumeration(t *testing.T) {
	store := fixtureStore(t, "demo", "1.0.0", "terminal.theme")
	broker := permissions.NewBroker(nil)
	if _, err := broker.Grant("demo", permissionKey(KindTerminalTheme), permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}
	fake := &fakeDispatch{handlers: helloHandlers()}
	registry := NewRegistry(store, broker, fake.dispatch)
	if providers := registry.List(context.Background(), "terminal.theme", "en"); len(providers) != 2 {
		t.Fatalf("granted plugin listed nothing: %+v", providers)
	}
	broker.RevokeAll("demo")
	if providers := registry.List(context.Background(), "terminal.theme", "en"); len(providers) != 0 {
		t.Fatalf("revoked plugin still listed: %+v", providers)
	}
}

func TestDeclarationValidation(t *testing.T) {
	valid := Declaration{ID: "com.demo.ok", Label: "OK", Kind: KindTerminalTheme}
	if err := ValidateDeclaration(valid); err != nil {
		t.Fatalf("valid declaration rejected: %v", err)
	}
	cases := []struct {
		name  string
		mutop func(*Declaration)
	}{
		{"bad id", func(d *Declaration) { d.ID = "BAD ID" }},
		{"unknown kind", func(d *Declaration) { d.Kind = "terminal.interceptor.output" }},
		{"empty label", func(d *Declaration) { d.Label = "  " }},
		{"long label", func(d *Declaration) { d.Label = strings.Repeat("x", 257) }},
		{"too many capabilities", func(d *Declaration) { d.Capabilities = make([]string, 33) }},
		{"empty capability", func(d *Declaration) { d.Capabilities = []string{""} }},
		{"huge schema", func(d *Declaration) { d.ConfigurationSchema = json.RawMessage(`"` + strings.Repeat("x", 17<<10) + `"`) }},
	}
	for _, testCase := range cases {
		declaration := valid
		testCase.mutop(&declaration)
		if err := ValidateDeclaration(declaration); err == nil {
			t.Fatalf("%s: declaration accepted", testCase.name)
		} else if !errors.Is(err, ErrInvalidDeclaration) {
			t.Fatalf("%s: wrong error: %v", testCase.name, err)
		}
	}
}

func TestBrokenPluginDoesNotBlankRegistry(t *testing.T) {
	store := fixtureStore(t, "demo", "1.0.0", "terminal.theme")
	if _, err := store.Install("broken", "1.0.0", strings.Repeat("d", 64), json.RawMessage(`{"apiVersion":2,"name":"broken","version":"1.0.0","displayName":"Broken","entrypoint":{"wasm":"main.wasm","sha256":"`+strings.Repeat("a", 64)+`"},"permissions":[{"kind":"provider","resource":"terminal.theme","mode":"read"}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := store.SetState("broken", pluginstore.StateEnabled); err != nil {
		t.Fatal(err)
	}
	broker := permissions.NewBroker(nil)
	for _, pluginID := range []string{"demo", "broken"} {
		if _, err := broker.Grant(pluginID, permissionKey(KindTerminalTheme), permissions.LifetimeSession, 0); err != nil {
			t.Fatal(err)
		}
	}
	// "broken" has no dispatch handler -> its transport fails while "demo"
	// (helloHandlers) keeps answering.
	fake := &fakeDispatch{handlers: helloHandlers()}
	registry := NewRegistry(store, broker, fake.dispatch)
	providers := registry.List(context.Background(), "terminal.theme", "en")
	if len(providers) != 2 {
		t.Fatalf("the healthy plugin's providers must survive a broken peer: %+v", providers)
	}
}

func TestSessionGrantConsumptionIsRespected(t *testing.T) {
	// A `once` grant is consumed by the first enumeration like every other
	// broker Check; the plugin then disappears from the registry.
	store := fixtureStore(t, "demo", "1.0.0", "terminal.theme")
	broker := permissions.NewBroker(nil)
	if _, err := broker.Grant("demo", permissionKey(KindTerminalTheme), permissions.LifetimeOnce, 0); err != nil {
		t.Fatal(err)
	}
	fake := &fakeDispatch{handlers: helloHandlers()}
	registry := NewRegistry(store, broker, fake.dispatch)
	if providers := registry.List(context.Background(), "terminal.theme", "en"); len(providers) != 2 {
		t.Fatalf("once grant must authorize the first enumeration: %+v", providers)
	}
	if providers := registry.List(context.Background(), "terminal.theme", "en"); len(providers) != 0 {
		t.Fatalf("consumed once grant must stop enumeration: %+v", providers)
	}
	_ = time.Now
}
