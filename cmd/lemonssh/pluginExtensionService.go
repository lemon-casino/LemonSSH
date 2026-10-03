// The plugin extension host serves the extension provider data plane
// (connection / sync / importer / authentication) on top of the same
// broker-gated registry and lemonssh-wasm-abi v1 dispatch channel the
// terminal provider registry uses (internal/plugin/providers).
//
// Security model (docs/plugin-platform/terminal-providers.md,
// sync-providers.md):
//   - Every operation resolves the providerId through the fail-closed
//     registry first: a provider that is disabled, ungranted or undeclared
//     never reaches a dispatch.
//   - Sync object bytes cross the WASM boundary as base64 chunks bounded by
//     the 1 MiB dispatch envelope; plaintext vault payloads never enter this
//     path (callers hand over already-encrypted objects).
//   - Sync connect secrets are sealed with the platform credential provider
//     and stored inside the owning plugin's record; a plugin only ever
//     receives its OWN resolved secret, never the vault master key.
//   - Plugin connections run as first class terminal sessions via the
//     terminaluse plugin-session seam; the plugin receives immutable session
//     metadata and byte streams, never renderer or host objects.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/binaricat/lemonssh/internal/platform/credentials"
	"github.com/binaricat/lemonssh/internal/plugin/providers"
	pluginstore "github.com/binaricat/lemonssh/internal/plugin/store"
	"github.com/binaricat/lemonssh/internal/plugin/wasm"
)

// Wire operations of the extension provider protocol (provider.invoke
// payload.operation). Names mirror the contract payload types
// (SyncConnectPayload, ConnectionOpenPayload, ImporterDetectPayload,
// AuthenticationBeginPayload).
const (
	// sync provider operations.
	opSyncConnect         = "connect"
	opSyncDisconnect      = "disconnect"
	opSyncGetAccount      = "getAccount"
	opSyncGetCapabilities = "getCapabilities"
	opSyncReadObject      = "readObject"
	opSyncReadChunk       = "readChunk"
	opSyncWriteObject     = "writeObject"
	opSyncWriteBegin      = "writeBegin"
	opSyncWriteChunk      = "writeChunk"
	opSyncWriteCommit     = "writeCommit"
	opSyncDeleteObject    = "deleteObject"

	// connection provider operations.
	opConnOpen       = "open"
	opConnWriteInput = "writeInput"
	opConnReadOutput = "readOutput"
	opConnResize     = "resize"
	opConnSignal     = "signal"
	opConnStatus     = "status"
	opConnClose      = "close"

	// importer provider operations.
	opImportDetect      = "detect"
	opImportParseBegin  = "parseBegin"
	opImportParseChunk  = "parseChunk"
	opImportParseFinish = "parseFinish"
	opImportParseRecord = "parseRecords"
	opImportParseAbort  = "parseAbort"

	// authentication provider operations.
	opAuthBegin   = "begin"
	opAuthRespond = "challengeResponse"

	// Protocol label for extension session snapshots (immutable metadata
	// only; no renderer or host objects cross this boundary).
	extensionSessionProtocol = "extension"
)

const (
	// Base64 chunks must fit the 1 MiB WASM dispatch envelope with JSON
	// overhead to spare (wasm.MaxRequestBytes / MaxResponseBytes).
	maxSyncChunkBytes  = 192 << 10
	maxConnChunkBytes  = 64 << 10
	maxImportChunkData = 192 << 10
	// The detect sample handed to the plugin at file-selection time.
	maxImporterSampleBytes = 64 << 10
	// Contract ImporterLimits (maxInputBytes / maxRecords).
	maxImporterInputBytes = 64 << 20
	maxImporterRecords    = 10000
	maxSecretPlaintext    = 96 << 10
	// Reserved settings key prefix for sealed sync secrets inside the
	// owning plugin's store record (never a declared setting, so the
	// declarative settings surface never exposes it).
	syncSecretKeyPrefix = "sync-secret/"
	// Credential-provider purpose shared by all sealed plugin sync secrets.
	syncSecretPurpose = "plugin-sync-secret"
	// Renderer event names (forwardService/windowLifecycleService style).
	eventConnectionData     = "plugin:connection-data"
	eventConnectionClosed   = "plugin:connection-closed"
	eventAuthenticationChal = "plugin:authentication-challenge"
	eventImporterProgress   = "plugin:importer-progress"
	idleConnPollInterval    = 25 * time.Millisecond
	closeDispatchDeadline   = 3 * time.Second
	cancelledRequestTTL     = 10 * time.Minute
	maxPendingChallenges    = 64
	maxActiveConnections    = 64
	maxStagedImporterFiles  = 16
)

// pluginExtensionHost owns the extension provider data plane state.
type pluginExtensionHost struct {
	service *PluginService

	// dispatch sends one WASM dispatch request; defaults to the service's
	// runtime transport and is replaceable in tests.
	dispatch func(ctx context.Context, pluginID, method, payloadJSON string) (*wasm.DispatchResult, error)

	emit func(name string, payload any)

	// Session sink hosts plugin connections as terminal sessions.
	sessions PluginConnectionSessionSink

	mu          sync.Mutex
	active      map[string]*activeExtensionOp // requestId -> dispatch cancel
	cancelled   map[string]time.Time
	connections map[string]*pluginConnection // renderer session id -> conn
	challenges  map[string]*pendingChallenge // challengeRequestId -> pending
	importFiles map[string]*stagedImporterFile
	secretStash map[string]map[string][]byte // pluginID -> key -> previous plaintext
	transfers   map[string]*syncTransfer     // transferId -> live chunked transfer

	credentials credentials.Provider

	// dataEvents gates the per-chunk plugin:connection-data event; the
	// data plane streams terminal output regardless.
	dataEvents atomic.Bool
}

// activeExtensionOp wraps one in-flight dispatch so cancellation entries are
// comparable pointer values.
type activeExtensionOp struct {
	cancel context.CancelFunc
}

