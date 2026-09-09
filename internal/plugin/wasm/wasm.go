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
}

// Module wraps one instantiated WASM module.
type Module struct {
	pluginID string
	closer   api.Closer
}

// Close tears down the module.
func (m *Module) Close(ctx context.Context) error { return m.closer.Close(ctx) }

// Runtime hosts WASM plugin instances.
type Runtime struct {
	mu      sync.Mutex
	closed  bool
	inner   wazero.Runtime
	modules map[string]api.Closer
}

// NewRuntime creates a WASM runtime with WASI disabled and
// close-on-context-done enabled.
func NewRuntime(ctx context.Context) (*Runtime, error) {
	config := wazero.NewRuntimeConfig().WithCloseOnContextDone(true)
	inner := wazero.NewRuntimeWithConfig(ctx, config)
	return &Runtime{inner: inner, modules: make(map[string]api.Closer)}, nil
}

// Instantiate compiles and instantiates a WASM module.
func (r *Runtime) Instantiate(ctx context.Context, pluginID string, wasmBytes []byte) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return ErrRuntimeClosed
	}
	if _, exists := r.modules[pluginID]; exists {
		r.mu.Unlock()
		return ErrAlreadyInstant
	}
	r.mu.Unlock()

	compiled, err := r.inner.CompileModule(ctx, wasmBytes)
	if err != nil {
		return fmt.Errorf("compile %s: %w", pluginID, err)
	}
	moduleConfig := wazero.NewModuleConfig().WithStartFunctions("_start")
	closer, err := r.inner.InstantiateModule(ctx, compiled, moduleConfig)
	if err != nil {
		return fmt.Errorf("instantiate %s: %w", pluginID, err)
	}
	r.mu.Lock()
	r.modules[pluginID] = closer
	r.mu.Unlock()
	return nil
}

// CloseModule tears down one module instance.
func (r *Runtime) CloseModule(pluginID string) error {
	r.mu.Lock()
	closer, ok := r.modules[pluginID]
	if ok {
		delete(r.modules, pluginID)
	}
	r.mu.Unlock()
	if !ok {
		return ErrModuleNotFound
	}
	return closer.Close(context.Background())
}

// Close tears down the runtime and all modules.
func (r *Runtime) Close(ctx context.Context) error {
	r.mu.Lock()
	r.closed = true
	modules := r.modules
	r.modules = make(map[string]api.Closer)
	r.mu.Unlock()
	for _, closer := range modules {
		_ = closer.Close(ctx)
	}
	return r.inner.Close(context.Background())
}
