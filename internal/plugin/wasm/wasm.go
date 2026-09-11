// Package wasm owns the plugin WASM runtime (P5-03): a wazero-based sandbox
// with context-driven cancellation, WASI disabled by default and clean
// teardown. The permission broker (P5-02A) gates all host function imports.
package wasm

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

var (
	ErrRuntimeClosed  = errors.New("wasm runtime closed")
	ErrModuleNotFound = errors.New("wasm module not found")
	ErrAlreadyInstant = errors.New("wasm module already instantiated")
)

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

// Module wraps one instantiated WASM module and the runtime that owns it
// (capped modules live in their own runtime).
type Module struct {
	pluginID string
	closer   api.Closer
	inner    wazero.Runtime
}

// Close tears down the module.
func (m *Module) Close(ctx context.Context) error { return m.closer.Close(ctx) }

// Runtime hosts WASM plugin instances.
type Runtime struct {
	mu      sync.Mutex
	closed  bool
	shared  wazero.Runtime
	modules map[string]*Module
}

// NewRuntime creates a WASM runtime with WASI disabled and
// close-on-context-done enabled.
func NewRuntime(ctx context.Context) (*Runtime, error) {
	config := wazero.NewRuntimeConfig().WithCloseOnContextDone(true)
	inner := wazero.NewRuntimeWithConfig(ctx, config)
	return &Runtime{shared: inner, modules: make(map[string]*Module)}, nil
}

// InstantiateWithConfig compiles and instantiates a WASM module, applying the
// memory cap in its own runtime when configured.
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
	compiled, err := inner.CompileModule(ctx, config.WASMBytes)
	if err != nil {
		if inner != r.shared {
			_ = inner.Close(ctx)
		}
		return fmt.Errorf("compile %s: %w", config.PluginID, err)
	}
	closer, err := inner.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithStartFunctions("_start"))
	if err != nil {
		if inner != r.shared {
			_ = inner.Close(ctx)
		}
		return fmt.Errorf("instantiate %s: %w", config.PluginID, err)
	}
	r.mu.Lock()
	r.modules[config.PluginID] = &Module{pluginID: config.PluginID, closer: closer, inner: inner}
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
	}
	r.mu.Unlock()
	if !ok {
		return ErrModuleNotFound
	}
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
