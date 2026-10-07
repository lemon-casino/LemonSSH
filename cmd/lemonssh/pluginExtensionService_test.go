package main

// Tests for the plugin extension data plane (pluginExtensionService.go /
// pluginExtensionConnection.go). The WASM transport is replaced with a fake
// dispatch that answers providers.list and provider.invoke for one fake
// plugin; the broker/registry/store/broker paths stay the real ones so the
// fail-closed gating is exercised end to end.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lemon-casino/lemonssh/internal/app/terminaluse"
	"github.com/lemon-casino/lemonssh/internal/plugin/providers"
	pluginstore "github.com/lemon-casino/lemonssh/internal/plugin/store"
	"github.com/lemon-casino/lemonssh/internal/plugin/wasm"
)

const fakeExtensionPluginID = "fake-ext"

// fakeCredentialsProvider is an injectable credentials.Provider for tests.
type fakeCredentialsProvider struct {
	mu      sync.Mutex
	sealed  map[string][]byte // plaintext(round-tripped) keyed by envelope
	counter int
}

func (f *fakeCredentialsProvider) Name() string { return "fake" }
func (f *fakeCredentialsProvider) Available() bool {
	return true
}
func (f *fakeCredentialsProvider) Seal(plaintext []byte, purpose string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counter++
	f.sealed[fmt.Sprintf("%d", f.counter)] = append([]byte(nil), plaintext...)
	return json.Marshal(map[string]string{"id": fmt.Sprintf("%d", f.counter), "purpose": purpose})
}
func (f *fakeCredentialsProvider) Open(envelope []byte, purpose string) ([]byte, error) {
	var decoded map[string]string
	if err := json.Unmarshal(envelope, &decoded); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	value, ok := f.sealed[decoded["id"]]
	if !ok {
		return nil, fmt.Errorf("unknown envelope")
	}
	return append([]byte(nil), value...), nil
}

// extensionHarness installs one enabled fake plugin with provider grants and
// wires fake dispatches for both the registry enumeration and the data plane.
type extensionHarness struct {
	service *PluginService

	mu      sync.Mutex
	invokes []recordedInvoke
	events  []extensionEvent

	syncAccount map[string]any
	outputQueue [][]byte
	authResult  map[string]any
	recordBatch map[string]any
	closed      *pluginCloseRecord
}

// providerInvokePayload is the provider.invoke envelope the fake plugin
// receives (mirrors the wire shape from pluginExtensionService.go).
type providerInvokePayload struct {
	ProviderID string         `json:"providerId"`
	Kind       string         `json:"kind"`
	Operation  string         `json:"operation"`
	RequestID  string         `json:"requestId"`
	Payload    map[string]any `json:"payload"`
}

type recordedInvoke struct {
	PluginID  string
	Method    string
	Payload   map[string]any
	Operation string
	Kind      string
	PayloadOp map[string]any
}

type extensionEvent struct {
	Name    string
	Payload map[string]any
}

type pluginCloseRecord struct {
	mu      sync.Mutex
	reasons []string
}

func (h *extensionHarness) handleDispatch(ctx context.Context, pluginID, method, payloadJSON string) (*wasm.DispatchResult, error) {
	if pluginID != fakeExtensionPluginID {
		return nil, wasm.ErrModuleNotFound
	}
	switch method {
	case providers.MethodList:
		return &wasm.DispatchResult{OK: true, Result: json.RawMessage(`{"providers":[
			{"id":"ext.sync","label":"Ext Sync","kind":"sync"},
			{"id":"ext.conn","label":"Ext Conn","kind":"connection"},
			{"id":"ext.auth","label":"Ext Auth","kind":"authentication"},
			{"id":"ext.import","label":"Ext Import","kind":"importer"}
		]}`)}, nil
	case providers.MethodInvoke:
		var payload providerInvokePayload
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return &wasm.DispatchResult{OK: false, Error: &wasm.DispatchError{Code: "invalid_argument", Message: err.Error()}}, nil
		}
		h.mu.Lock()
		h.invokes = append(h.invokes, recordedInvoke{
			PluginID: pluginID, Method: method,
			Operation: payload.Operation, Kind: payload.Kind,
			PayloadOp: payload.Payload,
		})
		h.mu.Unlock()
		return h.answerInvoke(providerInvokePayload{
			ProviderID: payload.ProviderID, Kind: payload.Kind, Operation: payload.Operation,
			RequestID: payload.RequestID, Payload: payload.Payload,
		}), nil
	default:
		return &wasm.DispatchResult{OK: false, Error: &wasm.DispatchError{Code: "not_found", Message: method}}, nil
	}
}

