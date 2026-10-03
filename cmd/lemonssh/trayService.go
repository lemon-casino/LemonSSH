package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// trayLabels carries the localized tray context-menu strings. The labels
// mirror the renderer's tray.* locale entries so the tray follows the
// Appearance language setting.
type trayLabels struct {
	Show           string
	Settings       string
	TrayPanel      string
	Sessions       string
	PortForwarding string
	Quit           string
}

var trayLabelsByLanguage = map[string]trayLabels{
	"en":    {Show: "Open Main Window", Settings: "Settings", TrayPanel: "Open Tray Panel", Sessions: "Sessions", PortForwarding: "Port Forwarding", Quit: "Quit"},
	"es":    {Show: "Abrir ventana principal", Settings: "Ajustes", TrayPanel: "Abrir panel de bandeja", Sessions: "Sesiones", PortForwarding: "Reenvío de puertos", Quit: "Salir"},
	"ru":    {Show: "Открыть главное окно", Settings: "Настройки", TrayPanel: "Открыть панель трея", Sessions: "Сессии", PortForwarding: "Проброс портов", Quit: "Выйти"},
	"zh-CN": {Show: "打开主窗口", Settings: "设置", TrayPanel: "打开托盘面板", Sessions: "会话", PortForwarding: "端口转发", Quit: "退出"},
	"zh-TW": {Show: "開啟主視窗", Settings: "設定", TrayPanel: "開啟托盤面板", Sessions: "工作階段", PortForwarding: "埠轉發", Quit: "結束"},
}

func trayLabelsFor(language string) trayLabels {
	if labels, ok := trayLabelsByLanguage[normalizeTrayLanguage(language)]; ok {
		return labels
	}
	return trayLabelsByLanguage["en"]
}

// normalizeTrayLanguage resolves a renderer locale id onto the supported
// set, accepting bare bases ("zh") and regional variants ("en-US").
func normalizeTrayLanguage(language string) string {
	trimmed := strings.TrimSpace(language)
	if labels := trayLabelsByLanguage[trimmed]; labels.Show != "" {
		return trimmed
	}
	base := trimmed
	if idx := strings.IndexAny(base, "-_"); idx > 0 {
		base = base[:idx]
	}
	if base == "zh" {
		return "zh-CN"
	}
	if _, ok := trayLabelsByLanguage[base]; ok {
		return base
	}
	prefixed := ""
	for id := range trayLabelsByLanguage {
		if strings.HasPrefix(id, base+"-") {
			prefixed = id
			break
		}
	}
	if prefixed != "" {
		return prefixed
	}
	return "en"
}

// TrayActions holds the main-window/settings/tray-panel launchers so the tray
// service stays decoupled from the window services.
type TrayActions struct {
	ShowMain      func()
	OpenSettings  func()
	OpenTrayPanel func()
}

// TrayForwards narrows the ForwardService surface the tray menu needs so the
// one-click rule toggle can be exercised in tests without a live SSH pool.
type TrayForwards interface {
	StopByRuleId(ruleID string) map[string]any
	List() []PortForwardListItem
}

// TraySessionState is one renderer session row pushed via UpdateTrayMenuData
// (LemonSSHBridge.updateTrayMenuData contract, types/global/
// lemonssh-bridge-app.d.ts). Status is connecting|connected|disconnected.
type TraySessionState struct {
	ID             string `json:"id"`
	Label          string `json:"label,omitempty"`
	HostLabel      string `json:"hostLabel,omitempty"`
	Status         string `json:"status,omitempty"`
	WorkspaceID    string `json:"workspaceId,omitempty"`
	WorkspaceTitle string `json:"workspaceTitle,omitempty"`
}

// TrayHostState is one vault host row (panel connect actions).
type TrayHostState struct {
	ID              string `json:"id"`
	Label           string `json:"label,omitempty"`
	Hostname        string `json:"hostname,omitempty"`
	Group           string `json:"group,omitempty"`
	Pinned          bool   `json:"pinned,omitempty"`
	LastConnectedAt int64  `json:"lastConnectedAt,omitempty"`
	Protocol        string `json:"protocol,omitempty"`
}

