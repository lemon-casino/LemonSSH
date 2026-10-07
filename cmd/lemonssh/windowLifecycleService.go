package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// mainWindowName is the role name of the primary window (mainWindowOptions).
const mainWindowName = "main"

// Wails event names broadcast by WindowLifecycleService. The Wails runtime
// adapter (infrastructure/runtime/wails/wailsRuntimeClient.ts) maps them onto
// the LemonSSHBridge window surfaces (onWindowShown / onWindowWillHide /
// onWindowFocusRequested / onCheckDirtyEditors); keep the constants in sync.
const (
	windowShownEvent             = "window:shown"
	windowWillHideEvent          = "window:will-hide"
	windowFocusRequestedEvent    = "window:focus-requested"
	windowCheckDirtyEditorsEvent = "window:check-dirty-editors"
)

// dirtyEditorsCheckTimeout bounds the quit-guard renderer round trip. It
// mirrors the Electron-era 5s budget documented in
// application/app/useAppStartupEffects.ts: a wedged renderer must not strand
// the user with an unquittable app, so the guard fails open instead.
const dirtyEditorsCheckTimeout = 5 * time.Second

// mainWindowHandle narrows the window operations the service performs so the
// close policy can be exercised in tests without a native window.
type mainWindowHandle interface {
	Hide() application.Window
	Focus()
}

// dirtyEditorsQuery is one in-flight quit-guard round trip. The first answer
// wins; late or duplicate reports are dropped. The verdict is broadcast by
// closing done, so every concurrent close request sharing the query wakes
// immediately with the same answer instead of a single channel receive
// stranding the other waiters until the timeout.
type dirtyEditorsQuery struct {
	once     sync.Once
	done     chan struct{}
	hasDirty bool
}

func (q *dirtyEditorsQuery) deliver(hasDirty bool) {
	q.once.Do(func() {
		q.hasDirty = hasDirty
		close(q.done)
	})
}

// windowFlagResult is the shared payload of the SetCloseToTray/IsCloseToTray
// bindings.
type windowFlagResult struct {
	Success bool `json:"success"`
	Enabled bool `json:"enabled"`
}

// windowCloseResult reports how RequestClose routed a renderer close request.
type windowCloseResult struct {
	Success bool `json:"success"`
	// Guarded is true when the request ran the main-window quit guard
	// (close-to-tray / dirty-editor confirmation) instead of closing a named
	// window directly.
	Guarded bool `json:"guarded"`
}

// WindowLifecycleService owns main-window close semantics for the Wails shell:
// the quit guard (unsaved editors / running transfers), close-to-tray, window
// opacity, and the show/hide/focus events the renderer's input-focus recovery
// subscribes to. It is registered as a Wails service; SetCloseToTray,
// IsCloseToTray, SetWindowOpacity, ReportDirtyEditorsResult and RequestClose
// are renderer-callable bindings.
type WindowLifecycleService struct {
	mu                sync.Mutex
	app               *application.App
	mainWindow        mainWindowHandle
	emit              func(name string)
	quit              func()
	closeToTray       bool
	quitting          bool
	pendingDirtyQuery *dirtyEditorsQuery
	dirtyCheckTimeout time.Duration
	// closeWindow overrides the named-window close for tests; nil uses
	// app.Window.GetByName(...).Close().
	closeWindow func(name string) bool
}