func (h *extensionHarness) answerInvoke(payload providerInvokePayload) *wasm.DispatchResult {
	ok := func(result any) *wasm.DispatchResult {
		encoded, _ := json.Marshal(result)
		return &wasm.DispatchResult{OK: true, Result: encoded}
	}
	switch payload.Operation {
	case opSyncConnect:
		h.mu.Lock()
		account := map[string]any{"id": "acct-1", "email": "ext@example.com"}
		if h.syncAccount != nil {
			account = h.syncAccount
		}
		h.mu.Unlock()
		return ok(map[string]any{"account": account})
	case opSyncGetAccount:
		return ok(map[string]any{"account": nil})
	case opSyncGetCapabilities:
		return ok(map[string]any{"revisions": true, "conditionalWrites": true, "atomicReplacement": false})
	case opSyncReadObject:
		streamed := payload.Payload["streamed"] == true
		if streamed {
			h.mu.Lock()
			total := 0
			for _, chunk := range h.outputQueue {
				total += len(chunk)
			}
			h.mu.Unlock()
			return ok(map[string]any{"found": true, "byteLength": total, "streamed": true, "revision": "r1"})
		}
		data := base64.StdEncoding.EncodeToString([]byte("inline-object"))
		return ok(map[string]any{"found": true, "byteLength": 13, "encoding": "base64", "data": data, "revision": "r0"})
	case opSyncReadChunk:
		h.mu.Lock()
		var chunk []byte
		if len(h.outputQueue) > 0 {
			chunk = h.outputQueue[0]
			h.outputQueue = h.outputQueue[1:]
		}
		done := len(h.outputQueue) == 0
		h.mu.Unlock()
		return ok(map[string]any{"encoding": "base64", "data": base64.StdEncoding.EncodeToString(chunk), "done": done})
	case opSyncWriteObject:
		return ok(map[string]any{"created": true, "revision": "w0"})
	case opSyncWriteBegin:
		return ok(map[string]any{"windowBytes": 64})
	case opSyncWriteChunk:
		return ok(map[string]any{"accepted": 64})
	case opSyncWriteCommit:
		return ok(map[string]any{"created": true, "revision": "w1"})
	case opSyncDeleteObject:
		return ok(map[string]any{"deleted": true})
	case opConnOpen:
		return ok(map[string]any{"connectionId": "conn-1", "status": "connecting"})
	case opConnReadOutput:
		h.mu.Lock()
		var chunk []byte
		if len(h.outputQueue) > 0 {
			chunk = h.outputQueue[0]
			h.outputQueue = h.outputQueue[1:]
		}
		closed := h.closed != nil && len(h.outputQueue) == 0
		h.mu.Unlock()
		result := map[string]any{"encoding": "base64", "data": base64.StdEncoding.EncodeToString(chunk)}
		if closed {
			result["closed"] = true
			result["reason"] = "exited"
		}
		return ok(result)
	case opConnWriteInput, opConnResize, opConnSignal, opConnClose:
		return ok(map[string]any{"accepted": true})
	case opConnStatus:
		return ok(map[string]any{"status": "connected"})
	case opImportDetect:
		return ok(map[string]any{"confidence": 0.9, "format": "csv"})
	case opImportParseBegin:
		return ok(map[string]any{"windowBytes": 128})
	case opImportParseChunk:
		return ok(map[string]any{"accepted": 128})
	case opImportParseFinish:
		return ok(map[string]any{"parsed": 2, "warnings": 1, "errors": 0})
	case opImportParseRecord:
		h.mu.Lock()
		batched := h.recordBatch
		h.recordBatch = nil
		h.mu.Unlock()
		return ok(batched)
	case opImportParseAbort:
		return ok(map[string]any{"aborted": true})
	case opAuthBegin, opAuthRespond:
		h.mu.Lock()
		next := h.authResult
		h.authResult = nil
		h.mu.Unlock()
		if next != nil {
			return ok(next)
		}
		return ok(map[string]any{"status": "authenticated"})
	default:
		return &wasm.DispatchResult{OK: false, Error: &wasm.DispatchError{Code: "not_found", Message: payload.Operation}}
	}
}

