package main

import (
	"context"
	"encoding/json"
	"sync"

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
	return &PluginService{store: pluginstore.New(), runtime: runtime, native: nativeRuntime, broker: broker}
}

func (s *PluginService) List() []*pluginstore.PackageRecord {
	return s.store.List()
}

func (s *PluginService) Install(pluginID, version, sha256Hex, manifestJSON string) (*pluginstore.PackageRecord, error) {
	return s.store.Install(pluginID, version, sha256Hex, []byte(manifestJSON))
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
	return s.runtime.Instantiate(context.Background(), pluginID, wasmBytes)
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

// GrantNative records a session-scoped companion.execute grant. StartNative
// remains fail-closed until this grant exists.
func (s *PluginService) GrantNative(pluginID string) error {
	if s.broker == nil {
		return nil
	}
	_, err := s.broker.Grant(pluginID, "companion.execute:write", permissions.LifetimeSession, 0)
	return err
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