// newWindowLifecycleService wires the main-window hooks. Hooks run ahead of
// the framework's built-in WindowClosing teardown listener, so cancelling the
// event inside the hook keeps the window alive and lets this service decide
// between hide-to-tray, staying open, and quitting (the same mechanism the
// settings window uses to hide instead of close).
func newWindowLifecycleService(app *application.App, mainWindow *application.WebviewWindow) *WindowLifecycleService {
	s := &WindowLifecycleService{
		app:        app,
		mainWindow: mainWindow,
		// Renderer default (useSettingsState.ts): close-to-tray on. The
		// persisted renderer value re-syncs this via SetCloseToTray on mount,
		// so a close that lands before first paint falls on the safe side.
		closeToTray:       true,
		dirtyCheckTimeout: dirtyEditorsCheckTimeout,
	}
	// emitToMainWindow delivers a Wails custom event to the main window's
	// renderer only. app.Event.Emit would broadcast to every window, and peer
	// session windows (#/session-window) mount the same App shell — a session
	// window must never answer the main window's quit-guard query or react to
	// its focus recovery.
	emitToMainWindow := func(name string) {
		if app == nil {
			return
		}
		window, ok := app.Window.GetByName(mainWindowName)
		if !ok {
			return
		}
		window.DispatchWailsEvent(&application.CustomEvent{Name: name})
	}
	s.emit = emitToMainWindow
	s.quit = func() {
		if app != nil {
			app.Quit()
		}
	}
	if mainWindow != nil {
		mainWindow.OnWindowEvent(events.Common.WindowShow, func(*application.WindowEvent) {
			s.emit(windowShownEvent)
		})
		mainWindow.OnWindowEvent(events.Common.WindowHide, func(*application.WindowEvent) {
			s.emit(windowWillHideEvent)
		})
		mainWindow.OnWindowEvent(events.Common.WindowFocus, func(*application.WindowEvent) {
			s.emit(windowFocusRequestedEvent)
		})
		mainWindow.RegisterHook(events.Common.WindowClosing, func(windowEvent *application.WindowEvent) {
			s.interceptWindowClose(windowEvent)
		})
	}
	return s
}

// interceptWindowClose is the Common.WindowClosing hook on the main window.
// It always cancels the framework teardown: every continuation path issues its
// own action (hide to tray, stay open, or quit the process).
func (s *WindowLifecycleService) interceptWindowClose(windowEvent *application.WindowEvent) {
	windowEvent.Cancel()
	s.handleCloseRequest()
}

// handleCloseRequest resolves a user-initiated close of the main window under
// the current settings:
//   - close-to-tray enabled → hide the window (sessions keep running);
//   - otherwise → ask the renderer whether unsaved editors or unfinished
//     transfers exist; a dirty answer keeps the window open (the renderer has
//     already surfaced the quit confirm), a clean answer quits the app.
func (s *WindowLifecycleService) handleCloseRequest() {
	s.mu.Lock()
	if s.quitting {
		s.mu.Unlock()
		return
	}
	if s.closeToTray {
		s.mu.Unlock()
		if s.mainWindow != nil {
			s.mainWindow.Hide()
		}
		return
	}
	query := s.pendingDirtyQuery
	owner := false
	if query == nil {
		query = &dirtyEditorsQuery{done: make(chan struct{})}
		s.pendingDirtyQuery = query
		owner = true
	}
	timeout := s.dirtyCheckTimeout
	s.mu.Unlock()

	if owner {
		if s.emit != nil {
			s.emit(windowCheckDirtyEditorsEvent)
		}
	}

	var hasDirty bool
	select {
	case <-query.done:
		hasDirty = query.hasDirty
	case <-time.After(timeout):
		if owner {
			// Fail open: a dead renderer must not trap the user. Losing
			// unsaved work to a 5s timeout equals the documented Electron
			// behaviour of quitting anyway after the timeout.
			hasDirty = false
		} else {
			// A joined duplicate close never overrules the outcome the
			// originating waiter already computed.
			return
		}
	}

	s.mu.Lock()
	if s.pendingDirtyQuery == query {
		s.pendingDirtyQuery = nil
	}
	// Every waiter of a shared query reaches this point with the same
	// verdict; only the first confirmed-clean one transitions quitting so
	// the quit hook fires exactly once per query.
	quitApp := !hasDirty && !s.quitting
	if quitApp {
		s.quitting = true
	}
	s.mu.Unlock()

	if quitApp && s.quit != nil {
		s.quit()
	}
}