func (h *extensionHarness) invocations() []recordedInvoke {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]recordedInvoke(nil), h.invokes...)
}

func (h *extensionHarness) eventsNamed(name string) []map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	var matched []map[string]any
	for _, event := range h.events {
		if event.Name == name {
			matched = append(matched, event.Payload)
		}
	}
	return matched
}

// newExtensionHarness wires a PluginService with fake dispatch + credentials
// and installs the fake extension plugin with grants for every kind.
func newExtensionHarness(t *testing.T) *extensionHarness {
	t.Helper()
	service := newPluginServiceWithStore(pluginstore.New())
	manifest := `{
		"apiVersion": 2,
		"name": "fake-ext",
		"version": "1.0.0",
		"displayName": "Fake Extension",
		"entrypoint": {"wasm": "plugin.wasm", "sha256": "` + strings.Repeat("ab", 32) + `"},
		"permissions": [
			{"kind": "provider", "resource": "sync", "mode": "read"},
			{"kind": "provider", "resource": "connection", "mode": "read"},
			{"kind": "provider", "resource": "authentication", "mode": "read"},
			{"kind": "provider", "resource": "importer", "mode": "read"}
		]
	}`
	if _, err := service.store.Install(fakeExtensionPluginID, "1.0.0", strings.Repeat("cd", 32), json.RawMessage(manifest)); err != nil {
		t.Fatalf("install fake plugin: %v", err)
	}
	if err := service.store.SetState(fakeExtensionPluginID, pluginstore.StateEnabled); err != nil {
		t.Fatalf("enable fake plugin: %v", err)
	}
	harness := &extensionHarness{}
	// The registry enumerates declarations through the same fake dispatch.
	service.providers = providers.NewRegistry(service.store, service.broker, harness.handleDispatch)
	service.ext.dispatch = harness.handleDispatch
	service.ext.credentials = &fakeCredentialsProvider{sealed: map[string][]byte{}}
	service.setPluginEventEmitter(func(name string, payload any) {
		harness.mu.Lock()
		defer harness.mu.Unlock()
		asMap, _ := payload.(map[string]any)
		harness.events = append(harness.events, extensionEvent{Name: name, Payload: asMap})
	})
	for _, kind := range []string{"sync", "connection", "authentication", "importer"} {
		if err := service.GrantPermission(fakeExtensionPluginID, "provider", kind, "read", "session"); err != nil {
			t.Fatalf("grant provider %s: %v", kind, err)
		}
	}
	service.ext.service = service
	harness.service = service
	return harness
}

func TestExtensionProvidersEnumerateThroughRegistry(t *testing.T) {
	harness := newExtensionHarness(t)
	listed, err := harness.service.ExtensionProviders("sync", "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Provider.ID != "ext.sync" || listed[0].Provider.Kind != "sync" {
		t.Fatalf("sync provider not enumerated through the fake dispatch: %+v", listed)
	}
}

