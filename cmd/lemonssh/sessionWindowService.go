package main

import (
	"net/url"
	"sync"
	"time"

	windowowner "github.com/binaricat/lemonssh/internal/terminal/windows"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Peer session windows (#/session-window) mount the same App shell as the
// main window and clone the requested session into their own tab (the
// frontend consumes the payload through createSessionFromCloneSource). The
// source tab stays in the main window — this is a "copy view to new window",
// not a detach, matching the tabs.copyTabToNewWindow* naming.
//
// Window identity rides the URL exactly like the terminal popup
// (popupWindowService.go): the per-window capability exists only in that
// window's URL, and the payload moves through the same pull handshake
// (GetConfig + Heartbeat) so a slow renderer can never miss a push event.
// The heartbeat lease doubles as crash cleanup: a renderer that stopped
// answering for a full lease closes instead of lingering as a blank window.
const sessionWindowLease = 45 * time.Second

type SessionWindowOpenResult struct {
	Success  bool   `json:"success"`
	WindowID string `json:"windowId,omitempty"`
	Error    string `json:"error,omitempty"`
}

type sessionWindowRecord struct {
	identity *windowowner.Window
	window   *application.WebviewWindow
	config   map[string]any
	timer    *time.Timer
	lastSeen time.Time
}

type SessionWindowService struct {
	mu      sync.Mutex
	app     *application.App
	owner   *windowowner.Manager
	windows map[string]*sessionWindowRecord
}

func newSessionWindowService(app *application.App) *SessionWindowService {
	return &SessionWindowService{app: app, owner: windowowner.NewManager(), windows: make(map[string]*sessionWindowRecord)}
}

// sessionWindowOptions sizes the peer window like the main shell (it renders
// the same App chrome), names it after the session-window identity so
// WindowLifecycleService.RequestClose can close it by name, and targets the
// #/session-window route.
func sessionWindowOptions(name, title string) application.WebviewWindowOptions {
	if title == "" {
		title = "LemonSSH"
	}
	return application.WebviewWindowOptions{
		Name:             name,
		Title:            title,
		Width:            1280,
		Height:           800,
		MinWidth:         960,
		MinHeight:        600,
		Frameless:        true,
		EnableFileDrop:   true,
		BackgroundColour: application.NewRGB(20, 23, 28),
		URL:              "/index.html#/session-window",
	}
}

// sessionWindowURL embeds the window identity into the query string. The
// renderer reads it back for the GetConfig/Heartbeat handshake and for
// closing itself through WindowLifecycleService.RequestClose.
func sessionWindowURL(id, token string) string {
	return "/index.html?sessionWindowId=" + url.QueryEscape(id) + "&sessionWindowToken=" + url.QueryEscape(token) + "#/session-window"
}

// Open creates a peer session window carrying the clone payload (title,
// sourceSession, localShellType). The payload is stored server-side; the new
// window pulls it via GetConfig once its renderer is up.
func (s *SessionWindowService) Open(payload map[string]any) SessionWindowOpenResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.app == nil {
		return SessionWindowOpenResult{Error: "application is not attached"}
	}
	identity, err := s.owner.Create(windowowner.RoleSession)
	if err != nil {
		return SessionWindowOpenResult{Error: err.Error()}
	}
	_ = s.owner.BindRenderer(identity.ID, identity.Token, identity.ID)
	config := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		config[key] = value
	}
	config["sessionWindowId"] = identity.ID
	title, _ := config["title"].(string)
	options := sessionWindowOptions(identity.ID, title)
	options.URL = sessionWindowURL(identity.ID, identity.Token)
	win := s.app.Window.NewWithOptions(options)
	setTaskbarIcon(win)
	registerFileDrops(win)
	record := &sessionWindowRecord{identity: identity, window: win, config: config, lastSeen: time.Now()}
	s.windows[identity.ID] = record
	record.timer = time.AfterFunc(sessionWindowLease, func() { s.expire(identity.ID, record) })
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
	return SessionWindowOpenResult{Success: true, WindowID: identity.ID}
}

// GetConfig is the pull handshake: configuration cannot race the renderer's
// event listener, and only the URL-authenticated window identity may read it.
func (s *SessionWindowService) GetConfig(windowID, token string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	identity, err := s.owner.Get(windowID, token)
	if err != nil {
		return nil, err
	}
	if err = windowowner.ValidateRoleForSession(identity.Role); err != nil {
		return nil, err
	}
	record := s.windows[windowID]
	if record == nil {
		return nil, windowowner.ErrWindowNotFound
	}
	copy := make(map[string]any, len(record.config))
	for k, v := range record.config {
		copy[k] = v
	}
	return copy, nil
}

// Heartbeat extends the window's lease; a renderer that stops answering
// (crash/hang) lets it lapse and the window closes itself.
func (s *SessionWindowService) Heartbeat(windowID, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.owner.Get(windowID, token); err != nil {
		return err
	}
	record := s.windows[windowID]
	if record == nil {
		return windowowner.ErrWindowNotFound
	}
	record.lastSeen = time.Now()
	record.timer.Reset(sessionWindowLease)
	return nil
}

func (s *SessionWindowService) expire(id string, record *sessionWindowRecord) { s.remove(id, record, true) }

func (s *SessionWindowService) remove(id string, record *sessionWindowRecord, crashed bool) {
	s.mu.Lock()
	if s.windows[id] != record {
		s.mu.Unlock()
		return
	}
	if crashed && time.Since(record.lastSeen) < sessionWindowLease {
		record.timer.Reset(sessionWindowLease - time.Since(record.lastSeen))
		s.mu.Unlock()
		return
	}
	delete(s.windows, id)
	record.timer.Stop()
	if crashed {
		s.owner.CrashCleanup(id)
	} else {
		_ = s.owner.Destroy(id, record.identity.Token)
	}
	s.mu.Unlock()
	// The clone session lives inside this window's renderer; closing the
	// window ends it. No home-window restore handshake is needed (unlike the
	// terminal popup, the source tab never left the main window).
	if crashed {
		record.window.Close()
	}
}

func (s *SessionWindowService) ServiceShutdown() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, record := range s.windows {
		record.timer.Stop()
		_ = s.owner.Destroy(id, record.identity.Token)
		delete(s.windows, id)
	}
	return nil
}
