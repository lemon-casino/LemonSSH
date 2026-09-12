package main

import (
	"net/url"
	"sync"
	"time"

	windowowner "github.com/binaricat/netcatty/internal/terminal/windows"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const popupRoute = "/index.html#/terminal-popup"
const popupLease = 45 * time.Second

type PopupOpenResult struct {
	Success bool   `json:"success"`
	PopupID string `json:"popupId,omitempty"`
	Error   string `json:"error,omitempty"`
}
type popupRecord struct {
	identity *windowowner.Window
	window   *application.WebviewWindow
	config   map[string]any
	timer    *time.Timer
	lastSeen time.Time
}
type PopupWindowService struct {
	mu     sync.Mutex
	app    *application.App
	owner  *windowowner.Manager
	popups map[string]*popupRecord
}

func newPopupWindowService(app *application.App) *PopupWindowService {
	return &PopupWindowService{app: app, owner: windowowner.NewManager(), popups: make(map[string]*popupRecord)}
}
func popupWindowOptions(name string) application.WebviewWindowOptions {
	return application.WebviewWindowOptions{Name: name, Title: "LemonSSH", Width: 960, Height: 640, MinWidth: 640, MinHeight: 400, Frameless: true, EnableFileDrop: true, BackgroundColour: application.NewRGB(20, 23, 28), URL: popupRoute}
}
func (s *PopupWindowService) Open(payload map[string]any) PopupOpenResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.app == nil {
		return PopupOpenResult{Error: "application is not attached"}
	}
	identity, err := s.owner.Create(windowowner.RolePopup)
	if err != nil {
		return PopupOpenResult{Error: err.Error()}
	}
	_ = s.owner.BindRenderer(identity.ID, identity.Token, identity.ID)
	config := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		config[key] = value
	}
	config["popupId"] = identity.ID
	options := popupWindowOptions(identity.ID)
	options.URL = "/index.html?popupId=" + url.QueryEscape(identity.ID) + "&popupToken=" + url.QueryEscape(identity.Token) + "#/terminal-popup"
	win := s.app.Window.NewWithOptions(options)
	record := &popupRecord{identity: identity, window: win, config: config, lastSeen: time.Now()}
	s.popups[identity.ID] = record
	record.timer = time.AfterFunc(popupLease, func() { s.expire(identity.ID, record) })
	registerFileDrops(win)
	win.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if _, err := s.owner.Get(identity.ID, identity.Token); err != nil {
			return
		}
		if err := s.owner.CloseAttempt(identity.ID, identity.Token); err != nil {
			event.Cancel()
			return
		}
		s.remove(identity.ID, record, false)
	})
	return PopupOpenResult{Success: true, PopupID: identity.ID}
}

// GetConfig is a pull handshake: configuration cannot race the renderer's event listener.
// The per-window capability appears only in that window's URL, never app-wide events.
func (s *PopupWindowService) GetConfig(popupID, token string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	identity, err := s.owner.Get(popupID, token)
	if err != nil {
		return nil, err
	}
	if err = windowowner.ValidateRoleForSession(identity.Role); err != nil {
		return nil, err
	}
	record := s.popups[popupID]
	if record == nil {
		return nil, windowowner.ErrWindowNotFound
	}
	copy := make(map[string]any, len(record.config))
	for k, v := range record.config {
		copy[k] = v
	}
	return copy, nil
}
func (s *PopupWindowService) Heartbeat(popupID, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.owner.Get(popupID, token); err != nil {
		return err
	}
	record := s.popups[popupID]
	if record == nil {
		return windowowner.ErrWindowNotFound
	}
	record.lastSeen = time.Now()
	record.timer.Reset(popupLease)
	return nil
}
func (s *PopupWindowService) expire(id string, record *popupRecord) { s.remove(id, record, true) }
func (s *PopupWindowService) remove(id string, record *popupRecord, crashed bool) {
	s.mu.Lock()
	if s.popups[id] != record {
		s.mu.Unlock()
		return
	}
	if crashed && time.Since(record.lastSeen) < popupLease {
		record.timer.Reset(popupLease - time.Since(record.lastSeen))
		s.mu.Unlock()
		return
	}
	delete(s.popups, id)
	record.timer.Stop()
	if crashed {
		s.owner.CrashCleanup(id)
	} else {
		_ = s.owner.Destroy(id, record.identity.Token)
	}
	s.mu.Unlock()
	// Popup is a view onto a terminal; dropping its lease never terminates that terminal.
	s.app.Event.Emit("terminal:popup-closed", map[string]any{"popupId": id, "crashed": crashed})
	if crashed {
		record.window.Close()
	}
}
func (s *PopupWindowService) ServiceShutdown() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, record := range s.popups {
		record.timer.Stop()
		_ = s.owner.Destroy(id, record.identity.Token)
		delete(s.popups, id)
	}
	return nil
}