func TestPluginSyncConnectResolvesSecretRefAndReturnsAccount(t *testing.T) {
	harness := newExtensionHarness(t)
	if _, err := harness.service.PluginSyncPutSecret(struct {
		ProviderID string `json:"providerId"`
		Key        string `json:"key"`
		Value      string `json:"value"`
	}{ProviderID: "ext.sync", Key: "token", Value: "s3cret"}); err != nil {
		t.Fatalf("put secret: %v", err)
	}
	result, err := harness.service.PluginSyncConnect(PluginSyncConnectRequest{
		RequestID:     "req-1",
		ProviderID:    "ext.sync",
		Configuration: map[string]any{"region": "eu"},
		Credential:    map[string]any{"kind": "secret", "id": "ext.sync", "key": "token"},
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if result.Account["id"] != "acct-1" {
		t.Fatalf("unexpected account: %+v", result.Account)
	}
	// The plugin received the resolved secret value, not the opaque ref.
	for _, invocation := range harness.invocations() {
		if invocation.Operation != opSyncConnect {
			continue
		}
		credential, _ := invocation.PayloadOp["credential"].(map[string]any)
		if credential == nil || credential["value"] != "s3cret" {
			t.Fatalf("connect dispatch must inline the resolved secret: %+v", invocation.PayloadOp)
		}
	}
	// Fail-closed: an unknown provider never dispatches.
	if _, err := harness.service.PluginSyncConnect(PluginSyncConnectRequest{ProviderID: "nope.sync"}); err == nil {
		t.Fatalf("unknown provider must fail closed")
	}
}

func TestPluginSyncInlineReadAndChunkedRead(t *testing.T) {
	harness := newExtensionHarness(t)
	read, err := harness.service.PluginSyncReadObject(PluginSyncReadObjectRequest{
		RequestID: "read-1", ProviderID: "ext.sync", Key: "payload.enc",
	})
	if err != nil {
		t.Fatalf("inline read: %v", err)
	}
	if !read.Found || read.Data == "" || read.ByteLength != 13 {
		t.Fatalf("unexpected inline read: %+v", read)
	}
	raw, err := base64.StdEncoding.DecodeString(read.Data)
	if err != nil || string(raw) != "inline-object" {
		t.Fatalf("inline payload round-trip failed: %q %v", read.Data, err)
	}

	// Streamed read: the transfer is remembered and pulled via readChunk.
	harness.mu.Lock()
	harness.outputQueue = [][]byte{[]byte("chunk-one-"), []byte("chunk-two")}
	harness.mu.Unlock()
	streamed, err := harness.service.PluginSyncReadObject(PluginSyncReadObjectRequest{
		RequestID: "read-2", ProviderID: "ext.sync", Key: "big.enc", PreferStream: true,
	})
	if err != nil {
		t.Fatalf("streamed read: %v", err)
	}
	if !streamed.Streamed || streamed.TransferID != "read-2" || streamed.ByteLength != 19 {
		t.Fatalf("unexpected streamed header: %+v", streamed)
	}
	first, err := harness.service.PluginSyncReadChunk(PluginSyncReadChunkRequest{RequestID: "read-2", TransferID: "read-2"})
	if err != nil {
		t.Fatalf("readChunk 1: %v", err)
	}
	second, err := harness.service.PluginSyncReadChunk(PluginSyncReadChunkRequest{RequestID: "read-2", TransferID: "read-2"})
	if err != nil {
		t.Fatalf("readChunk 2: %v", err)
	}
	if !second.Done {
		t.Fatalf("second chunk must finish the transfer: %+v", second)
	}
	firstBytes, err := base64.StdEncoding.DecodeString(first.Chunk)
	if err != nil {
		t.Fatalf("chunk 1 decode: %v", err)
	}
	secondBytes, err := base64.StdEncoding.DecodeString(second.Chunk)
	if err != nil {
		t.Fatalf("chunk 2 decode: %v", err)
	}
	if string(firstBytes)+string(secondBytes) != "chunk-one-chunk-two" {
		t.Fatalf("chunk reassembly mismatch: %q %q", firstBytes, secondBytes)
	}
	// The finished transfer is dropped: a further chunk call fails closed.
	if _, err := harness.service.PluginSyncReadChunk(PluginSyncReadChunkRequest{RequestID: "read-2", TransferID: "read-2"}); err == nil {
		t.Fatalf("finished transfer must be dropped")
	}
}

func TestPluginSyncChunkedWriteRegistry(t *testing.T) {
	harness := newExtensionHarness(t)
	begin, err := harness.service.PluginSyncWriteBegin(PluginSyncWriteBeginRequest{
		RequestID: "write-1", ProviderID: "ext.sync", Key: "big.enc", ByteLength: 100,
	})
	if err != nil {
		t.Fatalf("writeBegin: %v", err)
	}
	if begin.TransferID != "write-1" || begin.WindowBytes != 64 {
		t.Fatalf("unexpected begin: %+v", begin)
	}
	if _, err := harness.service.PluginSyncWriteChunk(PluginSyncWriteChunkRequest{
		RequestID: "write-1", TransferID: "write-1", Sequence: 0,
		Chunk: base64.StdEncoding.EncodeToString([]byte("0123456789")),
	}); err != nil {
		t.Fatalf("writeChunk: %v", err)
	}
	commit, err := harness.service.PluginSyncWriteCommit(PluginSyncWriteCommitRequest{RequestID: "write-1", TransferID: "write-1"})
	if err != nil {
		t.Fatalf("writeCommit: %v", err)
	}
	if !commit.Created || commit.Revision != "w1" {
		t.Fatalf("unexpected commit: %+v", commit)
	}
	// Committed transfers are dropped.
	if _, err := harness.service.PluginSyncWriteChunk(PluginSyncWriteChunkRequest{
		RequestID: "write-1", TransferID: "write-1", Sequence: 1, Chunk: "AAAA",
	}); err == nil {
		t.Fatalf("committed transfer must be dropped")
	}
}

// fakeSessionSink records the plugin-session seam interactions.
type fakeSessionSink struct {
	mu         sync.Mutex
	registered map[string]terminaluse.PluginSessionHooks
	published  map[string][]byte
	closed     map[string]string
	closing    map[string]bool
}

func (f *fakeSessionSink) RegisterPluginSession(sessionID, label string, hooks terminaluse.PluginSessionHooks) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.registered[sessionID] = hooks
	return nil
}
func (f *fakeSessionSink) PublishPluginOutput(sessionID string, data []byte) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closing[sessionID] {
		// A closing terminaluse session refuses publishes (term.closing).
		return false
	}
	f.published[sessionID] = append(f.published[sessionID], data...)
	return true
}
func (f *fakeSessionSink) PluginSessionClosed(sessionID, reason string) {
	f.mu.Lock()
	_, seen := f.closed[sessionID]
	if !seen {
		f.closed[sessionID] = reason
	}
	f.closing[sessionID] = true
	hooks := f.registered[sessionID]
	f.mu.Unlock()
	if seen {
		return
	}
	// terminaluse invokes the plugin close hook from the session close path;
	// run it synchronously here so the re-entry chain
	// (PluginSessionClosed -> hooks.Close -> teardownConnection) is exercised.
	if hooks.Close != nil {
		hooks.Close(reason)
	}
}
func (f *fakeSessionSink) CloseSession(sessionID string) error {
	f.mu.Lock()
	f.closing[sessionID] = true
	hooks := f.registered[sessionID]
	f.mu.Unlock()
	if _, seen := f.recordClose(sessionID, "closed"); !seen {
		if hooks.Close != nil {
			hooks.Close("closed")
		}
	}
	return nil
}

