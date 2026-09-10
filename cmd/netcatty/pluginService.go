package main

import pluginstore "github.com/binaricat/netcatty/internal/plugin/store"

type PluginService struct {
	store *pluginstore.Store
}

func newPluginService() *PluginService {
	return &PluginService{store: pluginstore.New()}
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
