package main

import (
	"errors"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// trayPanelWindowName is the role name of the tray panel window
// (trayPanelWindowOptions). TrayService pushes snapshots to this window by
// name, so keep the two in sync.
const trayPanelWindowName = "tray-panel"

// trayPanelWindowOptions mirrors the settings window's dedicated-frameless
// shape at tray-panel scale: created hidden, shown only after the panel page
// reported first paint, and kept warm (close = hide) so reopening is instant.
func trayPanelWindowOptions() application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Name:        trayPanelWindowName,
		Title:       "LemonSSH",
		Width:       380,
		Height:      520,
		MinWidth:    300,
		MinHeight:   360,
		Frameless:   true,
		AlwaysOnTop: true,
		Windows: application.WindowsWindow{
			// A tray panel must not claim a taskbar slot (Windows).
			HiddenOnTaskbar: true,
		},
		Hidden:           true,
		BackgroundColour: application.NewRGB(20, 23, 28),
		URL:              "/index.html#/tray",
	}
}

// TrayPanelWindowService owns the #/tray panel window lifecycle: lazy
// creation, the PaintReady show handshake (WebView2 renders nothing while a
// window is hidden, identical to the settings window), hide-on-close and the
// show/hide hooks TrayService uses to mirror menu data onto the panel.
type TrayPanelWindowService struct {
	mu          sync.Mutex
	app         *application.App
	created     bool
	painted     bool
	pendingShow bool
	// onPanelShown fires whenever the panel window becomes visible;
	// TrayService re-pushes the menu-data snapshot so a freshly mounted
	// panel never waits for the next data change.
	onPanelShown func()
	// onPanelHideRequest fires when the window close is intercepted;
	// TrayService emits tray:panel:close-request so the panel UI reacts
	// before the window hides.
	onPanelHideRequest func()
}

func newTrayPanelWindowService(app *application.App) *TrayPanelWindowService {
	return &TrayPanelWindowService{app: app}
}

// setPanelShown wires the shown callback (main-process wiring only).
func (s *TrayPanelWindowService) setPanelShown(onPanelShown func()) {
	s.mu.Lock()
	s.onPanelShown = onPanelShown
	s.mu.Unlock()
}

// setPanelHideRequest wires the close-intercepted callback.
func (s *TrayPanelWindowService) setPanelHideRequest(onPanelHideRequest func()) {
	s.mu.Lock()
	s.onPanelHideRequest = onPanelHideRequest
	s.mu.Unlock()
}

// ensureCreated lazily creates the hidden panel window. Callers must hold
// s.mu (same contract as the settings window service). The show/closing
// callbacks are captured at registration: the wiring in main.go runs before
// any window exists, and the hook body must not re-enter s.mu because Show()
// can fire the WindowShow event while PaintReady still holds the lock.
func (s *TrayPanelWindowService) ensureCreated() {
	if s.created {
		if _, ok := s.app.Window.GetByName(trayPanelWindowName); ok {
			return
		}
	}
	onPanelShown := s.onPanelShown
	onPanelHideRequest := s.onPanelHideRequest
	win := s.app.Window.NewWithOptions(trayPanelWindowOptions())
	win.RegisterHook(events.Common.WindowShow, func(*application.WindowEvent) {
		if onPanelShown != nil {
			onPanelShown()
		}
	})
	// The panel stays warm: an OS/programmatic close hides the window and
	// lets the panel UI react first. Destroying it would force a full page
	// load on the next open.
	win.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if onPanelHideRequest != nil {
			onPanelHideRequest()
		}
		event.Cancel()
		win.Hide()
	})
	s.created = true
	s.painted = false
	s.pendingShow = false
}

// Preload creates the hidden panel window off the open path so the first
// open only pays the show, not the WebView bootstrap.
func (s *TrayPanelWindowService) Preload() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.app == nil {
		return
	}
	s.ensureCreated()
}

// Open shows the tray panel window. If the panel page has not reported its
// first paint yet, the show is deferred to PaintReady so the user never sees
// a blank frame.
func (s *TrayPanelWindowService) Open() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.app == nil {
		return false, errors.New("tray panel: application is not attached")
	}
	s.ensureCreated()
	win, ok := s.app.Window.GetByName(trayPanelWindowName)
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

// PaintReady is called by the panel page after its first render
// (LemonSSHBridge.notifyTrayPanelPaintReady) and completes a pending show.
func (s *TrayPanelWindowService) PaintReady() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.app == nil {
		return false, nil
	}
	s.painted = true
	if s.pendingShow {
		s.pendingShow = false
		if win, ok := s.app.Window.GetByName(trayPanelWindowName); ok {
			win.Show()
			win.Focus()
		}
	}
	return true, nil
}

// Hide hides the panel window; the loaded page is kept for the next open.
func (s *TrayPanelWindowService) Hide() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.app == nil {
		return false, nil
	}
	if win, ok := s.app.Window.GetByName(trayPanelWindowName); ok {
		win.Hide()
	}
	return true, nil
}