// TrayPortForwardRuleState is one port-forwarding rule row. Status is
// inactive|connecting|active|error; Type is local|remote|dynamic.
type TrayPortForwardRuleState struct {
	ID         string `json:"id"`
	Label      string `json:"label,omitempty"`
	Type       string `json:"type,omitempty"`
	LocalPort  uint16 `json:"localPort,omitempty"`
	RemoteHost string `json:"remoteHost,omitempty"`
	RemotePort uint16 `json:"remotePort,omitempty"`
	Status     string `json:"status,omitempty"`
}

// TrayMenuData is the renderer-pushed tray menu payload. The renderer is the
// single content authority (useAppStartupEffects re-pushes whenever its
// sessions / forward rules / hosts change or the main window is shown again);
// the shell stores it, rebuilds the menu and mirrors the snapshot onto the
// tray panel window.
type TrayMenuData struct {
	Sessions         []TraySessionState         `json:"sessions,omitempty"`
	Hosts            []TrayHostState            `json:"hosts,omitempty"`
	PortForwardRules []TrayPortForwardRuleState `json:"portForwardRules,omitempty"`
}

// trayMenuResult is the shared payload of the tray action bindings.
type trayMenuResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// trayForwardToggleResult reports what a tray rule toggle did: Running=false
// means the shell stopped the rule's live tunnels itself; Running=true means
// the start request was handed to the main-window renderer (credentials and
// vault host resolution live there, and the app-lock deferral guard runs
// renderer-side).
type trayForwardToggleResult struct {
	Success bool   `json:"success"`
	Running bool   `json:"running"`
	Error   string `json:"error,omitempty"`
}

// Wails event names dispatched by TrayService. The Wails runtime adapter
// (infrastructure/runtime/wails/wailsRuntimeClient.ts) maps them onto the
// LemonSSHBridge tray surfaces (onTrayFocusSession / onTrayTogglePortForward /
// onTrayPanelJumpToSession / ...); keep the constants in sync. Main-window
// events are delivered via window-targeted DispatchWailsEvent so peer session
// windows never react to them.
const (
	trayFocusSessionEvent      = "tray:focus-session"
	trayToggleForwardEvent     = "tray:toggle-port-forward"
	trayPanelJumpSessionEvent  = "tray:panel:jump-to-session"
	trayPanelConnectHostEvent  = "tray:panel:connect-to-host"
	trayPanelCloseSessionEvent = "tray:panel:close-session"
	trayPanelMenuDataEvent     = "tray:panel:menu-data"
	trayPanelRefreshEvent      = "tray:panel:refresh"
	trayPanelCloseRequestEvent = "tray:panel:close-request"
)

type restoreWindow interface {
	IsMinimised() bool
	UnMinimise()
	Show()
	Focus()
}

func restoreMainWindow(win restoreWindow) {
	if win == nil {
		return
	}
	if win.IsMinimised() {
		win.UnMinimise()
	}
	win.Show()
	win.Focus()
}

type appWindow interface {
	IsMinimised() bool
	UnMinimise()
	Show() application.Window
	Focus()
}

type appRestoreWindow struct {
	win appWindow
}

func (w appRestoreWindow) IsMinimised() bool {
	return w.win != nil && w.win.IsMinimised()
}

func (w appRestoreWindow) UnMinimise() {
	if w.win != nil {
		w.win.UnMinimise()
	}
}

func (w appRestoreWindow) Show() {
	if w.win != nil {
		w.win.Show()
	}
}

func (w appRestoreWindow) Focus() {
	if w.win != nil {
		w.win.Focus()
	}
}

// TrayService owns the tray context menu and rebuilds it whenever the
// renderer pushes new tray menu data (sessions / hosts / port-forward rules)
// or reports a new appearance language.
type TrayService struct {
	mu       sync.Mutex
	app      *application.App
	tray     *application.SystemTray
	actions  TrayActions
	language string
	quit     func()
	// data is the latest renderer-pushed menu content; the tray menu is a
	// pure projection of (language, data).
	data TrayMenuData
	// forwards stops live tunnels for one-click toggles; nil defers every
	// toggle to the main-window renderer.
	forwards TrayForwards
	// emitMainWindow / emitPanel override the window-targeted event
	// dispatch (tests); nil uses app.Window.GetByName + DispatchWailsEvent.
	emitMainWindow func(name string, payload any)
	emitPanel      func(name string, payload any)
}

