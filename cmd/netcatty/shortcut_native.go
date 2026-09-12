package main

import (
	"errors"
	"github.com/binaricat/netcatty/internal/terminal/shortcuts"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func newNativeShortcutService(toggle func()) *ShortcutService {
	service := newShortcutService()
	service.registerNative = func(raw string) (func() error, error) {
		return shortcuts.RegisterNative(raw, toggle, application.InvokeSync)
	}
	return service
}

func (s *ShortcutService) ServiceShutdown() error {
	result := s.Unregister()
	if !result.Success {
		return errors.New(result.Error)
	}
	return nil
}
