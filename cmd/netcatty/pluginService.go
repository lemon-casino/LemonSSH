package main

import (
	"context"
	"sync"

	pluginstore "github.com/binaricat/netcatty/internal/plugin/store"
	"github.com/binaricat/netcatty/internal/plugin/wasm"
)

type PluginService struct {
	mu      sync.Mutex
	store   *pluginstore.Store
	runtime *wasm.Runtime
}

func newPluginService() *PluginService {
	runtime, _ := wasm.NewRuntime(context.Background())
	return &PluginService{store: pluginstore.New(), runtime: runtime}
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