func newTrayService(app *application.App, tray *application.SystemTray, actions TrayActions) *TrayService {
	return &TrayService{app: app, tray: tray, actions: actions, language: "en", quit: app.Quit}
}

// setForwards wires the ForwardService facade (unexported on purpose:
// main-process wiring, not a renderer-callable API).
func (s *TrayService) setForwards(forwards TrayForwards) {
	s.mu.Lock()
	s.forwards = forwards
	s.mu.Unlock()
}

// Quit terminates the application process (not merely hiding the window).
func (s *TrayService) Quit() {
	if s.quit != nil {
		s.quit()
	}
}

func (s *TrayService) initialMenu() *application.Menu {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buildMenu()
}

func (s *TrayService) buildMenu() *application.Menu {
	menu := application.NewMenu()
	appendTrayMenuEntries(menu, trayMenuPlan(s.language, s.data), s.bindTrayMenuEntry)
	return menu
}

// bindTrayMenuEntry attaches the shell action behind one menu row. The
// entry's identity is copied into the closure so late rebuilds never
// redirect an old click onto a new row.
func (s *TrayService) bindTrayMenuEntry(item *application.MenuItem, entry trayMenuEntry) {
	if handler := s.trayMenuHandler(entry); handler != nil {
		item.OnClick(handler)
	}
}

// trayMenuHandler builds the click handler for one menu row, shared by the
// bound menu items and the unit tests (MenuItem callbacks are unexported).
func (s *TrayService) trayMenuHandler(entry trayMenuEntry) func(*application.Context) {
	switch entry.action {
	case trayActionShowMain:
		return func(*application.Context) {
			if s.actions.ShowMain != nil {
				s.actions.ShowMain()
			}
		}
	case trayActionOpenSettings:
		return func(*application.Context) {
			if s.actions.OpenSettings != nil {
				s.actions.OpenSettings()
			}
		}
	case trayActionOpenPanel:
		return func(*application.Context) {
			if s.actions.OpenTrayPanel != nil {
				s.actions.OpenTrayPanel()
			}
		}
	case trayActionQuit:
		return func(*application.Context) {
			s.Quit()
		}
	case trayActionFocusSession:
		sessionID := entry.sessionID
		return func(*application.Context) {
			_, _ = s.FocusSession(sessionID)
		}
	case trayActionToggleForward:
		ruleID := entry.ruleID
		return func(*application.Context) {
			_, _ = s.TogglePortForward(ruleID)
		}
	default:
		return nil
	}
}

// SetLanguage switches the tray menu language and rebuilds it in place.
func (s *TrayService) SetLanguage(language string) bool {
	normalized := normalizeTrayLanguage(language)
	s.mu.Lock()
	changed := normalized != s.language
	s.language = normalized
	var menu *application.Menu
	if changed {
		menu = s.buildMenu()
	}
	s.mu.Unlock()
	if changed && s.tray != nil {
		s.tray.SetMenu(menu)
	}
	return changed
}

// trayMenuAction identifies the shell behaviour behind one tray menu row.
type trayMenuAction uint8

const (
	trayActionNone trayMenuAction = iota
	trayActionShowMain
	trayActionOpenSettings
	trayActionOpenPanel
	trayActionQuit
	trayActionFocusSession
	trayActionToggleForward
)

// trayMenuEntry is one row of the language- and data-dependent tray menu.
// It is pure data so the menu layout is unit-testable without a running app;
// appendTrayMenuEntries turns entries into Wails menu items.
type trayMenuEntry struct {
	kind      string // "item" | "checkbox" | "separator" | "submenu"
	label     string
	checked   bool
	action    trayMenuAction
	sessionID string
	ruleID    string
	children  []trayMenuEntry
}