// recordClose records the first close reason for a session and reports
// whether one was already recorded.
func (f *fakeSessionSink) recordClose(sessionID, reason string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, seen := f.closed[sessionID]; !seen {
		f.closed[sessionID] = reason
		return reason, false
	}
	return f.closed[sessionID], true
}

func TestPluginConnectionLifecycle(t *testing.T) {
	harness := newExtensionHarness(t)
	sink := &fakeSessionSink{
		registered: map[string]terminaluse.PluginSessionHooks{},
		published:  map[string][]byte{},
		closed:     map[string]string{},
		closing:    map[string]bool{},
	}
	if err := harness.service.setPluginSessionSink(sink); err != nil {
		t.Fatal(err)
	}
	harness.mu.Lock()
	harness.outputQueue = [][]byte{[]byte("banner\r\n")}
	harness.mu.Unlock()

	started, err := harness.service.StartPluginConnection(PluginConnectionStartRequest{
		RequestID:     "conn-req-1",
		SessionID:     "session-1",
		ProviderID:    "ext.conn",
		Configuration: map[string]any{"host": "example"},
		Columns:       80,
		Rows:          24,
	})
	if err != nil {
		t.Fatalf("start plugin connection: %v", err)
	}
	if started.SessionID != "session-1" || started.Status != "connecting" {
		t.Fatalf("unexpected start result: %+v", started)
	}

	// The poller delivers plugin output through the session data plane.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sink.mu.Lock()
		published := string(sink.published["session-1"])
		sink.mu.Unlock()
		if strings.Contains(published, "banner") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	sink.mu.Lock()
	published := string(sink.published["session-1"])
	sink.mu.Unlock()
	if !strings.Contains(published, "banner\r\n") {
		t.Fatalf("plugin output never reached the session data plane: %q", published)
	}

	// Renderer stdin reaches the plugin as base64 writeInput.
	if err := harness.service.WritePluginConnection("session-1", base64.StdEncoding.EncodeToString([]byte("ls\r\n"))); err != nil {
		t.Fatalf("writePluginConnection: %v", err)
	}
	foundWrite := false
	for _, invocation := range harness.invocations() {
		if invocation.Operation == opConnWriteInput {
			raw, _ := base64.StdEncoding.DecodeString(invocation.PayloadOp["data"].(string))
			if string(raw) == "ls\r\n" {
				foundWrite = true
			}
		}
	}
	if !foundWrite {
		t.Fatalf("writeInput never reached the plugin")
	}

	// Control: signal + status.
	if _, err := harness.service.ControlPluginConnection("session-1", "signal", map[string]any{"signal": "interrupt"}); err != nil {
		t.Fatalf("signal: %v", err)
	}
	status, err := harness.service.ControlPluginConnection("session-1", "getStatus", nil)
	if err != nil || status["status"] != "connected" {
		t.Fatalf("getStatus: %+v %v", status, err)
	}

	// Plugin-initiated close: the session ends with ordinary exit tracking
	// and the closed event fires.
	harness.mu.Lock()
	harness.closed = &pluginCloseRecord{}
	harness.mu.Unlock()
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sink.mu.Lock()
		_, done := sink.closed["session-1"]
		sink.mu.Unlock()
		if done {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	sink.mu.Lock()
	reason, closedByPlugin := sink.closed["session-1"]
	sink.mu.Unlock()
	if !closedByPlugin {
		t.Fatalf("plugin close never ended the session")
	}
	if reason != "exited" {
		t.Fatalf("unexpected close reason %q", reason)
	}
	closedEvents := harness.eventsNamed(eventConnectionClosed)
	if len(closedEvents) != 1 || closedEvents[0]["sessionId"] != "session-1" {
		t.Fatalf("expected exactly one connection-closed event, got %+v", closedEvents)
	}
}

func TestPluginAuthenticationChallengeRoundTrip(t *testing.T) {
	harness := newExtensionHarness(t)
	challenge := map[string]any{
		"status": "challenge",
		"challenge": map[string]any{
			"id": "chal-1", "kind": "password", "title": "Password",
		},
	}
	harness.mu.Lock()
	harness.authResult = challenge
	harness.mu.Unlock()

	begin, err := harness.service.InvokePluginExtensionProvider(InvokeExtensionProviderRequest{
		RequestID:  "auth-req-1",
		ProviderID: "ext.auth",
		Kind:       "authentication",
		Operation:  opAuthBegin,
		Payload:    map[string]any{"connectionProviderId": "ext.conn"},
	})
	if err != nil {
		t.Fatalf("authentication begin: %v", err)
	}
	if asMap, _ := begin.(map[string]any); asMap["status"] != "challenge" {
		t.Fatalf("expected challenge, got %+v", begin)
	}
	events := harness.eventsNamed(eventAuthenticationChal)
	if len(events) != 1 || events[0]["challengeRequestId"] != "auth-req-1" {
		t.Fatalf("challenge event not mirrored: %+v", events)
	}

	// The renderer answers; the plugin answers "authenticated" and the
	// pending challenge is cleared.
	if err := harness.service.RespondPluginAuthenticationChallenge(struct {
		RequestID          string `json:"requestId"`
		ChallengeRequestID string `json:"challengeRequestId"`
		ChallengeID        string `json:"challengeId"`
		Response           any    `json:"response,omitempty"`
		Cancelled          bool   `json:"cancelled,omitempty"`
	}{RequestID: "auth-req-1", ChallengeRequestID: "auth-req-1", ChallengeID: "chal-1", Response: "hunter2"}); err != nil {
		t.Fatalf("respond: %v", err)
	}
	harness.service.ext.mu.Lock()
	_, stillPending := harness.service.ext.challenges["auth-req-1"]
	harness.service.ext.mu.Unlock()
	if stillPending {
		t.Fatalf("authenticated challenge must be deregistered")
	}

	// Unknown challenge ids fail closed.
	if err := harness.service.RespondPluginAuthenticationChallenge(struct {
		RequestID          string `json:"requestId"`
		ChallengeRequestID string `json:"challengeRequestId"`
		ChallengeID        string `json:"challengeId"`
		Response           any    `json:"response,omitempty"`
		Cancelled          bool   `json:"cancelled,omitempty"`
	}{ChallengeRequestID: "missing"}); err == nil {
		t.Fatalf("unknown challenge must fail")
	}
}

func TestPluginImporterParseFlow(t *testing.T) {
	harness := newExtensionHarness(t)
	staged := &stagedImporterFile{token: "tok-1", path: "unused", name: "hosts.csv"}
	content := []byte("host,user\na,root\nb,admin\n")
	tmp := t.TempDir()
	path := tmp + "/hosts.csv"
	if err := writeExtensionTestFile(path, content); err != nil {
		t.Fatal(err)
	}
	staged.path = path
	harness.service.ext.mu.Lock()
	harness.service.ext.importFiles["tok-1"] = staged
	harness.service.ext.mu.Unlock()
	harness.mu.Lock()
	harness.recordBatch = map[string]any{
		"records": []any{
			map[string]any{"type": "progress", "completed": 1, "total": 2},
			map[string]any{"type": "draft", "draft": map[string]any{"kind": "host", "value": map[string]any{"label": "a", "hostname": "a"}}},
			map[string]any{"type": "warning", "message": "dup"},
			map[string]any{"type": "draft", "draft": map[string]any{"kind": "host", "value": map[string]any{"label": "b", "hostname": "b"}}},
		},
		"done": true,
	}
	harness.mu.Unlock()

	parsed, err := harness.service.ParsePluginImporterFile(PluginImporterParseRequest{
		RequestID: "import-1", ProviderID: "ext.import", SelectionToken: "tok-1",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed["providerId"] != "ext.import" {
		t.Fatalf("unexpected parse result provider: %+v", parsed)
	}
	records, _ := parsed["records"].([]any)
	if len(records) != 3 { // progress record is mirrored as an event, not returned
		t.Fatalf("expected 3 returned records, got %d", len(records))
	}
	progressEvents := harness.eventsNamed(eventImporterProgress)
	if len(progressEvents) != 1 || progressEvents[0]["requestId"] != "import-1" {
		t.Fatalf("progress event not mirrored: %+v", progressEvents)
	}

	// Releasing the staged file invalidates the token.
	released, err := harness.service.ReleasePluginImporterFile("tok-1")
	if err != nil || !released {
		t.Fatalf("release: %v %v", released, err)
	}
	if _, err := harness.service.ParsePluginImporterFile(PluginImporterParseRequest{
		ProviderID: "ext.import", SelectionToken: "tok-1",
	}); err == nil {
		t.Fatalf("released token must fail closed")
	}
}

func TestCancelPluginExtensionRequestMarksCancelled(t *testing.T) {
	harness := newExtensionHarness(t)
	if _, err := harness.service.CancelPluginExtensionRequest("req-x"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if !harness.service.ext.isCancelled("req-x") {
		t.Fatalf("cancelled request id not remembered")
	}
}

// startLifecycleConnection boots one plugin connection on the given session
// id with the shared fake provider surface.
func startLifecycleConnection(t *testing.T, harness *extensionHarness, sink *fakeSessionSink, requestID, sessionID string, queue [][]byte) *PluginConnectionStartResult {
	t.Helper()
	if err := harness.service.setPluginSessionSink(sink); err != nil {
		t.Fatal(err)
	}
	harness.mu.Lock()
	harness.outputQueue = queue
	harness.mu.Unlock()
	started, err := harness.service.StartPluginConnection(PluginConnectionStartRequest{
		RequestID:     requestID,
		SessionID:     sessionID,
		ProviderID:    "ext.conn",
		Configuration: map[string]any{"host": "example"},
		Columns:       80,
		Rows:          24,
	})
	if err != nil {
		t.Fatalf("start plugin connection: %v", err)
	}
	return started
}

// TestPluginConnectionRendererCloseWhileStreaming covers the close window the
// first implementation deadlocked in: the renderer closes (or cancels) while
// the poller is mid-flight, publishOutput refuses the in-loop chunk, and the
// loop re-enters the close path. The synchronous Wails close call must
// always return; the fake sink re-enters hooks.Close like terminaluse does.
func TestPluginConnectionRendererCloseWhileStreaming(t *testing.T) {
	harness := newExtensionHarness(t)
	sink := &fakeSessionSink{
		registered: map[string]terminaluse.PluginSessionHooks{},
		published:  map[string][]byte{},
		closed:     map[string]string{},
		closing:    map[string]bool{},
	}
	startLifecycleConnection(t, harness, sink, "conn-req-2", "session-2", [][]byte{
		[]byte("chunk-one\r\n"), []byte("chunk-two\r\n"), []byte("chunk-three\r\n"),
	})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sink.mu.Lock()
		published := len(sink.published["session-2"])
		sink.mu.Unlock()
		if published > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := harness.service.ControlPluginConnection("session-2", "close", nil); err != nil {
			t.Errorf("close: %v", err)
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("renderer close deadlocked while the output loop was streaming")
	}
	// The session must be reclaimed through the ordinary close path.
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sink.mu.Lock()
		_, closed := sink.closed["session-2"]
		sink.mu.Unlock()
		if closed {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	sink.mu.Lock()
	_, closedByHook := sink.closed["session-2"]
	harness.service.ext.mu.Lock()
	_, stillTracked := harness.service.ext.connections["session-2"]
	harness.service.ext.mu.Unlock()
	sink.mu.Unlock()
	if !closedByHook {
		t.Fatalf("close never reached the session sink")
	}
	if stillTracked {
		t.Fatalf("host connection entry leaked after close")
	}
	// Renderer-initiated closes emit no plugin:connection-closed event.
	if events := harness.eventsNamed(eventConnectionClosed); len(events) != 0 {
		t.Fatalf("renderer close must not emit connection-closed, got %+v", events)
	}
}

// TestPluginConnectionCancelReclaimsSession covers CancelPluginExtensionRequest
// against a live connection: the poller stops, host state and the terminal
// session are reclaimed, and the cancel returns promptly.
func TestPluginConnectionCancelReclaimsSession(t *testing.T) {
	harness := newExtensionHarness(t)
	sink := &fakeSessionSink{
		registered: map[string]terminaluse.PluginSessionHooks{},
		published:  map[string][]byte{},
		closed:     map[string]string{},
		closing:    map[string]bool{},
	}
	startLifecycleConnection(t, harness, sink, "conn-req-3", "session-3", [][]byte{[]byte("early output\r\n")})

	reclaimed := make(chan struct{})
	go func() {
		defer close(reclaimed)
		handled, err := harness.service.CancelPluginExtensionRequest("conn-req-3")
		if err != nil || !handled {
			t.Errorf("cancel: handled=%v err=%v", handled, err)
		}
	}()
	select {
	case <-reclaimed:
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not return promptly")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sink.mu.Lock()
		_, closed := sink.closed["session-3"]
		harness.service.ext.mu.Lock()
		_, tracked := harness.service.ext.connections["session-3"]
		harness.service.ext.mu.Unlock()
		sink.mu.Unlock()
		if closed && !tracked {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	sink.mu.Lock()
	_, closed := sink.closed["session-3"]
	sink.mu.Unlock()
	harness.service.ext.mu.Lock()
	_, tracked := harness.service.ext.connections["session-3"]
	harness.service.ext.mu.Unlock()
	t.Fatalf("cancel did not reclaim the connection: closed=%v tracked=%v", closed, tracked)
}

func writeExtensionTestFile(path string, content []byte) error {
	return os.WriteFile(path, content, 0o600)
}
