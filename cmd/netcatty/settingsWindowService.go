package main

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const settingsWindowName = "settings"

func settingsWindowOptions() application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Name:             settingsWindowName,
		Title:            "LemonSSH Settings",
		Width:            980,
		Height:           720,
		MinWidth:         820,
		MinHeight:        600,
		Frameless:        true,
		Hidden:           true,
		BackgroundColour: application.NewRGB(20, 23, 28),
		URL:              "/index.html#/settings",
	}
}

type SettingsWindowService struct {
	mu  sync.Mutex
	app *application.App
}

func newSettingsWindowService(app *application.App) *SettingsWindowService {
	return &SettingsWindowService{app: app}
}

func (s *SettingsWindowService) Open() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.app.Window.GetByName(settingsWindowName); ok {
		return true, nil
	}
	s.app.Window.NewWithOptions(settingsWindowOptions())
	return true, nil
}

func (s *SettingsWindowService) Show() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	win, ok := s.app.Window.GetByName(settingsWindowName)
	if !ok {
		return nil
	}
	win.Show()
	win.Focus()
	return nil
}

func (s *SettingsWindowService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	win, ok := s.app.Window.GetByName(settingsWindowName)
	if !ok {
		return nil
	}
	win.Close()
	return nil
}
