// Package wasm owns the plugin WASM runtime (P5-03): a wazero-based sandbox
// with context-driven cancellation, WASI disabled and clean teardown. The
// permission broker (P5-02A) gates all host function imports, and the
// lemonssh-wasm-abi v1 dispatch channel (see
// docs/plugin-platform/isolated-runtime.md) is the only RPC path into a
// module: one JSON envelope exchanged through the guest's own
// lemonssh_alloc/lemonssh_dispatch/lemonssh_free exports. Pre-rename plugin
// binaries built against the netcatty ABI keep working: the host registers
// the legacy module name and the guest export resolution falls back.
package wasm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/lemon-casino/lemonssh/internal/plugin/permissions"
)

var (
	ErrRuntimeClosed    = errors.New("wasm runtime closed")
	ErrModuleNotFound   = errors.New("wasm module not found")
	ErrAlreadyInstant   = errors.New("wasm module already instantiated")
	ErrNoDispatchABI    = errors.New("wasm module does not export the lemonssh dispatch ABI")
	ErrRequestTooLarge  = errors.New("wasm dispatch request exceeds the ABI limit")
	ErrResponseTooLarge = errors.New("wasm dispatch response exceeds the ABI limit")
	ErrDispatchOOM      = errors.New("wasm dispatch failed to allocate module memory")
	ErrDispatchTimeout  = errors.New("wasm dispatch timed out")
	ErrDispatchFailed   = errors.New("wasm dispatch failed")
)

// Canonical lemonssh-wasm-abi v1 names (mirrored by
// packages/plugin-contract schema definition WasmAbi), plus the pre-rename
// lemonssh-wasm-abi v1 names kept as a legacy fallback: already-installed
// plugin binaries are persistent data and keep instantiating against them.
const (
	// HostModuleName is the wazero host module that provides the gated
	// lemonssh_host_* imports. LegacyHostModuleName carries the same gated
	// functions under the pre-rename import names for old plugin binaries.
	HostModuleName       = "lemonssh"
	LegacyHostModuleName = "netcatty"
	// ExportAlloc reserves guest bytes: (size i32) -> ptr i32 (0 on failure).
	ExportAlloc = "lemonssh_alloc"
	// ExportFree releases a guest region: (ptr i32, len i32).
	ExportFree = "lemonssh_free"
	// ExportDispatch runs one request: (reqPtr i32, reqLen i32) -> respPtr i32
	// pointing at a [u32 LE length][payload] region, or 0 on failure.
	ExportDispatch = "lemonssh_dispatch"
	// ImportHostLog logs a line: (level i32, ptr i32, len i32) -> status i32.
	ImportHostLog = "lemonssh_host_log"
	// ImportHostSettingGet reads one declared non-secret setting:
	// (keyPtr, keyLen, bufPtr, bufCap i32) -> written/needed i32.
	ImportHostSettingGet = "lemonssh_host_setting_get"

	// Legacy ABI names resolved when the new ones are absent (guest exports)
	// and registered under LegacyHostModuleName (host imports).
	LegacyExportAlloc          = "netcatty_alloc"
	LegacyExportFree           = "netcatty_free"
	LegacyExportDispatch       = "netcatty_dispatch"
	LegacyImportHostLog        = "netcatty_host_log"
	LegacyImportHostSettingGet = "netcatty_host_setting_get"
)

// Host import status codes returned to the guest as i32. Negative values are
// failures; a denial is a structured code, never a trap or host panic.
const (
	HostImportOK               = 0
	HostImportPermissionDenied = -1
	HostImportInvalid          = -2
	HostImportUnavailable      = -3
)

// Dispatch limits (mirrored by the WasmAbi schema constants). Request and
// response envelopes reuse the contract's 1 MiB RPC byte limit.
const (
	MaxRequestBytes      = 1 << 20
	MaxResponseBytes     = 1 << 20
	DefaultDispatchLimit = 10 * time.Second
	// maxLogBytes caps one host_log message; messages beyond it are truncated.
	maxLogBytes = 8 << 10
	// maxLogEntries caps the per-plugin diagnostic log ring.
	maxLogEntries = 64
	// maxSettingKeyBytes caps the host_setting_get key length.
	maxSettingKeyBytes = 256
)

