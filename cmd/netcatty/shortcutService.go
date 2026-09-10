package main

import "github.com/binaricat/netcatty/internal/terminal/shortcuts"

type ShortcutService struct {
	registry *shortcuts.Registry
}

func newShortcutService() *ShortcutService {
	return &ShortcutService{registry: shortcuts.NewRegistry()}
}

func (s *ShortcutService) Register(raw, callbackID string) error {
	return s.registry.Register(raw, callbackID)
}

func (s *ShortcutService) Unregister(raw string) error {
	return s.registry.Unregister(raw)
}

func (s *ShortcutService) List() []string {
	return s.registry.List()
}
