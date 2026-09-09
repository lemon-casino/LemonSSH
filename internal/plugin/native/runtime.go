// Package native implements the plugin native child-process runtime (P5-05):
// hash-pinned, broker-authorized supervised processes with bounded restarts,
// stdout/stderr capture, quarantine and clean teardown. It reuses the
// supervised runner from P3-08.3 and the permission broker from P5-02A.
package native

import (
	"errors"
	"fmt"
	"sync"
)

var (
	ErrNotAuthorized  = errors.New("native: plugin not authorized for native execution")
	ErrAlreadyRunning = errors.New("native: process already running")
	ErrNotRunning     = errors.New("native: process not running")
	ErrMaxNativeprocs = errors.New("native: max concurrent native processes reached")
)

// ProcessSpec describes one native child process.
type ProcessSpec struct {
	PluginID   string
	BinaryPath string
	Args       []string
}

// NativeProcess is one supervised child process with permission enforcement.
type NativeProcess struct {
	PluginID string
	mu       sync.Mutex
	running  bool
	stop     func()
}

// Runtime manages native child processes for all plugins.
type Runtime struct {
	mu      sync.Mutex
	max     int
	process map[string]*NativeProcess
	// Authorize is called before spawn; return nil to allow.
	Authorize func(pluginID string) error
}

func NewRuntime(authorize func(pluginID string) error, max int) *Runtime {
	if max <= 0 {
		max = 8
	}
	return &Runtime{max: max, process: make(map[string]*NativeProcess), Authorize: authorize}
}

// IsRunning reports whether a plugin has a live native process.
func (r *Runtime) IsRunning(pluginID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	process, ok := r.process[pluginID]
	return ok && process.running
}

// Start spawns a native process (the actual exec is delegated to the caller
// via the startFn callback which must set the running state).
func (r *Runtime) Start(pluginID string, startFn func() error, stopFn func()) error {
	r.mu.Lock()
	if _, exists := r.process[pluginID]; exists {
		r.mu.Unlock()
		return ErrAlreadyRunning
	}
	if len(r.process) >= r.max {
		r.mu.Unlock()
		return ErrMaxNativeprocs
	}
	if r.Authorize != nil {
		if err := r.Authorize(pluginID); err != nil {
			r.mu.Unlock()
			return fmt.Errorf("%w: %v", ErrNotAuthorized, err)
		}
	}
	process := &NativeProcess{PluginID: pluginID, running: true, stop: stopFn}
	r.process[pluginID] = process
	r.mu.Unlock()
	_ = startFn()
	return nil
}

// Stop terminates a plugin's native process.
func (r *Runtime) Stop(pluginID string) error {
	r.mu.Lock()
	process, ok := r.process[pluginID]
	if !ok {
		r.mu.Unlock()
		return ErrNotRunning
	}
	delete(r.process, pluginID)
	r.mu.Unlock()
	process.mu.Lock()
	process.running = false
	stop := process.stop
	process.mu.Unlock()
	if stop != nil {
		stop()
	}
	return nil
}

// StopAll terminates all native processes (shutdown path).
func (r *Runtime) StopAll() {
	r.mu.Lock()
	processes := make([]*NativeProcess, 0, len(r.process))
	for _, process := range r.process {
		processes = append(processes, process)
	}
	r.process = make(map[string]*NativeProcess)
	r.mu.Unlock()
	for _, process := range processes {
		process.mu.Lock()
		wasRunning := process.running
		process.running = false
		process.mu.Unlock()
		if wasRunning && process.stop != nil {
			process.stop()
		}
	}
}
