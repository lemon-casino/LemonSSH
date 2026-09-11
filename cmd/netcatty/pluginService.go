package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/binaricat/netcatty/internal/plugin/manifest"
	"github.com/binaricat/netcatty/internal/plugin/native"
	"github.com/binaricat/netcatty/internal/plugin/permissions"
	pluginstore "github.com/binaricat/netcatty/internal/plugin/store"
	"github.com/binaricat/netcatty/internal/plugin/wasm"
)

type PluginService struct {
	mu      sync.Mutex
	store   *pluginstore.Store
	runtime *wasm.Runtime
	native  *native.Runtime
	broker  *permissions.Broker
}

func newPluginService() *PluginService {
	runtime, _ := wasm.NewRuntime(context.Background())
	broker := permissions.NewBroker(nil)
	nativeRuntime := native.NewRuntime(nil, 8)
	nativeRuntime.SetBroker(broker)
	service := &PluginService{store: pluginstore.New(), runtime: runtime, native: nativeRuntime, broker: broker}
	// Recovery: staged installs from an interrupted publish never run.
	service.store.RecoverStaged()
	return service
}

func (s *PluginService) List() []*pluginstore.PackageRecord {
	return s.store.List()
}

// parseManifestV2 unmarshals and validates a manifest v2 document.
func parseManifestV2(manifestJSON string) (manifest.Manifest, error) {
	var parsed manifest.Manifest
	if err := json.Unmarshal([]byte(manifestJSON), &parsed); err != nil {
		return manifest.Manifest{}, fmt.Errorf("manifest json: %w", err)
	}
	if err := manifest.Validate(parsed); err != nil {
		return manifest.Manifest{}, err
	}
	return parsed, nil
}

func (s *PluginService) Install(pluginID, version, sha256Hex, manifestJSON string) (*pluginstore.PackageRecord, error) {
	// The manifest is the contract: an invalid v2 document never enters the
	// store, so the WASM/native runtimes can trust the stored snapshot.
	if _, err := parseManifestV2(manifestJSON); err != nil {
		return nil, err
	}
	return s.store.Install(pluginID, version, sha256Hex, json.RawMessage(manifestJSON))
}

// StageInstall begins a two-phase publish: validate, stage, then commit.
func (s *PluginService) StageInstall(pluginID, version, sha256Hex, manifestJSON string) (*pluginstore.PackageRecord, error) {
	if _, err := parseManifestV2(manifestJSON); err != nil {
		return nil, err
	}
	return s.store.StageInstall(pluginID, version, sha256Hex, json.RawMessage(manifestJSON))
}

// CommitStaged promotes a staged plugin to installed.
func (s *PluginService) CommitStaged(pluginID string) (*pluginstore.PackageRecord, error) {
	return s.store.CommitStaged(pluginID)
}

// RecoverStaged drops installs that never committed (interrupted publish).
func (s *PluginService) RecoverStaged() int {
	return s.store.RecoverStaged()
}

func (s *PluginService) SetEnabled(pluginID string, enabled bool) error {
	state := pluginstore.StateDisabled
	if enabled {
		state = pluginstore.StateEnabled
	}
	return s.store.SetState(pluginID, state)
}

func (s *PluginService) InstantiateWASM(pluginID string, wasmBytes []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtime == nil {
		runtime, err := wasm.NewRuntime(context.Background())
		if err != nil {
			return err
		}
		s.runtime = runtime
	}
	config := wasm.Config{PluginID: pluginID, WASMBytes: wasmBytes}
	// The stored manifest caps linear memory so a plugin cannot grow past the
	// entrypoint.memoryMB it declared at install time.
	if record, ok := s.store.Get(pluginID); ok {
		var parsed manifest.Manifest
		if err := json.Unmarshal(record.Manifest, &parsed); err == nil && parsed.Entrypoint.MemoryMB > 0 {
			config.MemoryLimitPages = wasm.MemoryPagesFromMB(parsed.Entrypoint.MemoryMB)
		}
	}
	return s.runtime.InstantiateWithConfig(context.Background(), config)
}

// GrantNative records a session-scoped companion.execute grant. StartNative
// remains fail-closed until this grant exists.
func (s *PluginService) GrantNative(pluginID string) error {
	if s.broker == nil {
		return nil
	}
	_, err := s.broker.Grant(pluginID, "companion.execute:write", permissions.LifetimeSession, 0)
	return err
}

// NativeStartRequest is the Wails-facing native plugin spawn payload.
type NativeStartRequest struct {
	PluginID   string            `json:"pluginId"`
	BinaryPath string            `json:"binaryPath"`
	SHA256     string            `json:"sha256"`
	Args       []string          `json:"args"`
	WorkDir    string            `json:"workDir"`
	Env        map[string]string `json:"env"`
}

// StartNative verifies, authorizes and spawns a contained native plugin process.
func (s *PluginService) StartNative(request NativeStartRequest) error {
	return s.native.Start(context.Background(), native.Spec{
		PluginID:   request.PluginID,
		BinaryPath: request.BinaryPath,
		SHA256:     request.SHA256,
		Args:       request.Args,
		WorkDir:    request.WorkDir,
		Env:        request.Env,
	})
}

// StopNative terminates a native plugin process.
func (s *PluginService) StopNative(pluginID string) error {
	return s.native.Stop(pluginID)
}

// CallNative sends one framed RPC request to a live native plugin.
func (s *PluginService) CallNative(pluginID, method, paramsJSON string) (string, error) {
	var params json.RawMessage
	if paramsJSON != "" {
		params = json.RawMessage(paramsJSON)
	}
	result, err := s.native.Call(context.Background(), pluginID, method, params)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// NativeRunning reports whether a native plugin process is live.
func (s *PluginService) NativeRunning(pluginID string) bool {
	return s.native.IsRunning(pluginID)
}