type pluginConnection struct {
	sessionID  string
	pluginID   string
	providerID string
	// start is immutable after construction (reconnect reuses its
	// configuration); the mutable correlation fields live under mu.
	start  PluginConnectionStartRequest
	cancel context.CancelFunc
	// done is owned by the output loop and closes exactly when that loop
	// exits; no close path ever waits on it (a close path may BE the loop).
	done chan struct{}

	mu sync.Mutex
	// requestID and connectionID are correlation fields mutated by
	// reconnect and read by the poller/cancel paths — always via the
	// accessors below.
	requestID    string
	connectionID string
	closed       bool
}

// beginClose transitions the connection to closed exactly once and reports
// whether this caller won the transition. Losers must return without
// touching host state: the winner owns teardown and event emission.
func (c *pluginConnection) beginClose() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	c.closed = true
	return true
}

func (c *pluginConnection) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *pluginConnection) connectionTarget() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectionID
}

func (c *pluginConnection) setConnectionTarget(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connectionID = id
}

func (c *pluginConnection) connectionRequestID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requestID
}

func (c *pluginConnection) setConnectionRequestID(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requestID = id
}

type pendingChallenge struct {
	pluginID   string
	providerID string
	requestID  string
}

type stagedImporterFile struct {
	token  string
	path   string
	name   string
	sample []byte
	media  string
}

// syncTransfer tracks one chunked sync transfer (streamed read or
// begin/chunk/commit write). The renderer's chunk calls carry only
// requestId+transferId, so the host resolves the owning provider here —
// and a stale or spoofed transfer id resolves to nothing.
type syncTransfer struct {
	pluginID   string
	providerID string
	requestID  string
	createdAt  time.Time
}

func (h *pluginExtensionHost) rememberTransfer(transferID, pluginID, providerID, requestID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sweepTransfersLocked(time.Now())
	h.transfers[transferID] = &syncTransfer{
		pluginID:   pluginID,
		providerID: providerID,
		requestID:  requestID,
		createdAt:  time.Now(),
	}
}

// transferProvider resolves the owning contribution of a chunked transfer.
func (h *pluginExtensionHost) transferProvider(transferID string) (pluginID, providerID string, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	transfer, hit := h.transfers[transferID]
	if !hit || transfer == nil {
		return "", "", false
	}
	return transfer.pluginID, transfer.providerID, true
}

func (h *pluginExtensionHost) dropTransfer(transferID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.transfers, transferID)
}

func (h *pluginExtensionHost) sweepTransfersLocked(now time.Time) {
	for id, transfer := range h.transfers {
		if transfer == nil || now.Sub(transfer.createdAt) > cancelledRequestTTL {
			delete(h.transfers, id)
		}
	}
}

func newPluginExtensionHost(service *PluginService) *pluginExtensionHost {
	host := &pluginExtensionHost{
		service:     service,
		active:      make(map[string]*activeExtensionOp),
		cancelled:   make(map[string]time.Time),
		connections: make(map[string]*pluginConnection),
		challenges:  make(map[string]*pendingChallenge),
		importFiles: make(map[string]*stagedImporterFile),
		secretStash: make(map[string]map[string][]byte),
		transfers:   make(map[string]*syncTransfer),
		credentials: credentials.NewOSProvider(),
	}
	host.dispatch = service.dispatchPluginCtx
	return host
}

func (h *pluginExtensionHost) emitEvent(name string, payload any) {
	if h.emit != nil {
		h.emit(name, payload)
	}
}

// setEventEmitter wires the renderer event bus (main.go).
func (s *PluginService) setPluginEventEmitter(emit func(name string, payload any)) {
	s.ext.emit = emit
}

// setPluginSessionSink wires the terminal session sink (main.go, after the
// TerminalService exists). Unexported: it is process-internal wiring, never
// a renderer-callable binding.
func (s *PluginService) setPluginSessionSink(sink PluginConnectionSessionSink) error {
	if sink == nil {
		return errors.New("plugin session sink is required")
	}
	s.ext.sessions = sink
	return nil
}

// SetPluginConnectionDataEvents gates the per-chunk plugin:connection-data
// renderer event. Terminal output always flows through the session data
// plane; the event exists for non-terminal consumers and stays off unless a
// listener subscribed.
func (s *PluginService) SetPluginConnectionDataEvents(active bool) error {
	s.ext.dataEvents.Store(active)
	return nil
}

// ---------------------------------------------------------------------------
// Dispatch plumbing
// ---------------------------------------------------------------------------

func mintPluginRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("ext-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func clampDispatchDeadline(deadlineMs int) time.Duration {
	if deadlineMs <= 0 || deadlineMs > int(wasm.DefaultDispatchLimit/time.Millisecond) {
		return wasm.DefaultDispatchLimit
	}
	return time.Duration(deadlineMs) * time.Millisecond
}

// dispatchPluginCtx is the ctx-aware twin of CallPlugin's checks.
func (s *PluginService) dispatchPluginCtx(ctx context.Context, pluginID, method, payloadJSON string) (*wasm.DispatchResult, error) {
	record, ok := s.store.Get(pluginID)
	if !ok {
		return nil, pluginstore.ErrNotInstalled
	}
	if record.State != pluginstore.StateEnabled {
		return nil, ErrPluginDisabled
	}
	if s.runtime == nil {
		return nil, wasm.ErrModuleNotFound
	}
	return s.runtime.Dispatch(ctx, pluginID, method, payloadJSON)
}

// markCancelled remembers a cancelled request id so late operations with the
// same id fail fast instead of re-entering a plugin.
func (h *pluginExtensionHost) markCancelled(requestID string) {
	if requestID == "" {
		return
	}
	h.mu.Lock()
	if op, ok := h.active[requestID]; ok {
		delete(h.active, requestID)
		if op != nil && op.cancel != nil {
			op.cancel()
		}
	}
	now := time.Now()
	for id, at := range h.cancelled {
		if now.Sub(at) > cancelledRequestTTL {
			delete(h.cancelled, id)
		}
	}
	h.cancelled[requestID] = now
	h.mu.Unlock()
}

