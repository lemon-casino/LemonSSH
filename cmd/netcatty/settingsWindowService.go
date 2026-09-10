package main

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
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
	mu      sync.Mutex
	app     *application.App
	created bool
}

func newSettingsWindowService(app *application.App) *SettingsWindowService {
	return &SettingsWindowService{app: app}
}

func (s *SettingsWindowService) ensureCreated() {
	if s.created {
		if _, ok := s.app.Window.GetByName(settingsWindowName); ok {
			return
		}
	}
	win := s.app.Window.NewWithOptions(settingsWindowOptions())
	win.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		win.Hide()
	})
	s.created = true
}

func (s *SettingsWindowService) Preload() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCreated()
}

func (s *SettingsWindowService) Open() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCreated()
	if win, ok := s.app.Window.GetByName(settingsWindowName); ok {
		win.Show()
		win.Focus()
	}
	return true, nil
}

func (s *SettingsWindowService) Show() error {
	return nil
}

func (s *SettingsWindowService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if win, ok := s.app.Window.GetByName(settingsWindowName); ok {
		win.Hide()
	}
	return nil
}