// Module wraps one instantiated WASM module and the runtime that owns it
// (capped modules live in their own runtime).
type Module struct {
	pluginID string
	closer   api.Closer
	inner    wazero.Runtime
	mod      api.Module

	hasABI     bool
	allocFn    api.Function
	freeFn     api.Function
	dispatchFn api.Function

	// dispatchMu serializes dispatch: guest allocators (bump pointers,
	// globals) are not re-entrant.
	dispatchMu sync.Mutex
	closed     bool

	logMu  sync.Mutex
	logs   []LogEntry
	denied uint64
}

// LogEntry is one broker-approved host_log line.
type LogEntry struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// HasDispatchABI reports whether the module exports the lemonssh dispatch ABI.
func (m *Module) HasDispatchABI() bool { return m.hasABI }

// Close tears down the module.
func (m *Module) Close(ctx context.Context) error { return m.closer.Close(ctx) }

// Runtime hosts WASM plugin instances.
type Runtime struct {
	mu      sync.Mutex
	closed  bool
	shared  wazero.Runtime
	modules map[string]*Module
	// byInstance resolves the importing plugin inside host import functions.
	byInstance map[api.Module]*Module
	// hostModules holds the instantiated host module pair per wazero
	// runtime (capped plugins get their own copy).
	hostModules map[wazero.Runtime]api.Module

	broker           *permissions.Broker
	settingsProvider func(pluginID string) (map[string]any, error)
}

// NewRuntime creates a WASM runtime with WASI disabled and
// close-on-context-done enabled.
func NewRuntime(ctx context.Context) (*Runtime, error) {
	config := wazero.NewRuntimeConfig().WithCloseOnContextDone(true)
	inner := wazero.NewRuntimeWithConfig(ctx, config)
	return &Runtime{
		shared:      inner,
		modules:     make(map[string]*Module),
		byInstance:  make(map[api.Module]*Module),
		hostModules: make(map[wazero.Runtime]api.Module),
	}, nil
}

// SetBroker attaches the fail-closed permission broker consulted by every
// host import. A nil broker denies all imports.
func (r *Runtime) SetBroker(broker *permissions.Broker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.broker = broker
}

// SetSettingsProvider wires the controlled settings reader used by
// lemonssh_host_setting_get. The provider must return only the plugin's own
// declared non-secret settings; a nil provider denies all reads.
func (r *Runtime) SetSettingsProvider(provider func(pluginID string) (map[string]any, error)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.settingsProvider = provider
}

// RecentLogs returns the plugin's retained host_log lines (newest last).
func (r *Runtime) RecentLogs(pluginID string) []LogEntry {
	r.mu.Lock()
	module, ok := r.modules[pluginID]
	r.mu.Unlock()
	if !ok {
		return nil
	}
	module.logMu.Lock()
	defer module.logMu.Unlock()
	out := make([]LogEntry, len(module.logs))
	copy(out, module.logs)
	return out
}

// ImportDenials reports how many host import calls were denied by the broker
// for this plugin (diagnostics; never grows on the happy path).
func (r *Runtime) ImportDenials(pluginID string) uint64 {
	r.mu.Lock()
	module, ok := r.modules[pluginID]
	r.mu.Unlock()
	if !ok {
		return 0
	}
	module.logMu.Lock()
	defer module.logMu.Unlock()
	return module.denied
}

// Config for one WASM module instantiation.
type Config struct {
	PluginID  string
	WASMBytes []byte
	// MemoryLimitPages caps linear memory in 64 KiB pages; 0 uses the module's
	// own declaration. Comes from manifest entrypoint.memoryMB (P5-03). A
	// capped module gets its own wazero runtime because wazero applies the
	// limit per runtime, not per module.
	MemoryLimitPages uint32
}

// MemoryPagesFromMB converts the manifest's memoryMB into 64 KiB pages.
func MemoryPagesFromMB(memoryMB int) uint32 {
	if memoryMB <= 0 {
		return 0
	}
	const pagesPerMB = 1024 * 1024 / 65536 // 16 pages per MiB
	return uint32(memoryMB) * pagesPerMB
}