// trayMenuPlan projects (language, data) onto the concrete menu rows:
// the base trio plus "Open Tray Panel", the active session list, the
// port-forwarding rule toggles, then Quit.
func trayMenuPlan(language string, data TrayMenuData) []trayMenuEntry {
	labels := trayLabelsFor(language)
	plan := []trayMenuEntry{
		{kind: "item", label: labels.Show, action: trayActionShowMain},
		{kind: "item", label: labels.Settings, action: trayActionOpenSettings},
		{kind: "item", label: labels.TrayPanel, action: trayActionOpenPanel},
	}
	sessions := trayMenuSessionEntries(data.Sessions)
	forwards := trayMenuForwardEntries(data.PortForwardRules)
	if len(sessions) > 0 || len(forwards) > 0 {
		plan = append(plan, trayMenuEntry{kind: "separator"})
		if len(sessions) > 0 {
			plan = append(plan, trayMenuEntry{
				kind:     "submenu",
				label:    fmt.Sprintf("%s (%d)", labels.Sessions, len(sessions)),
				children: sessions,
			})
		}
		if len(forwards) > 0 {
			plan = append(plan, trayMenuEntry{
				kind:     "submenu",
				label:    labels.PortForwarding,
				children: forwards,
			})
		}
	}
	plan = append(plan,
		trayMenuEntry{kind: "separator"},
		trayMenuEntry{kind: "item", label: labels.Quit, action: trayActionQuit},
	)
	return plan
}

// trayMenuSessionEntries renders the active (connected / connecting) sessions;
// disconnected rows are not jumpable, mirroring the tray panel's list.
func trayMenuSessionEntries(sessions []TraySessionState) []trayMenuEntry {
	entries := make([]trayMenuEntry, 0, len(sessions))
	for _, session := range sessions {
		if strings.TrimSpace(session.ID) == "" {
			continue
		}
		if session.Status != "connected" && session.Status != "connecting" {
			continue
		}
		label := session.HostLabel
		if strings.TrimSpace(label) == "" {
			label = session.Label
		}
		if strings.TrimSpace(label) == "" {
			label = session.ID
		}
		entries = append(entries, trayMenuEntry{
			kind:      "item",
			label:     label,
			action:    trayActionFocusSession,
			sessionID: session.ID,
		})
	}
	return entries
}

// trayMenuForwardEntries renders one checkbox row per rule; checked rows have
// a live tunnel. The label mirrors the tray panel's fallback rendering.
func trayMenuForwardEntries(rules []TrayPortForwardRuleState) []trayMenuEntry {
	entries := make([]trayMenuEntry, 0, len(rules))
	for _, rule := range rules {
		if strings.TrimSpace(rule.ID) == "" {
			continue
		}
		label := rule.Label
		if strings.TrimSpace(label) == "" {
			if rule.Type == "dynamic" {
				label = fmt.Sprintf("SOCKS:%d", rule.LocalPort)
			} else {
				label = fmt.Sprintf("%d → %s:%d", rule.LocalPort, rule.RemoteHost, rule.RemotePort)
			}
		}
		entries = append(entries, trayMenuEntry{
			kind:    "checkbox",
			label:   label,
			checked: rule.Status == "active" || rule.Status == "connecting",
			action:  trayActionToggleForward,
			ruleID:  rule.ID,
		})
	}
	return entries
}

// appendTrayMenuEntries materializes plan rows into menu. bind attaches each
// row's click action, so tests can drive the same pipeline the shell uses.
func appendTrayMenuEntries(menu *application.Menu, entries []trayMenuEntry, bind func(*application.MenuItem, trayMenuEntry)) {
	for _, entry := range entries {
		switch entry.kind {
		case "separator":
			menu.AddSeparator()
		case "submenu":
			submenu := menu.AddSubmenu(entry.label)
			appendTrayMenuEntries(submenu, entry.children, bind)
		case "checkbox":
			item := menu.AddCheckbox(entry.label, entry.checked)
			bind(item, entry)
		default:
			item := menu.Add(entry.label)
			bind(item, entry)
		}
	}
}