func (h *pluginExtensionHost) isCancelled(requestID string) bool {
	if requestID == "" {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, hit := h.cancelled[requestID]
	return hit
}

type extensionInvocation struct {
	pluginID   string
	providerID string
	kind       string
	operation  string
	requestID  string
	deadline   time.Duration
	payload    map[string]any
}

// invokeExtension resolves nothing: callers must resolve the contribution
// through the registry first (fail-closed). The request id is registered for
// cancellation for the duration of the dispatch.
func (h *pluginExtensionHost) invokeExtension(ctx context.Context, inv extensionInvocation) (json.RawMessage, error) {
	if h.isCancelled(inv.requestID) {
		return nil, errExtensionCancelled
	}
	callCtx, cancel := context.WithTimeout(ctx, inv.deadline)
	defer cancel()
	op := &activeExtensionOp{cancel: cancel}
	h.mu.Lock()
	h.active[inv.requestID] = op
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		if h.active[inv.requestID] == op {
			delete(h.active, inv.requestID)
		}
		h.mu.Unlock()
	}()

	payload, err := json.Marshal(map[string]any{
		"providerId": inv.providerID,
		"kind":       inv.kind,
		"operation":  inv.operation,
		"requestId":  inv.requestID,
		"session": map[string]any{
			"sessionId": inv.requestID,
			"protocol":  extensionSessionProtocol,
			"status":    "connected",
		},
		"payload":    inv.payload,
		"deadlineMs": int(inv.deadline / time.Millisecond),
	})
	if err != nil {
		return nil, fmt.Errorf("encode provider.invoke payload: %w", err)
	}
	response, err := h.dispatch(callCtx, inv.pluginID, providers.MethodInvoke, string(payload))
	if err != nil {
		if errors.Is(callCtx.Err(), context.Canceled) || h.isCancelled(inv.requestID) {
			return nil, errExtensionCancelled
		}
		if errors.Is(callCtx.Err(), context.DeadlineExceeded) || errors.Is(err, wasm.ErrDispatchTimeout) {
			return nil, fmt.Errorf("plugin provider operation %q timed out", inv.operation)
		}
		return nil, fmt.Errorf("plugin provider transport: %w", err)
	}
	if response == nil {
		return nil, fmt.Errorf("plugin provider returned no result")
	}
	if !response.OK {
		code, message := "unknown", "provider operation failed"
		if response.Error != nil {
			code = response.Error.Code
			message = response.Error.Message
		}
		return nil, fmt.Errorf("plugin provider error %s: %s", code, message)
	}
	return response.Result, nil
}

// resolveExtensionProvider resolves a providerId through the broker-checked
// registry. Unknown or ungranted providers fail closed.
func (s *PluginService) resolveExtensionProvider(kind, providerID string) (*providers.Contribution, error) {
	if providerID == "" {
		return nil, fmt.Errorf("providerId is required")
	}
	if !providers.ExtensionKinds()[kind] {
		return nil, fmt.Errorf("unsupported extension provider kind %q", kind)
	}
	contributions := s.providers.List(context.Background(), kind, "")
	for index := range contributions {
		if contributions[index].Provider.ID == providerID {
			contribution := contributions[index]
			return &contribution, nil
		}
	}
	return nil, fmt.Errorf("plugin %s provider %q is not available", kind, providerID)
}

var errExtensionCancelled = errors.New("plugin extension request was cancelled")

// ---------------------------------------------------------------------------
// Generic extension provider invocation + cancellation
// ---------------------------------------------------------------------------

// InvokeExtensionProviderRequest is the renderer's generic extension provider
// call (LemonSSHBridge.invokePluginExtensionProvider). Authentication begin
// results carrying a challenge are registered and mirrored to the renderer
// through the plugin:authentication-challenge event.
type InvokeExtensionProviderRequest struct {
	RequestID  string         `json:"requestId,omitempty"`
	ProviderID string         `json:"providerId"`
	Kind       string         `json:"kind"`
	Operation  string         `json:"operation"`
	Payload    map[string]any `json:"payload,omitempty"`
	DeadlineMs int            `json:"deadlineMs,omitempty"`
}

// InvokePluginExtensionProvider runs one provider operation of any extension
// kind through the dispatch channel.
func (s *PluginService) InvokePluginExtensionProvider(request InvokeExtensionProviderRequest) (any, error) {
	if strings.TrimSpace(request.Operation) == "" {
		return nil, errors.New("operation is required")
	}
	contribution, err := s.resolveExtensionProvider(request.Kind, request.ProviderID)
	if err != nil {
		return nil, err
	}
	requestID := request.RequestID
	if requestID == "" {
		requestID = mintPluginRequestID()
	}
	payload := request.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	// Authentication begin: the renderer omits operationId — mint one from
	// the request id so challenge responses can be correlated.
	if request.Kind == providers.KindAuthentication && request.Operation == opAuthBegin {
		if _, ok := payload["operationId"]; !ok {
			payload["operationId"] = requestID
		}
	}
	result, err := s.ext.invokeExtension(context.Background(), extensionInvocation{
		pluginID:   contribution.PluginID,
		providerID: contribution.Provider.ID,
		kind:       request.Kind,
		operation:  request.Operation,
		requestID:  requestID,
		deadline:   clampDispatchDeadline(request.DeadlineMs),
		payload:    payload,
	})
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(result, &decoded); err != nil {
		return nil, fmt.Errorf("plugin provider returned invalid JSON: %w", err)
	}
	if request.Kind == providers.KindAuthentication && request.Operation == opAuthBegin {
		s.ext.registerChallengeResult(contribution.PluginID, contribution.Provider.ID, requestID, decoded)
	}
	return decoded, nil
}