// InstantiateWithConfig compiles and instantiates a WASM module, applying the
// memory cap in its own runtime when configured. The host module pair
// (gated imports) is instantiated first so importing guests resolve it.
func (r *Runtime) InstantiateWithConfig(ctx context.Context, config Config) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return ErrRuntimeClosed
	}
	if _, exists := r.modules[config.PluginID]; exists {
		r.mu.Unlock()
		return ErrAlreadyInstant
	}
	r.mu.Unlock()

	inner := r.shared
	if config.MemoryLimitPages > 0 {
		capped := wazero.NewRuntimeConfig().
			WithCloseOnContextDone(true).
			WithMemoryLimitPages(config.MemoryLimitPages)
		inner = wazero.NewRuntimeWithConfig(ctx, capped)
	}
	if err := r.ensureHostModule(ctx, inner); err != nil {
		if inner != r.shared {
			_ = inner.Close(ctx)
		}
		return fmt.Errorf("instantiate host module for %s: %w", config.PluginID, err)
	}
	compiled, err := inner.CompileModule(ctx, config.WASMBytes)
	if err != nil {
		if inner != r.shared {
			_ = inner.Close(ctx)
		}
		return fmt.Errorf("compile %s: %w", config.PluginID, err)
	}
	mod, err := inner.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithStartFunctions("_start"))
	if err != nil {
		if inner != r.shared {
			_ = inner.Close(ctx)
		}
		return fmt.Errorf("instantiate %s: %w", config.PluginID, err)
	}
	// Guest export resolution: the new ABI names win; pre-rename binaries
	// exporting the netcatty_* names resolve through the legacy fallback.
	module := &Module{
		pluginID:   config.PluginID,
		closer:     mod,
		inner:      inner,
		mod:        mod,
		allocFn:    exportOrLegacy(mod, ExportAlloc, LegacyExportAlloc),
		freeFn:     exportOrLegacy(mod, ExportFree, LegacyExportFree),
		dispatchFn: exportOrLegacy(mod, ExportDispatch, LegacyExportDispatch),
	}
	module.hasABI = module.allocFn != nil && module.freeFn != nil && module.dispatchFn != nil && mod.Memory() != nil

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		_ = mod.Close(ctx)
		if inner != r.shared {
			_ = inner.Close(ctx)
		}
		return ErrRuntimeClosed
	}
	if _, exists := r.modules[config.PluginID]; exists {
		r.mu.Unlock()
		_ = mod.Close(ctx)
		if inner != r.shared {
			_ = inner.Close(ctx)
		}
		return ErrAlreadyInstant
	}
	r.modules[config.PluginID] = module
	r.byInstance[mod] = module
	r.mu.Unlock()
	return nil
}

// exportOrLegacy resolves a guest export by new ABI name first, falling back
// to the pre-rename netcatty name for already-installed plugin binaries.
func exportOrLegacy(mod api.Module, name, legacyName string) api.Function {
	if fn := mod.ExportedFunction(name); fn != nil {
		return fn
	}
	return mod.ExportedFunction(legacyName)
}

