package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const probeScheme = "netcatty-wails-probe"

var probeRoles = map[string]struct{}{
	"main": {}, "settings": {}, "session": {}, "popup": {},
}

type ProbeEvent struct {
	Kind      string `json:"kind"`
	Detail    string `json:"detail"`
	Timestamp string `json:"timestamp"`
}

type WindowStatus struct {
	Role              string `json:"role"`
	Open              bool   `json:"open"`
	Visible           bool   `json:"visible"`
	Focused           bool   `json:"focused"`
	CloseVeto         bool   `json:"closeVeto"`
	NativeHandleReady bool   `json:"nativeHandleReady"`
}

type ProbeSnapshot struct {
	Platform string         `json:"platform"`
	Windows  []WindowStatus `json:"windows"`
	Events   []ProbeEvent   `json:"events"`
}

type ProbeService struct {
	mu        sync.RWMutex
	windowMu  sync.Mutex
	app       *application.App
	tokens    map[string]string
	closeVeto map[string]bool
	closing   map[uint]bool
	events    []ProbeEvent
}

func NewProbeService() *ProbeService {
	return &ProbeService{
		tokens:    make(map[string]string),
		closeVeto: make(map[string]bool),
		closing:   make(map[uint]bool),
	}
}

func (p *ProbeService) attach(app *application.App) {
	p.mu.Lock()
	p.app = app
	p.mu.Unlock()
}

func (p *ProbeService) OpenWindow(callerRole, callerToken, targetRole string) error {
	targetRole, err := p.authorize(callerRole, callerToken, targetRole)
	if err != nil {
		return err
	}
	_, err = p.openWindow(targetRole)
	return err
}

func (p *ProbeService) openWindow(role string) (application.Window, error) {
	role, err := normalizeRole(role)
	if err != nil {
		return nil, err
	}
	app, err := p.application()
	if err != nil {
		return nil, err
	}
	p.windowMu.Lock()
	defer p.windowMu.Unlock()
	name := p.windowName(role)
	if window, exists := app.Window.GetByName(name); exists {
		showAndFocus(window)
		p.record("window-focus", role)
		return window, nil
	}

	token := p.token(role)
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             name,
		Title:            fmt.Sprintf("Netcatty shell probe - %s", role),
		Width:            1080,
		Height:           720,
		MinWidth:         760,
		MinHeight:        520,
		BackgroundColour: application.NewRGB(20, 23, 28),
		URL:              fmt.Sprintf("/?role=%s&token=%s", url.QueryEscape(role), url.QueryEscape(token)),
	})
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		p.mu.RLock()
		veto := p.closeVeto[role]
		p.mu.RUnlock()
		if veto {
			event.Cancel()
			p.record("close-veto", role)
			return
		}
		p.record("close-approved", role)
		if p.beginCloseObservation(window.ID()) {
			go p.recordWindowRemoval(role, window.ID())
		}
	})
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		p.record("runtime-ready", role)
	})
	window.Show()
	p.record("window-open", role)
	return window, nil
}

func (p *ProbeService) FocusWindow(callerRole, callerToken, targetRole string) error {
	targetRole, err := p.authorize(callerRole, callerToken, targetRole)
	if err != nil {
		return err
	}
	window, err := p.findWindow(targetRole)
	if err != nil {
		return err
	}
	showAndFocus(window)
	p.record("window-focus", targetRole)
	return nil
}

func (p *ProbeService) CloseWindow(callerRole, callerToken, targetRole string) error {
	targetRole, err := p.authorize(callerRole, callerToken, targetRole)
	if err != nil {
		return err
	}
	window, err := p.findWindow(targetRole)
	if err != nil {
		return err
	}
	window.Close()
	return nil
}

func (p *ProbeService) ReloadWindow(callerRole, callerToken, targetRole string) error {
	targetRole, err := p.authorize(callerRole, callerToken, targetRole)
	if err != nil {
		return err
	}
	window, err := p.findWindow(targetRole)
	if err != nil {
		return err
	}
	window.Reload()
	p.record("reload-request", targetRole)
	return nil
}

func (p *ProbeService) SetCloseVeto(callerRole, callerToken, targetRole string, enabled bool) error {
	targetRole, err := p.authorize(callerRole, callerToken, targetRole)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.closeVeto[targetRole] = enabled
	p.mu.Unlock()
	p.record("close-veto-change", fmt.Sprintf("%s=%t", targetRole, enabled))
	return nil
}

func (p *ProbeService) ShowNativeDialog(callerRole, callerToken, targetRole string) error {
	targetRole, err := p.authorize(callerRole, callerToken, targetRole)
	if err != nil {
		return err
	}
	window, err := p.findWindow(targetRole)
	if err != nil {
		return err
	}
	if window.NativeWindow() == nil {
		return errors.New("native window handle is not ready")
	}
	app, _ := p.application()
	app.Dialog.Info().
		SetTitle("Native handle probe").
		SetMessage("This dialog is parented to the selected Wails window.").
		AttachToWindow(window).
		Show()
	p.record("native-dialog", targetRole)
	return nil
}