// CancelPluginExtensionRequest aborts one in-flight extension request (data
// plane dispatches, pending authentication challenges).
func (s *PluginService) CancelPluginExtensionRequest(requestID string) (bool, error) {
	if requestID == "" {
		return false, nil
	}
	s.ext.markCancelled(requestID)
	// The registry tracks its own provider fan-outs.
	if s.providers.Cancel(requestID) {
		return true, nil
	}
	s.ext.mu.Lock()
	pending, hit := s.ext.challenges[requestID]
	if hit {
		delete(s.ext.challenges, requestID)
	}
	var target *pluginConnection
	for _, conn := range s.ext.connections {
		if conn.connectionRequestID() == requestID {
			target = conn
			break
		}
	}
	s.ext.mu.Unlock()
	if pending != nil {
		s.ext.emitEvent(eventAuthenticationChal, map[string]any{
			"requestId":          pending.requestID,
			"challengeRequestId": requestID,
			"cancelled":          true,
		})
		// Best-effort: tell the plugin the challenge was cancelled.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), closeDispatchDeadline)
			defer cancel()
			_, _ = s.ext.invokeExtension(ctx, extensionInvocation{
				pluginID:   pending.pluginID,
				providerID: pending.providerID,
				kind:       providers.KindAuthentication,
				operation:  opAuthRespond,
				// A fresh transport request id is required: the
				// cancellation marker recorded above exists to fail late
				// data-plane operations and would otherwise short-circuit
				// this very dispatch at the entry check.
				requestID: mintPluginRequestID(),
				deadline:  closeDispatchDeadline,
				payload: map[string]any{
					"operationId": requestID,
					"challengeId": "",
					"cancelled":   true,
				},
			})
		}()
		return true, nil
	}
	if target != nil {
		// Reclaim a cancelled connection: stop the poller, tell the plugin,
		// drop host state and close the terminal session with ordinary exit
		// tracking. Off the caller's stack: the plugin close dispatch is
		// bounded but not free, and Cancel must return promptly.
		go func() {
			s.ext.teardownConnection(target, "cancelled")
			if s.ext.sessions != nil {
				_ = s.ext.sessions.CloseSession(target.sessionID)
			}
		}()
		return true, nil
	}
	return false, nil
}

// ---------------------------------------------------------------------------
// Sync provider data plane
// ---------------------------------------------------------------------------

// resolveSyncSecret inline-resolves a host-stored SecretRef before the
// connect dispatch. The plugin receives only its own secret, never refs it
// cannot resolve (the WASM host imports have no secret channel).
func (h *pluginExtensionHost) resolveSyncSecret(pluginID string, credential map[string]any) (map[string]any, error) {
	if len(credential) == 0 {
		return nil, nil
	}
	switch credential["kind"] {
	case "credential":
		// Renderer-managed credential catalog entry: forwarded verbatim.
		return credential, nil
	case "secret":
		key, _ := credential["key"].(string)
		refID, _ := credential["id"].(string)
		if key == "" {
			return nil, errors.New("secret credential ref requires a key")
		}
		value, err := h.loadSyncSecret(pluginID, key)
		if err != nil {
			return nil, fmt.Errorf("resolve sync secret %q: %w", key, err)
		}
		resolved := map[string]any{"kind": "secret", "id": refID, "key": key, "value": value}
		return resolved, nil
	default:
		return nil, fmt.Errorf("unsupported sync credential kind %q", fmt.Sprint(credential["kind"]))
	}
}

// sealedSecretEnvelope is the at-rest shape of one sealed sync secret inside
// the owning plugin's store record.
type sealedSecretEnvelope struct {
	Version  int    `json:"v"`
	Envelope string `json:"envelope"`
}

func syncSecretStorageKey(key string) string {
	return syncSecretKeyPrefix + key
}