// ReportDirtyEditorsResult resolves the pending quit-guard query. The
// renderer listens for window:check-dirty-editors and reports back through
// this binding (LemonSSHBridge.reportDirtyEditorsResult).
func (s *WindowLifecycleService) ReportDirtyEditorsResult(hasDirty bool) {
	s.mu.Lock()
	query := s.pendingDirtyQuery
	s.mu.Unlock()
	if query != nil {
		query.deliver(hasDirty)
	}
}

// RequestClose resolves a renderer-initiated close for the calling window.
// The renderer passes its own Wails window name: "main" (or empty) runs the
// quit guard — close-to-tray hides, otherwise the dirty-editor round trip
// decides between staying open and quitting. Any other name closes that
// window directly; the window's own WindowClosing hooks keep their policy
// (settings hides to stay warm, popups honour the owner close-attempt gate),
// so this method never bypasses an existing close contract.
func (s *WindowLifecycleService) RequestClose(window string) (windowCloseResult, error) {
	name := strings.TrimSpace(window)
	if name == "" || name == mainWindowName {
		s.handleCloseRequest()
		return windowCloseResult{Success: true, Guarded: true}, nil
	}
	s.mu.Lock()
	closeWindow := s.closeWindow
	s.mu.Unlock()
	if closeWindow == nil {
		s.mu.Lock()
		app := s.app
		s.mu.Unlock()
		if app == nil {
			return windowCloseResult{}, errors.New("window lifecycle: application is not attached")
		}
		closeWindow = func(target string) bool {
			found, ok := app.Window.GetByName(target)
			if !ok {
				return false
			}
			found.Close()
			return true
		}
	}
	if !closeWindow(name) {
		return windowCloseResult{}, fmt.Errorf("window lifecycle: window %q not found", name)
	}
	return windowCloseResult{Success: true}, nil
}

// SetCloseToTray syncs the renderer's close-to-tray setting into the shell.
func (s *WindowLifecycleService) SetCloseToTray(enabled bool) (windowFlagResult, error) {
	s.mu.Lock()
	s.closeToTray = enabled
	s.mu.Unlock()
	return windowFlagResult{Success: true, Enabled: enabled}, nil
}

// IsCloseToTray reports the shell's current close-to-tray setting.
func (s *WindowLifecycleService) IsCloseToTray() (windowFlagResult, error) {
	s.mu.Lock()
	enabled := s.closeToTray
	s.mu.Unlock()
	return windowFlagResult{Success: true, Enabled: enabled}, nil
}

// SetWindowOpacity applies whole-window transparency to the main window.
// Returns false when the platform cannot honour it (unsupported platform or
// no main window); the renderer surfaces that as a skipped apply.
func (s *WindowLifecycleService) SetWindowOpacity(opacity float64) (bool, error) {
	s.mu.Lock()
	app := s.app
	s.mu.Unlock()
	if app == nil {
		return false, nil
	}
	window, ok := app.Window.GetByName(mainWindowName)
	if !ok {
		return false, nil
	}
	// NativeWindow (the HWND on Windows) lives on the concrete webview
	// window; GetByName returns the wider Window interface.
	webview, isWebview := window.(*application.WebviewWindow)
	if !isWebview {
		return false, nil
	}
	return applyWindowOpacity(webview, opacity), nil
}

// windowOpacityPlan converts a normalized opacity into layered-window
// parameters. opacity must be within [0,1] (NaN fails both bounds checks).
// keepLayered reports whether the WS_EX_LAYERED style must remain set.
func windowOpacityPlan(opacity float64) (alpha uint32, keepLayered bool, ok bool) {
	if !(opacity >= 0 && opacity <= 1) {
		return 0, false, false
	}
	if opacity >= 1 {
		return 255, false, true
	}
	return uint32(opacity*255.0 + 0.5), true, true
}