// ensureHostModule instantiates the gated host modules once per wazero
// runtime: the lemonssh module under the current import names and a legacy
// netcatty module carrying the same gated functions under the pre-rename
// names, so both current and pre-rename plugin binaries resolve their
// imports. Host functions resolve the importing module through byInstance,
// so one host module pair serves every plugin in that runtime.
func (r *Runtime) ensureHostModule(ctx context.Context, inner wazero.Runtime) error {
	r.mu.Lock()
	if _, ok := r.hostModules[inner]; ok {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()

	builder := inner.NewHostModuleBuilder(HostModuleName)
	builder.NewFunctionBuilder().WithFunc(r.hostLog).Export(ImportHostLog)
	builder.NewFunctionBuilder().WithFunc(r.hostSettingGet).Export(ImportHostSettingGet)
	hostModule, err := builder.Instantiate(ctx)
	if err != nil {
		return err
	}
	// Legacy module: identical gated functions under the pre-rename names.
	legacyBuilder := inner.NewHostModuleBuilder(LegacyHostModuleName)
	legacyBuilder.NewFunctionBuilder().WithFunc(r.hostLog).Export(LegacyImportHostLog)
	legacyBuilder.NewFunctionBuilder().WithFunc(r.hostSettingGet).Export(LegacyImportHostSettingGet)
	legacyModule, err := legacyBuilder.Instantiate(ctx)
	if err != nil {
		_ = hostModule.Close(ctx)
		return err
	}
	r.mu.Lock()
	if _, ok := r.hostModules[inner]; ok {
		r.mu.Unlock()
		_ = hostModule.Close(ctx)
		_ = legacyModule.Close(ctx)
		return nil
	}
	// Either instance works as the presence marker; both are torn down by
	// the owning runtime's Close.
	r.hostModules[inner] = hostModule
	_ = legacyModule
	r.mu.Unlock()
	return nil
}

// Instantiate compiles and instantiates a WASM module.
func (r *Runtime) Instantiate(ctx context.Context, pluginID string, wasmBytes []byte) error {
	return r.InstantiateWithConfig(ctx, Config{PluginID: pluginID, WASMBytes: wasmBytes})
}

// CloseModule tears down one module instance.
func (r *Runtime) CloseModule(pluginID string) error {
	r.mu.Lock()
	module, ok := r.modules[pluginID]
	if ok {
		delete(r.modules, pluginID)
		delete(r.byInstance, module.mod)
	}
	r.mu.Unlock()
	if !ok {
		return ErrModuleNotFound
	}
	// Serialize against in-flight dispatches before closing guest memory.
	module.dispatchMu.Lock()
	module.closed = true
	module.dispatchMu.Unlock()
	if err := module.closer.Close(context.Background()); err != nil {
		return err
	}
	if module.inner != r.shared {
		return module.inner.Close(context.Background())
	}
	return nil
}

// Close tears down the runtime and all modules.
func (r *Runtime) Close(ctx context.Context) error {
	r.mu.Lock()
	r.closed = true
	modules := r.modules
	r.modules = make(map[string]*Module)
	r.byInstance = make(map[api.Module]*Module)
	for _, module := range modules {
		module.dispatchMu.Lock()
		module.closed = true
		module.dispatchMu.Unlock()
	}
	// Host modules belong to their wazero runtime and are torn down by the
	// inner.Close calls below; r.hostModules only prevents double
	// instantiation while the runtime is live.
	r.hostModules = make(map[wazero.Runtime]api.Module)
	r.mu.Unlock()
	var firstErr error
	closed := map[wazero.Runtime]bool{r.shared: true}
	for _, module := range modules {
		if err := module.closer.Close(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
		if !closed[module.inner] {
			closed[module.inner] = true
		}
	}
	for inner := range closed {
		if err := inner.Close(context.Background()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Dispatch envelope --------------------------------------------------------------------

// DispatchRequest is the JSON envelope the host writes into guest memory.
type DispatchRequest struct {
	Method  string          `json:"method"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// DispatchError is the plugin's in-band structured failure.
type DispatchError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// DispatchResult is the JSON envelope the guest returns behind the
// [u32 LE length] prefix: ok=true carries result, ok=false carries error.
type DispatchResult struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *DispatchError  `json:"error,omitempty"`
}

// Dispatch sends one lemonssh-wasm-abi v1 request to the plugin's
// lemonssh_dispatch export. Transport failures (missing module/ABI, size
// caps, allocation failures, traps, timeouts) are returned as errors; a
// plugin-declared failure arrives as a DispatchResult with OK=false.
func (r *Runtime) Dispatch(parent context.Context, pluginID, method, payloadJSON string) (result *DispatchResult, err error) {
	defer func() {
		if p := recover(); p != nil {
			result = nil
			err = fmt.Errorf("%w: host panic recovered: %v", ErrDispatchFailed, p)
		}
	}()
	if method == "" {
		return nil, fmt.Errorf("%w: method is required", ErrDispatchFailed)
	}
	var payload json.RawMessage
	if payloadJSON != "" {
		if !json.Valid([]byte(payloadJSON)) {
			return nil, fmt.Errorf("%w: payload is not valid JSON", ErrDispatchFailed)
		}
		payload = json.RawMessage(payloadJSON)
	}
	request, err := json.Marshal(DispatchRequest{Method: method, Payload: payload})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDispatchFailed, err)
	}
	if len(request) > MaxRequestBytes {
		return nil, ErrRequestTooLarge
	}

	r.mu.Lock()
	module, ok := r.modules[pluginID]
	r.mu.Unlock()
	if !ok {
		return nil, ErrModuleNotFound
	}
	if !module.hasABI {
		return nil, ErrNoDispatchABI
	}
	module.dispatchMu.Lock()
	defer module.dispatchMu.Unlock()
	if module.closed {
		return nil, ErrModuleNotFound
	}
	memory := module.mod.Memory()

	timeout := DefaultDispatchLimit
	if deadline, ok := parent.Deadline(); ok && time.Until(deadline) < timeout {
		timeout = time.Until(deadline)
	}
	if timeout <= 0 {
		return nil, ErrDispatchTimeout
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	reqPtr, err := guestAlloc(ctx, module, len(request))
	if err != nil {
		return nil, err
	}
	if !memory.Write(reqPtr, request) {
		guestFree(ctx, module, reqPtr, uint32(len(request)))
		return nil, fmt.Errorf("%w: request memory write failed", ErrDispatchFailed)
	}
	respRaw, err := module.dispatchFn.Call(ctx, uint64(reqPtr), uint64(len(request)))
	if err != nil {
		guestFree(ctx, module, reqPtr, uint32(len(request)))
		return nil, wrapCallError(pluginID, "dispatch", ctx, err)
	}
	if len(respRaw) == 0 {
		return nil, fmt.Errorf("%w: dispatch returned no value", ErrDispatchFailed)
	}
	respPtr := uint32(respRaw[0])
	if respPtr == 0 {
		guestFree(ctx, module, reqPtr, uint32(len(request)))
		return nil, ErrDispatchOOM
	}
	respLen, ok := memory.ReadUint32Le(respPtr)
	if !ok {
		guestFree(ctx, module, respPtr, 4)
		guestFree(ctx, module, reqPtr, uint32(len(request)))
		return nil, fmt.Errorf("%w: response length prefix is out of bounds", ErrDispatchFailed)
	}
	if respLen == 0 || respLen > MaxResponseBytes {
		guestFree(ctx, module, respPtr, 4+min(respLen, MaxResponseBytes))
		guestFree(ctx, module, reqPtr, uint32(len(request)))
		if respLen > MaxResponseBytes {
			return nil, ErrResponseTooLarge
		}
		return nil, fmt.Errorf("%w: empty response envelope", ErrDispatchFailed)
	}
	view, ok := memory.Read(respPtr+4, respLen)
	if !ok {
		guestFree(ctx, module, respPtr, 4+respLen)
		guestFree(ctx, module, reqPtr, uint32(len(request)))
		return nil, fmt.Errorf("%w: response payload is out of bounds", ErrDispatchFailed)
	}
	envelope := make([]byte, respLen)
	copy(envelope, view)
	guestFree(ctx, module, respPtr, 4+respLen)
	guestFree(ctx, module, reqPtr, uint32(len(request)))

	var decoded DispatchResult
	if err := json.Unmarshal(envelope, &decoded); err != nil {
		return nil, fmt.Errorf("%w: malformed response envelope: %v", ErrDispatchFailed, err)
	}
	return &decoded, nil
}

func wrapCallError(pluginID, stage string, ctx context.Context, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%w: %s %s aborted", ErrDispatchTimeout, pluginID, stage)
	}
	return fmt.Errorf("%w: %s %s: %v", ErrDispatchFailed, pluginID, stage, err)
}

func guestAlloc(ctx context.Context, module *Module, size int) (uint32, error) {
	if size <= 0 || size > MaxRequestBytes {
		return 0, fmt.Errorf("%w: request size %d out of range", ErrDispatchFailed, size)
	}
	ret, err := module.allocFn.Call(ctx, uint64(size))
	if err != nil {
		return 0, wrapCallError(module.pluginID, "alloc", ctx, err)
	}
	if len(ret) == 0 {
		return 0, fmt.Errorf("%w: alloc returned no value", ErrDispatchFailed)
	}
	ptr := uint32(ret[0])
	if ptr == 0 {
		return 0, ErrDispatchOOM
	}
	return ptr, nil
}

func guestFree(ctx context.Context, module *Module, ptr, size uint32) {
	if module.freeFn == nil {
		return
	}
	_, _ = module.freeFn.Call(ctx, uint64(ptr), uint64(size))
}

// hostStatus converts a structured import status to the i32 the guest sees
// (negative codes wrap as two's complement).
func hostStatus(v int32) uint32 { return uint32(v) }

func min(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

// Host imports --------------------------------------------------------------------------

// hostLog implements the gated host_log import: level 0..3 (debug..error),
// message at [ptr, ptr+len). Broker-gated by the plugin's declared
// ["runtime","log"] write permission. Returns a structured status code; it
// never traps and never panics.
func (r *Runtime) hostLog(ctx context.Context, m api.Module, level, ptr, length uint32) uint32 {
	module := r.moduleForInstance(m)
	if module == nil {
		return hostStatus(HostImportInvalid)
	}
	if level > 3 {
		return hostStatus(HostImportInvalid)
	}
	if !r.authorize(module.pluginID, "log", "write") {
		module.recordDenied()
		return hostStatus(HostImportPermissionDenied)
	}
	if length > maxLogBytes {
		length = maxLogBytes
	}
	message, ok := readGuestBytes(m, ptr, length)
	if !ok {
		return hostStatus(HostImportInvalid)
	}
	module.appendLog(logLevelName(level), string(message))
	return hostStatus(HostImportOK)
}

// hostSettingGet implements the gated host_setting_get import: reads one of
// the plugin's own declared non-secret settings into the guest buffer.
// Returns the value byte length: ret > 0 && ret <= bufCap means the value was
// written; ret > bufCap means the value needs ret bytes and nothing was
// written; negatives are the structured failure codes above. Broker-gated by
// the plugin's declared ["runtime","settings"] read permission.
func (r *Runtime) hostSettingGet(ctx context.Context, m api.Module, keyPtr, keyLen, bufPtr, bufCap uint32) uint32 {
	module := r.moduleForInstance(m)
	if module == nil {
		return hostStatus(HostImportInvalid)
	}
	if keyLen == 0 || keyLen > maxSettingKeyBytes {
		return hostStatus(HostImportInvalid)
	}
	if !r.authorize(module.pluginID, "settings", "read") {
		module.recordDenied()
		return hostStatus(HostImportPermissionDenied)
	}
	r.mu.Lock()
	provider := r.settingsProvider
	r.mu.Unlock()
	if provider == nil {
		return hostStatus(HostImportUnavailable)
	}
	keyBytes, ok := readGuestBytes(m, keyPtr, keyLen)
	if !ok {
		return hostStatus(HostImportInvalid)
	}
	values, err := provider(module.pluginID)
	if err != nil {
		return hostStatus(HostImportUnavailable)
	}
	value, ok := values[string(keyBytes)]
	if !ok {
		return hostStatus(HostImportInvalid)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return hostStatus(HostImportUnavailable)
	}
	if uint32(len(encoded)) > MaxResponseBytes || bufCap > MaxResponseBytes {
		return hostStatus(HostImportInvalid)
	}
	needed := uint32(len(encoded))
	if needed > bufCap {
		return needed
	}
	if !m.Memory().Write(bufPtr, encoded) {
		return hostStatus(HostImportUnavailable)
	}
	return needed
}

func (r *Runtime) moduleForInstance(m api.Module) *Module {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byInstance[m]
}

// authorize consults the fail-closed broker with the same canonical resource
// key host.Host uses for grants: ["runtime","<name>"]:<mode>.
func (r *Runtime) authorize(pluginID, name, mode string) bool {
	r.mu.Lock()
	broker := r.broker
	r.mu.Unlock()
	if broker == nil {
		return false
	}
	key, err := json.Marshal([]string{"runtime", name})
	if err != nil {
		return false
	}
	return broker.Check(pluginID, string(key)+":"+mode, mode) == nil
}

func readGuestBytes(m api.Module, ptr, length uint32) ([]byte, bool) {
	if length == 0 {
		return nil, false
	}
	view, ok := m.Memory().Read(ptr, length)
	if !ok {
		return nil, false
	}
	out := make([]byte, length)
	copy(out, view)
	return out, true
}

func logLevelName(level uint32) string {
	switch level {
	case 0:
		return "debug"
	case 1:
		return "info"
	case 2:
		return "warn"
	default:
		return "error"
	}
}

func (m *Module) appendLog(level, message string) {
	m.logMu.Lock()
	defer m.logMu.Unlock()
	if len(message) > maxLogBytes {
		message = message[:maxLogBytes]
	}
	if len(m.logs) >= maxLogEntries {
		m.logs = m.logs[1:]
	}
	m.logs = append(m.logs, LogEntry{Level: level, Message: message})
}

func (m *Module) recordDenied() {
	m.logMu.Lock()
	m.denied++
	m.logMu.Unlock()
}