func (h *pluginExtensionHost) loadSyncSecret(pluginID, key string) (string, error) {
	record, ok := h.service.store.Get(pluginID)
	if !ok {
		return "", errors.New("plugin is not installed")
	}
	raw, ok := record.Settings[syncSecretStorageKey(key)]
	if !ok {
		return "", errors.New("secret not found")
	}
	var stored struct {
		Version  int    `json:"v"`
		Envelope string `json:"envelope"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil || stored.Version != 1 || stored.Envelope == "" {
		return "", errors.New("malformed stored secret")
	}
	envelope, err := base64.StdEncoding.DecodeString(stored.Envelope)
	if err != nil {
		return "", errors.New("malformed stored secret envelope")
	}
	plaintext, err := h.credentials.Open(envelope, syncSecretPurpose)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (h *pluginExtensionHost) stashSecret(pluginID, key string, value []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	bucket, ok := h.secretStash[pluginID]
	if !ok {
		bucket = make(map[string][]byte)
		h.secretStash[pluginID] = bucket
	}
	stashed := make([]byte, len(value))
	copy(stashed, value)
	bucket[key] = stashed
}

// PluginSyncPutSecret seals and stores one sync connect secret for the
// provider's owning plugin; the renderer only ever sees the opaque ref.
func (s *PluginService) PluginSyncPutSecret(request struct {
	ProviderID string `json:"providerId"`
	Key        string `json:"key"`
	Value      string `json:"value"`
}) (map[string]any, error) {
	if request.Key == "" || request.ProviderID == "" {
		return nil, errors.New("providerId and key are required")
	}
	if len(request.Value) > maxSecretPlaintext {
		return nil, errors.New("sync secret exceeds the size bound")
	}
	contribution, err := s.resolveExtensionProvider(providers.KindSync, request.ProviderID)
	if err != nil {
		return nil, err
	}
	if !s.ext.credentials.Available() {
		return nil, errors.New("secure credential storage is unavailable")
	}
	previous := ""
	if old, oldErr := s.ext.loadSyncSecret(contribution.PluginID, request.Key); oldErr == nil {
		previous = old
	}
	envelope, err := s.ext.credentials.Seal([]byte(request.Value), syncSecretPurpose)
	if err != nil {
		return nil, fmt.Errorf("seal sync secret: %w", err)
	}
	encoded, err := json.Marshal(sealedSecretEnvelope{Version: 1, Envelope: base64.StdEncoding.EncodeToString(envelope)})
	if err != nil {
		return nil, err
	}
	if err := s.store.SetSetting(contribution.PluginID, syncSecretStorageKey(request.Key), encoded); err != nil {
		return nil, err
	}
	if previous != "" {
		s.ext.stashSecret(contribution.PluginID, request.Key, []byte(previous))
	}
	return map[string]any{
		"kind":    "secret",
		"id":      request.ProviderID,
		"key":     request.Key,
		"created": previous == "",
	}, nil
}

// PluginSyncDeleteSecrets removes stored sync secrets (sign-out cleanup).
func (s *PluginService) PluginSyncDeleteSecrets(request struct {
	ProviderID string   `json:"providerId"`
	Keys       []string `json:"keys,omitempty"`
}) (map[string]any, error) {
	contribution, err := s.resolveExtensionProvider(providers.KindSync, request.ProviderID)
	if err != nil {
		return nil, err
	}
	record, ok := s.store.Get(contribution.PluginID)
	if !ok {
		return map[string]any{"deleted": 0}, nil
	}
	prefix := syncSecretKeyPrefix
	deleted := 0
	for key := range record.Settings {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		suffix := key[len(prefix):]
		if len(request.Keys) > 0 && !containsString(request.Keys, suffix) {
			continue
		}
		if err := s.store.DeleteSetting(contribution.PluginID, key); err == nil {
			deleted++
		}
	}
	return map[string]any{"deleted": deleted}, nil
}

// PluginSyncRestoreSecrets re-applies previously stashed plaintext values
// (captured on overwrite/delete) after a rejected reconnect overwrite, or
// discards the stash. Session-scoped: the stash lives in memory only.
func (s *PluginService) PluginSyncRestoreSecrets(request struct {
	ProviderID string   `json:"providerId"`
	Keys       []string `json:"keys"`
	Discard    bool     `json:"discard,omitempty"`
}) (map[string]any, error) {
	contribution, err := s.resolveExtensionProvider(providers.KindSync, request.ProviderID)
	if err != nil {
		return nil, err
	}
	pluginID := contribution.PluginID
	s.ext.mu.Lock()
	bucket := s.ext.secretStash[pluginID]
	stashed := make(map[string][]byte, len(request.Keys))
	for _, key := range request.Keys {
		if value, ok := bucket[key]; ok {
			stashed[key] = value
		}
	}
	if request.Discard {
		for _, key := range request.Keys {
			delete(bucket, key)
		}
	}
	s.ext.mu.Unlock()
	if request.Discard {
		return map[string]any{"restored": 0, "discarded": len(stashed)}, nil
	}
	restored := 0
	if !s.ext.credentials.Available() {
		return map[string]any{"restored": 0}, nil
	}
	for key, value := range stashed {
		envelope, sealErr := s.ext.credentials.Seal(value, syncSecretPurpose)
		if sealErr != nil {
			continue
		}
		encoded, marshalErr := json.Marshal(sealedSecretEnvelope{Version: 1, Envelope: base64.StdEncoding.EncodeToString(envelope)})
		if marshalErr != nil {
			continue
		}
		if err := s.store.SetSetting(pluginID, syncSecretStorageKey(key), encoded); err == nil {
			restored++
			s.ext.mu.Lock()
			delete(bucket, key)
			s.ext.mu.Unlock()
		}
	}
	return map[string]any{"restored": restored}, nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Sync provider object operations
// ---------------------------------------------------------------------------

type PluginSyncRequestBase struct {
	RequestID  string `json:"requestId,omitempty"`
	ProviderID string `json:"providerId"`
	DeadlineMs int    `json:"deadlineMs,omitempty"`
}

type PluginSyncConnectRequest struct {
	RequestID     string         `json:"requestId,omitempty"`
	ProviderID    string         `json:"providerId"`
	Configuration map[string]any `json:"configuration,omitempty"`
	Credential    map[string]any `json:"credential,omitempty"`
	DeadlineMs    int            `json:"deadlineMs,omitempty"`
}

type PluginSyncConnectResult struct {
	Account map[string]any `json:"account"`
}

// PluginSyncConnect runs the sync connect flow (contract SyncConnectPayload).
func (s *PluginService) PluginSyncConnect(request PluginSyncConnectRequest) (*PluginSyncConnectResult, error) {
	contribution, err := s.resolveExtensionProvider(providers.KindSync, request.ProviderID)
	if err != nil {
		return nil, err
	}
	requestID := orMintRequestID(request.RequestID)
	credential, err := s.ext.resolveSyncSecret(contribution.PluginID, request.Credential)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"configuration": request.Configuration,
		"operationId":   requestID,
	}
	if credential != nil {
		payload["credential"] = credential
	}
	result, err := s.ext.invokeExtension(context.Background(), extensionInvocation{
		pluginID:   contribution.PluginID,
		providerID: contribution.Provider.ID,
		kind:       providers.KindSync,
		operation:  opSyncConnect,
		requestID:  requestID,
		deadline:   clampDispatchDeadline(request.DeadlineMs),
		payload:    payload,
	})
	if err != nil {
		return nil, err
	}
	var decoded struct {
		Account map[string]any `json:"account"`
	}
	if err := json.Unmarshal(result, &decoded); err != nil || decoded.Account == nil {
		return nil, errors.New("plugin sync connect returned no account")
	}
	return &PluginSyncConnectResult{Account: decoded.Account}, nil
}

// PluginSyncDisconnect clears the plugin's connected account.
func (s *PluginService) PluginSyncDisconnect(request PluginSyncRequestBase) (map[string]any, error) {
	contribution, err := s.resolveExtensionProvider(providers.KindSync, request.ProviderID)
	if err != nil {
		return nil, err
	}
	requestID := orMintRequestID(request.RequestID)
	if _, err := s.ext.invokeExtension(context.Background(), extensionInvocation{
		pluginID:   contribution.PluginID,
		providerID: contribution.Provider.ID,
		kind:       providers.KindSync,
		operation:  opSyncDisconnect,
		requestID:  requestID,
		deadline:   clampDispatchDeadline(request.DeadlineMs),
		payload:    map[string]any{"operationId": requestID},
	}); err != nil {
		return nil, err
	}
	return nil, nil
}

// PluginSyncGetAccount returns the plugin's connected account or null.
func (s *PluginService) PluginSyncGetAccount(request PluginSyncRequestBase) (map[string]any, error) {
	contribution, err := s.resolveExtensionProvider(providers.KindSync, request.ProviderID)
	if err != nil {
		return nil, err
	}
	requestID := orMintRequestID(request.RequestID)
	result, err := s.ext.invokeExtension(context.Background(), extensionInvocation{
		pluginID:   contribution.PluginID,
		providerID: contribution.Provider.ID,
		kind:       providers.KindSync,
		operation:  opSyncGetAccount,
		requestID:  requestID,
		deadline:   clampDispatchDeadline(request.DeadlineMs),
		payload:    map[string]any{"operationId": requestID},
	})
	if err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := json.Unmarshal(result, &decoded); err != nil {
		return nil, errors.New("plugin sync getAccount returned invalid JSON")
	}
	if decoded == nil {
		decoded = map[string]any{"account": nil}
	}
	if _, ok := decoded["account"]; !ok {
		decoded["account"] = nil
	}
	return decoded, nil
}

// PluginSyncGetCapabilities returns the provider's encrypted-object
// capabilities (contract SyncCapabilitiesResult).
func (s *PluginService) PluginSyncGetCapabilities(request PluginSyncRequestBase) (map[string]any, error) {
	contribution, err := s.resolveExtensionProvider(providers.KindSync, request.ProviderID)
	if err != nil {
		return nil, err
	}
	requestID := orMintRequestID(request.RequestID)
	result, err := s.ext.invokeExtension(context.Background(), extensionInvocation{
		pluginID:   contribution.PluginID,
		providerID: contribution.Provider.ID,
		kind:       providers.KindSync,
		operation:  opSyncGetCapabilities,
		requestID:  requestID,
		deadline:   clampDispatchDeadline(request.DeadlineMs),
		payload:    map[string]any{"operationId": requestID},
	})
	if err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := json.Unmarshal(result, &decoded); err != nil || decoded == nil {
		return nil, errors.New("plugin sync getCapabilities returned invalid JSON")
	}
	return decoded, nil
}

type PluginSyncReadObjectRequest struct {
	RequestID    string `json:"requestId,omitempty"`
	ProviderID   string `json:"providerId"`
	Key          string `json:"key"`
	PreferStream bool   `json:"preferStream,omitempty"`
	DeadlineMs   int    `json:"deadlineMs,omitempty"`
}

type PluginSyncReadObjectResult struct {
	Found       bool   `json:"found"`
	Key         string `json:"key"`
	Data        string `json:"data,omitempty"` // base64
	Streamed    bool   `json:"streamed,omitempty"`
	TransferID  string `json:"transferId,omitempty"`
	ByteLength  int64  `json:"byteLength,omitempty"`
	Revision    string `json:"revision,omitempty"`
	ContentType string `json:"contentType,omitempty"`
}

// PluginSyncReadObject reads one encrypted object. Inline reads return the
// base64 payload; streamed reads return a transfer id the renderer pulls
// with pluginSyncReadChunk (the plugin keeps its read cursor in guest memory
// between dispatches).
func (s *PluginService) PluginSyncReadObject(request PluginSyncReadObjectRequest) (*PluginSyncReadObjectResult, error) {
	contribution, err := s.resolveExtensionProvider(providers.KindSync, request.ProviderID)
	if err != nil {
		return nil, err
	}
	requestID := orMintRequestID(request.RequestID)
	payload := map[string]any{
		"key":         request.Key,
		"operationId": requestID,
	}
	if request.PreferStream {
		payload["streamed"] = true
	}
	result, err := s.ext.invokeExtension(context.Background(), extensionInvocation{
		pluginID:   contribution.PluginID,
		providerID: contribution.Provider.ID,
		kind:       providers.KindSync,
		operation:  opSyncReadObject,
		requestID:  requestID,
		deadline:   clampDispatchDeadline(request.DeadlineMs),
		payload:    payload,
	})
	if err != nil {
		return nil, err
	}
	var decoded struct {
		Found       bool   `json:"found"`
		ByteLength  int64  `json:"byteLength"`
		Encoding    string `json:"encoding"`
		Data        string `json:"data"`
		Streamed    bool   `json:"streamed"`
		Revision    string `json:"revision"`
		ContentType string `json:"contentType"`
	}
	if err := json.Unmarshal(result, &decoded); err != nil {
		return nil, errors.New("plugin sync readObject returned invalid JSON")
	}
	out := &PluginSyncReadObjectResult{
		Found:       decoded.Found,
		Key:         request.Key,
		Revision:    decoded.Revision,
		ContentType: decoded.ContentType,
		ByteLength:  decoded.ByteLength,
	}
	if !decoded.Found {
		return out, nil
	}
	if decoded.Streamed {
		out.Streamed = true
		out.TransferID = requestID
		s.ext.rememberTransfer(requestID, contribution.PluginID, contribution.Provider.ID, requestID)
		return out, nil
	}
	if decoded.Encoding != "" && decoded.Encoding != "base64" {
		return nil, fmt.Errorf("plugin sync readObject returned unsupported encoding %q", decoded.Encoding)
	}
	if decoded.Data == "" {
		return nil, errors.New("plugin sync readObject returned no data")
	}
	if out.ByteLength == 0 {
		if raw, decodeErr := base64.StdEncoding.DecodeString(decoded.Data); decodeErr == nil {
			out.ByteLength = int64(len(raw))
		}
	}
	out.Data = decoded.Data
	return out, nil
}

type PluginSyncReadChunkRequest struct {
	RequestID  string `json:"requestId"`
	TransferID string `json:"transferId"`
	MaxBytes   int    `json:"maxBytes,omitempty"`
}

type PluginSyncReadChunkResult struct {
	Chunk string `json:"chunk"` // base64
	Done  bool   `json:"done"`
}

// PluginSyncReadChunk pulls the next window of a streamed read.
func (s *PluginService) PluginSyncReadChunk(request PluginSyncReadChunkRequest) (*PluginSyncReadChunkResult, error) {
	if request.TransferID == "" || request.RequestID == "" {
		return nil, errors.New("plugin sync read chunk requires requestId and transferId")
	}
	pluginID, providerID, ok := s.ext.transferProvider(request.TransferID)
	if !ok {
		return nil, errors.New("plugin sync transfer is unknown or expired")
	}
	maxBytes := request.MaxBytes
	if maxBytes <= 0 || maxBytes > maxSyncChunkBytes {
		maxBytes = maxSyncChunkBytes
	}
	result, err := s.ext.invokeExtension(context.Background(), extensionInvocation{
		pluginID:   pluginID,
		providerID: providerID,
		kind:       providers.KindSync,
		operation:  opSyncReadChunk,
		requestID:  request.RequestID,
		deadline:   clampDispatchDeadline(0),
		payload: map[string]any{
			"transferId":  request.TransferID,
			"operationId": request.RequestID,
			"maxBytes":    maxBytes,
		},
	})
	if err != nil {
		return nil, err
	}
	var decoded struct {
		Encoding string `json:"encoding"`
		Data     string `json:"data"`
		Done     bool   `json:"done"`
	}
	if err := json.Unmarshal(result, &decoded); err != nil || (decoded.Data == "" && !decoded.Done) {
		return nil, errors.New("plugin sync readChunk returned invalid JSON")
	}
	if decoded.Encoding != "" && decoded.Encoding != "base64" {
		return nil, fmt.Errorf("plugin sync readChunk returned unsupported encoding %q", decoded.Encoding)
	}
	if decoded.Done {
		s.ext.dropTransfer(request.TransferID)
	}
	return &PluginSyncReadChunkResult{Chunk: decoded.Data, Done: decoded.Done}, nil
}

type PluginSyncWriteObjectRequest struct {
	RequestID        string `json:"requestId,omitempty"`
	ProviderID       string `json:"providerId"`
	Key              string `json:"key"`
	Data             string `json:"data"` // base64
	ExpectedRevision string `json:"expectedRevision,omitempty"`
	PreferStream     bool   `json:"preferStream,omitempty"`
	DeadlineMs       int    `json:"deadlineMs,omitempty"`
}

type PluginSyncWriteResult struct {
	Created  bool   `json:"created"`
	Revision string `json:"revision,omitempty"`
}

// PluginSyncWriteObject writes one small encrypted object inline.
func (s *PluginService) PluginSyncWriteObject(request PluginSyncWriteObjectRequest) (*PluginSyncWriteResult, error) {
	contribution, err := s.resolveExtensionProvider(providers.KindSync, request.ProviderID)
	if err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(request.Data)
	if err != nil {
		return nil, errors.New("plugin sync write data must be base64")
	}
	if len(raw) > maxSyncChunkBytes {
		return nil, errors.New("plugin sync inline write exceeds the chunk bound; use the streamed write")
	}
	requestID := orMintRequestID(request.RequestID)
	payload := map[string]any{
		"key":         request.Key,
		"operationId": requestID,
		"byteLength":  len(raw),
		"encoding":    "base64",
		"data":        request.Data,
	}
	if request.ExpectedRevision != "" {
		payload["expectedRevision"] = request.ExpectedRevision
	}
	result, err := s.ext.invokeExtension(context.Background(), extensionInvocation{
		pluginID:   contribution.PluginID,
		providerID: contribution.Provider.ID,
		kind:       providers.KindSync,
		operation:  opSyncWriteObject,
		requestID:  requestID,
		deadline:   clampDispatchDeadline(request.DeadlineMs),
		payload:    payload,
	})
	if err != nil {
		return nil, err
	}
	var decoded PluginSyncWriteResult
	if err := json.Unmarshal(result, &decoded); err != nil {
		return nil, errors.New("plugin sync writeObject returned invalid JSON")
	}
	return &decoded, nil
}

type PluginSyncWriteBeginRequest struct {
	RequestID        string `json:"requestId,omitempty"`
	ProviderID       string `json:"providerId"`
	Key              string `json:"key"`
	ByteLength       int64  `json:"byteLength"`
	ExpectedRevision string `json:"expectedRevision,omitempty"`
	DeadlineMs       int    `json:"deadlineMs,omitempty"`
}

type PluginSyncWriteBeginResult struct {
	TransferID  string `json:"transferId"`
	WindowBytes int    `json:"windowBytes"`
}

// PluginSyncWriteBegin opens a chunked write transfer.
func (s *PluginService) PluginSyncWriteBegin(request PluginSyncWriteBeginRequest) (*PluginSyncWriteBeginResult, error) {
	contribution, err := s.resolveExtensionProvider(providers.KindSync, request.ProviderID)
	if err != nil {
		return nil, err
	}
	if request.ByteLength < 0 || request.ByteLength > pluginSyncMaxObjectBytes {
		return nil, fmt.Errorf("plugin sync object size %d exceeds the contract bound", request.ByteLength)
	}
	requestID := orMintRequestID(request.RequestID)
	payload := map[string]any{
		"key":         request.Key,
		"operationId": requestID,
		"byteLength":  request.ByteLength,
	}
	if request.ExpectedRevision != "" {
		payload["expectedRevision"] = request.ExpectedRevision
	}
	result, err := s.ext.invokeExtension(context.Background(), extensionInvocation{
		pluginID:   contribution.PluginID,
		providerID: contribution.Provider.ID,
		kind:       providers.KindSync,
		operation:  opSyncWriteBegin,
		requestID:  requestID,
		deadline:   clampDispatchDeadline(request.DeadlineMs),
		payload:    payload,
	})
	if err != nil {
		return nil, err
	}
	var decoded struct {
		WindowBytes int `json:"windowBytes"`
	}
	_ = json.Unmarshal(result, &decoded)
	windowBytes := decoded.WindowBytes
	if windowBytes <= 0 || windowBytes > maxSyncChunkBytes {
		windowBytes = maxSyncChunkBytes
	}
	s.ext.rememberTransfer(requestID, contribution.PluginID, contribution.Provider.ID, requestID)
	return &PluginSyncWriteBeginResult{TransferID: requestID, WindowBytes: windowBytes}, nil
}

// pluginSyncMaxObjectBytes mirrors the contract SyncLimits.maxObjectBytes.
const pluginSyncMaxObjectBytes = 64 << 20

type PluginSyncWriteChunkRequest struct {
	RequestID  string `json:"requestId"`
	TransferID string `json:"transferId"`
	Sequence   int    `json:"sequence"`
	Chunk      string `json:"chunk"` // base64
}

// PluginSyncWriteChunk streams one window of a chunked write.
func (s *PluginService) PluginSyncWriteChunk(request PluginSyncWriteChunkRequest) (map[string]any, error) {
	if request.TransferID == "" || request.RequestID == "" {
		return nil, errors.New("plugin sync write chunk requires requestId and transferId")
	}
	raw, err := base64.StdEncoding.DecodeString(request.Chunk)
	if err != nil {
		return nil, errors.New("plugin sync write chunk must be base64")
	}
	if len(raw) > maxSyncChunkBytes {
		return nil, errors.New("plugin sync write chunk exceeds the chunk bound")
	}
	pluginID, providerID, ok := s.ext.transferProvider(request.TransferID)
	if !ok {
		return nil, errors.New("plugin sync transfer is unknown or expired")
	}
	result, err := s.ext.invokeExtension(context.Background(), extensionInvocation{
		pluginID:   pluginID,
		providerID: providerID,
		kind:       providers.KindSync,
		operation:  opSyncWriteChunk,
		requestID:  request.RequestID,
		deadline:   clampDispatchDeadline(0),
		payload: map[string]any{
			"transferId":  request.TransferID,
			"operationId": request.RequestID,
			"sequence":    request.Sequence,
			"encoding":    "base64",
			"data":        request.Chunk,
		},
	})
	if err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := json.Unmarshal(result, &decoded); err != nil || decoded == nil {
		decoded = map[string]any{"accepted": len(raw)}
	}
	if _, ok := decoded["accepted"]; !ok {
		decoded["accepted"] = len(raw)
	}
	return decoded, nil
}

type PluginSyncWriteCommitRequest struct {
	RequestID  string `json:"requestId"`
	TransferID string `json:"transferId"`
}

// PluginSyncWriteCommit finalizes a chunked write.
func (s *PluginService) PluginSyncWriteCommit(request PluginSyncWriteCommitRequest) (*PluginSyncWriteResult, error) {
	if request.TransferID == "" || request.RequestID == "" {
		return nil, errors.New("plugin sync write commit requires requestId and transferId")
	}
	pluginID, providerID, ok := s.ext.transferProvider(request.TransferID)
	if !ok {
		return nil, errors.New("plugin sync transfer is unknown or expired")
	}
	defer s.ext.dropTransfer(request.TransferID)
	result, err := s.ext.invokeExtension(context.Background(), extensionInvocation{
		pluginID:   pluginID,
		providerID: providerID,
		kind:       providers.KindSync,
		operation:  opSyncWriteCommit,
		requestID:  request.RequestID,
		deadline:   clampDispatchDeadline(0),
		payload: map[string]any{
			"transferId":  request.TransferID,
			"operationId": request.RequestID,
		},
	})
	if err != nil {
		return nil, err
	}
	var decoded PluginSyncWriteResult
	if err := json.Unmarshal(result, &decoded); err != nil {
		return nil, errors.New("plugin sync writeCommit returned invalid JSON")
	}
	return &decoded, nil
}

type PluginSyncDeleteObjectRequest struct {
	RequestID        string `json:"requestId,omitempty"`
	ProviderID       string `json:"providerId"`
	Key              string `json:"key"`
	ExpectedRevision string `json:"expectedRevision,omitempty"`
	DeadlineMs       int    `json:"deadlineMs,omitempty"`
}

// PluginSyncDeleteObject deletes one object (conditional when a revision is
// supplied).
func (s *PluginService) PluginSyncDeleteObject(request PluginSyncDeleteObjectRequest) (map[string]any, error) {
	contribution, err := s.resolveExtensionProvider(providers.KindSync, request.ProviderID)
	if err != nil {
		return nil, err
	}
	requestID := orMintRequestID(request.RequestID)
	payload := map[string]any{
		"key":         request.Key,
		"operationId": requestID,
	}
	if request.ExpectedRevision != "" {
		payload["expectedRevision"] = request.ExpectedRevision
	}
	result, err := s.ext.invokeExtension(context.Background(), extensionInvocation{
		pluginID:   contribution.PluginID,
		providerID: contribution.Provider.ID,
		kind:       providers.KindSync,
		operation:  opSyncDeleteObject,
		requestID:  requestID,
		deadline:   clampDispatchDeadline(request.DeadlineMs),
		payload:    payload,
	})
	if err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := json.Unmarshal(result, &decoded); err != nil || decoded == nil {
		return nil, errors.New("plugin sync deleteObject returned invalid JSON")
	}
	return decoded, nil
}

func orMintRequestID(requestID string) string {
	if strings.TrimSpace(requestID) != "" {
		return requestID
	}
	return mintPluginRequestID()
}
