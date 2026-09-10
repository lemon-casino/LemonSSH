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