// normalizeTrayMenuData drops rows without an identity so stale renderer
// payloads cannot build unclickable menu entries.
func normalizeTrayMenuData(data TrayMenuData) TrayMenuData {
	normalized := TrayMenuData{}
	for _, session := range data.Sessions {
		if strings.TrimSpace(session.ID) == "" {
			continue
		}
		normalized.Sessions = append(normalized.Sessions, session)
	}
	for _, host := range data.Hosts {
		if strings.TrimSpace(host.ID) == "" {
			continue
		}
		normalized.Hosts = append(normalized.Hosts, host)
	}
	for _, rule := range data.PortForwardRules {
		if strings.TrimSpace(rule.ID) == "" {
			continue
		}
		normalized.PortForwardRules = append(normalized.PortForwardRules, rule)
	}
	return normalized
}

// UpdateTrayMenuData stores the renderer-pushed sessions / hosts / forward
// rules, rebuilds the tray menu and mirrors the snapshot onto the tray panel
// window. This is the single menu-content mechanism: the renderer re-pushes
// on session changes, rule start/stops and main-window re-shows, so the menu
// and the panel always render the same projection.
func (s *TrayService) UpdateTrayMenuData(data TrayMenuData) (trayMenuResult, error) {
	s.mu.Lock()
	s.data = normalizeTrayMenuData(data)
	menu := s.buildMenu()
	s.mu.Unlock()
	if s.tray != nil {
		s.tray.SetMenu(menu)
	}
	s.pushPanelSnapshot()
	return trayMenuResult{Success: true}, nil
}

// CurrentTrayMenuData reports the latest renderer-pushed menu data (panel
// bootstrap fallback).
func (s *TrayService) CurrentTrayMenuData() TrayMenuData {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneTrayMenuData(s.data)
}

// FocusSession restores the main window and asks its renderer to activate one
// session. Shared by the tray menu session rows and the panel's jump action
// path (JumpToSessionFromPanel emits the panel-specific twin event).
func (s *TrayService) FocusSession(sessionID string) (trayMenuResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return trayMenuResult{}, errors.New("tray: session id is required")
	}
	if s.actions.ShowMain != nil {
		s.actions.ShowMain()
	}
	s.emitToMainWindow(trayFocusSessionEvent, sessionID)
	return trayMenuResult{Success: true}, nil
}

// JumpToSessionFromPanel is the tray panel's jump entry point. It routes the
// request through the same focus pipeline as FocusSession, emitting the
// panel-scoped event the main window's tray-panel subscription listens for.
func (s *TrayService) JumpToSessionFromPanel(sessionID string) (trayMenuResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return trayMenuResult{}, errors.New("tray: session id is required")
	}
	if s.actions.ShowMain != nil {
		s.actions.ShowMain()
	}
	s.emitToMainWindow(trayPanelJumpSessionEvent, sessionID)
	return trayMenuResult{Success: true}, nil
}

// ConnectToHost restores the main window and asks its renderer to open one
// vault host (the panel's connect action; credential and vault handling stay
// renderer-side, including the app-lock connect queue).
func (s *TrayService) ConnectToHost(hostID string) (trayMenuResult, error) {
	hostID = strings.TrimSpace(hostID)
	if hostID == "" {
		return trayMenuResult{}, errors.New("tray: host id is required")
	}
	if s.actions.ShowMain != nil {
		s.actions.ShowMain()
	}
	s.emitToMainWindow(trayPanelConnectHostEvent, hostID)
	return trayMenuResult{Success: true}, nil
}

// CloseSessionFromPanel asks the main-window renderer to close one session.
// The main window stays where it is: closing a session from the tray panel
// must not yank focus.
func (s *TrayService) CloseSessionFromPanel(sessionID string) (trayMenuResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return trayMenuResult{}, errors.New("tray: session id is required")
	}
	s.emitToMainWindow(trayPanelCloseSessionEvent, sessionID)
	return trayMenuResult{Success: true}, nil
}

