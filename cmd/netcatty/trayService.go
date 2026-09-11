package main

import (
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// trayLabels carries the localized tray context-menu strings. The labels
// mirror the renderer's tray.* locale entries so the tray follows the
// Appearance language setting.
type trayLabels struct {
	Show     string
	Settings string
	Quit     string
}

var trayLabelsByLanguage = map[string]trayLabels{
	"en":    {Show: "Open Main Window", Settings: "Settings", Quit: "Quit"},
	"es":    {Show: "Abrir ventana principal", Settings: "Ajustes", Quit: "Salir"},
	"ru":    {Show: "Открыть главное окно", Settings: "Настройки", Quit: "Выйти"},
	"zh-CN": {Show: "打开主窗口", Settings: "设置", Quit: "退出"},
	"zh-TW": {Show: "開啟主視窗", Settings: "設定", Quit: "結束"},
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

// TrayActions holds the main-window/settings launchers so the tray service
// stays decoupled from the window services.
type TrayActions struct {
	ShowMain     func()
	OpenSettings func()
}

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
// renderer reports a new appearance language.
type TrayService struct {
	mu       sync.Mutex
	app      *application.App
	tray     *application.SystemTray
	actions  TrayActions
	language string
	quit     func()
}

func newTrayService(app *application.App, tray *application.SystemTray, actions TrayActions) *TrayService {
	return &TrayService{app: app, tray: tray, actions: actions, language: "en", quit: app.Quit}
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
	labels := trayLabelsFor(s.language)
	menu := s.app.NewMenu()
	menu.Add(labels.Show).OnClick(func(*application.Context) {
		if s.actions.ShowMain != nil {
			s.actions.ShowMain()
		}
	})
	menu.Add(labels.Settings).OnClick(func(*application.Context) {
		if s.actions.OpenSettings != nil {
			s.actions.OpenSettings()
		}
	})
	menu.Add(labels.Quit).OnClick(func(*application.Context) {
		s.Quit()
	})
	return menu
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
