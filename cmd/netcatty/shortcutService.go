package main

import (
	"strings"
	"sync"

	"github.com/binaricat/netcatty/internal/terminal/shortcuts"
)

type ShortcutService struct {
	mu       sync.Mutex
	registry *shortcuts.Registry
	current  string
}

type HotkeyResult struct {
	Success     bool   `json:"success"`
	Enabled     bool   `json:"enabled,omitempty"`
	Error       string `json:"error,omitempty"`
	Accelerator string `json:"accelerator,omitempty"`
}

type HotkeyStatus struct {
	Enabled bool    `json:"enabled"`
	Hotkey  *string `json:"hotkey"`
}

func newShortcutService() *ShortcutService {
	return &ShortcutService{registry: shortcuts.NewRegistry()}
}

func wailsAccelerator(raw string) (string, error) {
	parsed, err := shortcuts.ParseAccelerator(raw)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(parsed.Modifiers)+1)
	for _, modifier := range parsed.Modifiers {
		switch modifier {
		case "cmdorctrl":
			parts = append(parts, "CmdOrCtrl")
		case "control":
			parts = append(parts, "Ctrl")
		case "cmd":
			parts = append(parts, "Cmd")
		case "ctrl":
			parts = append(parts, "Ctrl")
		case "alt":
			parts = append(parts, "Alt")
		case "shift":
			parts = append(parts, "Shift")
		case "super":
			parts = append(parts, "Super")
		default:
			if modifier == "" {
				continue
			}
			parts = append(parts, strings.ToUpper(modifier[:1])+modifier[1:])
		}
	}
	parts = append(parts, parsed.Key)
	return strings.Join(parts, "+"), nil
}

func (s *ShortcutService) Register(raw string) HotkeyResult {
	accel, err := wailsAccelerator(raw)
	if err != nil {
		return HotkeyResult{Success: false, Error: err.Error()}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != "" && s.current != accel {
		_ = s.registry.Unregister(s.current)
	}
	if err := s.registry.Register(accel, "toggle-window"); err != nil && !strings.Contains(err.Error(), "conflict") {
		return HotkeyResult{Success: false, Error: err.Error(), Accelerator: accel}
	}
	s.current = accel
	return HotkeyResult{
		Success:     false,
		Enabled:     false,
		Accelerator: accel,
		Error:       "native global hotkeys are not available on Wails v3.0.0-alpha.63",
	}
}

func (s *ShortcutService) Unregister() HotkeyResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != "" {
		_ = s.registry.Unregister(s.current)
	}
	s.current = ""
	return HotkeyResult{Success: true}
}

func (s *ShortcutService) Status() HotkeyStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == "" {
		return HotkeyStatus{Enabled: false, Hotkey: nil}
	}
	current := s.current
	return HotkeyStatus{Enabled: true, Hotkey: &current}
}

func (s *ShortcutService) List() []string {
	return s.registry.List()
}