// TogglePortForward is the one-click rule toggle behind the README's tray
// port-forwarding promise. A rule with live tunnels is stopped natively via
// ForwardService.StopByRuleId; starting needs the renderer's vault host
// resolution and credentials (and honours its app-lock deferral), so the
// start request is dispatched to the main window whose existing tray
// toggle handler drives ForwardService.Start.
func (s *TrayService) TogglePortForward(ruleID string) (trayForwardToggleResult, error) {
	ruleID = strings.TrimSpace(ruleID)
	if ruleID == "" {
		return trayForwardToggleResult{}, errors.New("tray: rule id is required")
	}
	s.mu.Lock()
	forwards := s.forwards
	s.mu.Unlock()
	if forwards != nil && ruleHasLiveTunnel(forwards, ruleID) {
		forwards.StopByRuleId(ruleID)
		return trayForwardToggleResult{Success: true, Running: false}, nil
	}
	s.emitToMainWindow(trayToggleForwardEvent, map[string]any{"ruleId": ruleID, "start": true})
	return trayForwardToggleResult{Success: true, Running: true}, nil
}

// OpenMainWindow restores the main window (close-to-tray / minimised safe).
func (s *TrayService) OpenMainWindow() (trayMenuResult, error) {
	if s.actions.ShowMain == nil {
		return trayMenuResult{}, errors.New("tray: main window action is not wired")
	}
	s.actions.ShowMain()
	return trayMenuResult{Success: true}, nil
}

// ruleHasLiveTunnel reports whether the shell currently holds at least one
// tunnel for the rule.
func ruleHasLiveTunnel(forwards TrayForwards, ruleID string) bool {
	for _, item := range forwards.List() {
		if item.RuleID == ruleID {
			return true
		}
	}
	return false
}

// pushPanelSnapshot mirrors the current menu data onto the tray panel window.
// Called after every UpdateTrayMenuData and whenever the panel window shows,
// so a freshly mounted panel receives the snapshot after its subscriptions
// are live.
func (s *TrayService) pushPanelSnapshot() {
	s.mu.Lock()
	data := cloneTrayMenuData(s.data)
	s.mu.Unlock()
	s.emitToPanelWindow(trayPanelMenuDataEvent, data)
	s.emitToPanelWindow(trayPanelRefreshEvent, nil)
}

// notifyPanelCloseRequest tells the panel renderer the shell is about to hide
// the panel window (window close interception).
func (s *TrayService) notifyPanelCloseRequest() {
	s.emitToPanelWindow(trayPanelCloseRequestEvent, nil)
}

// emitToPanelWindow delivers one Wails event to the tray panel window only.
func (s *TrayService) emitToPanelWindow(name string, payload any) {
	s.mu.Lock()
	emit := s.emitPanel
	app := s.app
	s.mu.Unlock()
	if emit != nil {
		emit(name, payload)
		return
	}
	if app == nil {
		return
	}
	win, ok := app.Window.GetByName(trayPanelWindowName)
	if !ok {
		return
	}
	win.DispatchWailsEvent(&application.CustomEvent{Name: name, Data: payload})
}

// emitToMainWindow delivers one Wails event to the main window's renderer
// only (same targeting rule as WindowLifecycleService: peer session windows
// mount the same App shell and must never react to tray actions).
func (s *TrayService) emitToMainWindow(name string, payload any) {
	s.mu.Lock()
	emit := s.emitMainWindow
	app := s.app
	s.mu.Unlock()
	if emit != nil {
		emit(name, payload)
		return
	}
	if app == nil {
		return
	}
	win, ok := app.Window.GetByName(mainWindowName)
	if !ok {
		return
	}
	win.DispatchWailsEvent(&application.CustomEvent{Name: name, Data: payload})
}

func cloneTrayMenuData(data TrayMenuData) TrayMenuData {
	clone := TrayMenuData{
		Sessions:         make([]TraySessionState, len(data.Sessions)),
		Hosts:            make([]TrayHostState, len(data.Hosts)),
		PortForwardRules: make([]TrayPortForwardRuleState, len(data.PortForwardRules)),
	}
	copy(clone.Sessions, data.Sessions)
	copy(clone.Hosts, data.Hosts)
	copy(clone.PortForwardRules, data.PortForwardRules)
	return clone
}
