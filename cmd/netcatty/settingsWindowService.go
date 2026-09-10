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
	mu          sync.Mutex
	app         *application.App
	created     bool
	painted     bool
	pendingShow bool
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
	s.painted = false
	s.pendingShow = false
}

// Preload creates the hidden settings window off the boot path.
func (s *SettingsWindowService) Preload() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCreated()
}

// Open shows the settings window. If its first paint has not happened yet
// (WebView2 renders nothing while hidden), the show is deferred until
// PaintReady so the user never sees an empty black/white frame.
func (s *SettingsWindowService) Open() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCreated()
	win, ok := s.app.Window.GetByName(settingsWindowName)
	if !ok {
		return true, nil
	}
	if s.painted {
		win.Show()
		win.Focus()
	} else {
		s.pendingShow = true
	}
	return true, nil
}

// PaintReady is called by the settings page after its first render.
func (s *SettingsWindowService) PaintReady() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.painted = true
	if s.pendingShow {
		s.pendingShow = false
		if win, ok := s.app.Window.GetByName(settingsWindowName); ok {
			win.Show()
			win.Focus()
		}
	}
	return true, nil
}

// Show force-shows the window (explicit request path).
func (s *SettingsWindowService) Show() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if win, ok := s.app.Window.GetByName(settingsWindowName); ok {
		win.Show()
		win.Focus()
	}
	return nil
}

// Close hides the window; the loaded page is kept for the next open.
func (s *SettingsWindowService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if win, ok := s.app.Window.GetByName(settingsWindowName); ok {
		win.Hide()
	}
	return nil
}