func (p *ProbeService) Snapshot(callerRole, callerToken string) (ProbeSnapshot, error) {
	if _, err := p.authorize(callerRole, callerToken, callerRole); err != nil {
		return ProbeSnapshot{}, err
	}
	app, _ := p.application()
	statuses := make([]WindowStatus, 0, len(probeRoles))
	for _, role := range sortedRoles() {
		status := WindowStatus{Role: role}
		p.mu.RLock()
		status.CloseVeto = p.closeVeto[role]
		p.mu.RUnlock()
		if app != nil {
			if window, exists := app.Window.GetByName(p.windowName(role)); exists {
				status.Open = true
				status.Visible = window.IsVisible()
				status.Focused = window.IsFocused()
				status.NativeHandleReady = window.NativeWindow() != nil
			}
		}
		statuses = append(statuses, status)
	}
	p.mu.RLock()
	eventsCopy := append([]ProbeEvent(nil), p.events...)
	p.mu.RUnlock()
	return ProbeSnapshot{Platform: runtime.GOOS + "/" + runtime.GOARCH, Windows: statuses, Events: eventsCopy}, nil
}

func (p *ProbeService) handleSecondInstance(data application.SecondInstanceData) {
	detail := fmt.Sprintf("cwd=%s args=%s", data.WorkingDir, strings.Join(data.Args, " | "))
	p.record("second-instance", detail)
	if deepLink, ok := findProbeDeepLink(data.Args); ok {
		p.handleDeepLink(deepLink, "second-instance")
	}
	if _, err := p.openWindow("main"); err != nil {
		p.record("window-open-error", err.Error())
	}
}

func (p *ProbeService) handleDeepLink(value, source string) {
	if !isProbeDeepLink(value) {
		p.record("deep-link-rejected", source+": "+value)
		return
	}
	p.record("deep-link", source+": "+value)
	if _, err := p.openWindow("main"); err != nil {
		p.record("window-open-error", err.Error())
	}
}

func (p *ProbeService) record(kind, detail string) {
	event := ProbeEvent{Kind: kind, Detail: detail, Timestamp: time.Now().UTC().Format(time.RFC3339Nano)}
	p.mu.Lock()
	p.events = append(p.events, event)
	if len(p.events) > 100 {
		p.events = append([]ProbeEvent(nil), p.events[len(p.events)-100:]...)
	}
	app := p.app
	p.mu.Unlock()
	if app != nil {
		app.Event.Emit("probe:event", event)
	}
}

func (p *ProbeService) application() (*application.App, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.app == nil {
		return nil, errors.New("probe application is not attached")
	}
	return p.app, nil
}

func (p *ProbeService) findWindow(role string) (application.Window, error) {
	role, err := normalizeRole(role)
	if err != nil {
		return nil, err
	}
	app, err := p.application()
	if err != nil {
		return nil, err
	}
	window, exists := app.Window.GetByName(p.windowName(role))
	if !exists {
		return nil, fmt.Errorf("%s window is not open", role)
	}
	return window, nil
}

func (p *ProbeService) authorize(callerRole, callerToken, targetRole string) (string, error) {
	callerRole, err := normalizeRole(callerRole)
	if err != nil {
		return "", err
	}
	targetRole, err = normalizeRole(targetRole)
	if err != nil {
		return "", err
	}
	if callerToken == "" || callerToken != p.token(callerRole) {
		return "", errors.New("invalid window role token")
	}
	if callerRole != "main" && callerRole != targetRole {
		return "", errors.New("only the main window may control another role")
	}
	return targetRole, nil
}

func (p *ProbeService) recordWindowRemoval(role string, windowID uint) {
	defer p.finishCloseObservation(windowID)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ticker.C:
			app, err := p.application()
			if err != nil {
				return
			}
			if _, exists := app.Window.GetByID(windowID); !exists {
				p.record("window-closed", role)
				return
			}
		case <-timer.C:
			p.record("window-close-timeout", role)
			return
		}
	}
}

func (p *ProbeService) beginCloseObservation(windowID uint) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closing[windowID] {
		return false
	}
	p.closing[windowID] = true
	return true
}

func (p *ProbeService) finishCloseObservation(windowID uint) {
	p.mu.Lock()
	delete(p.closing, windowID)
	p.mu.Unlock()
}

func (p *ProbeService) token(role string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if token := p.tokens[role]; token != "" {
		return token
	}
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		panic(fmt.Errorf("create role token: %w", err))
	}
	p.tokens[role] = hex.EncodeToString(bytes)
	return p.tokens[role]
}

func (p *ProbeService) windowName(role string) string {
	return "probe-" + role + "-" + p.token(role)
}

func normalizeRole(role string) (string, error) {
	role = strings.ToLower(strings.TrimSpace(role))
	if _, exists := probeRoles[role]; !exists {
		return "", fmt.Errorf("unsupported window role %q", role)
	}
	return role, nil
}

func sortedRoles() []string {
	roles := make([]string, 0, len(probeRoles))
	for role := range probeRoles {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	return roles
}

func findProbeDeepLink(args []string) (string, bool) {
	for _, arg := range args {
		if isProbeDeepLink(arg) {
			return arg, true
		}
	}
	return "", false
}

func isProbeDeepLink(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && strings.EqualFold(parsed.Scheme, probeScheme) && parsed.Host != ""
}

func showAndFocus(window application.Window) {
	window.Show()
	window.Restore()
	window.Focus()
}
