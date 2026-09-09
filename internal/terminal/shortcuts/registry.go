// Package shortcuts owns the global shortcut registry (P4-03, SYS-02):
// conflict detection, platform-accelerator parsing, and registration state.
// The Wails adapter translates these into real OS-level registrations.
package shortcuts

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

var (
	ErrConflict       = errors.New("shortcut conflict with existing registration")
	ErrNotRegistered  = errors.New("shortcut not registered")
	ErrAcceleratorBad = errors.New("invalid accelerator string")
)

// Accelerator is a parsed keyboard shortcut definition.
type Accelerator struct {
	Raw       string
	Modifiers []string // "cmd","ctrl","alt","shift","super"
	Key       string   // "F12", "Space", "A"...
}

// ParseAccelerator parses "CmdOrCtrl+Shift+F12" style strings.
func ParseAccelerator(raw string) (*Accelerator, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrAcceleratorBad
	}
	parts := strings.Split(raw, "+")
	if len(parts) < 1 {
		return nil, ErrAcceleratorBad
	}
	accel := &Accelerator{Raw: raw}
	validModifiers := map[string]bool{
		"cmd": true, "ctrl": true, "control": true, "alt": true,
		"shift": true, "super": true, "cmdorctrl": true, "cmdoralt": true,
	}
	for i, part := range parts {
		lower := strings.ToLower(strings.TrimSpace(part))
		if i < len(parts)-1 {
			// must be a modifier
			if !validModifiers[lower] {
				return nil, fmt.Errorf("%w: %q is not a modifier", ErrAcceleratorBad, part)
			}
			accel.Modifiers = append(accel.Modifiers, lower)
		} else {
			// must be a key
			if strings.TrimSpace(lower) == "" {
				return nil, ErrAcceleratorBad
			}
			accel.Key = strings.ToUpper(lower)
		}
	}
	if accel.Key == "" {
		return nil, ErrAcceleratorBad
	}
	return accel, nil
}

// ShortcutEntry is one registered shortcut with its callback ID.
type ShortcutEntry struct {
	Accelerator *Accelerator
	CallbackID  string
}

// Registry owns global shortcut registrations. The platform adapter calls
// Register for each entry and receives the callback ID when the OS fires.
type Registry struct {
	mu         sync.Mutex
	registered map[string]*ShortcutEntry // accelerator.Raw -> entry
	conflicts  map[string]string         // raw accelerator -> callback ID of conflicting registration
}

func NewRegistry() *Registry {
	return &Registry{
		registered: make(map[string]*ShortcutEntry),
		conflicts:  make(map[string]string),
	}
}

// Register adds a shortcut. Returns ErrConflict if the same accelerator is
// already registered by another callback.
func (r *Registry) Register(raw, callbackID string) error {
	accel, err := ParseAccelerator(raw)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.registered[strings.ToLower(raw)]; ok {
		return fmt.Errorf("%w: %q held by %q", ErrConflict, raw, existing.CallbackID)
	}
	_ = accel
	r.registered[strings.ToLower(raw)] = &ShortcutEntry{
		Accelerator: accel,
		CallbackID:  callbackID,
	}
	return nil
}

// Unregister removes a shortcut.
func (r *Registry) Unregister(raw string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := strings.ToLower(strings.TrimSpace(raw))
	if _, ok := r.registered[key]; !ok {
		return ErrNotRegistered
	}
	delete(r.registered, key)
	return nil
}

// Lookup finds the callback ID for an accelerator string.
func (r *Registry) Lookup(raw string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.registered[strings.ToLower(strings.TrimSpace(raw))]
	if !ok {
		return "", ErrNotRegistered
	}
	return entry.CallbackID, nil
}

// List returns all registered accelerator raw strings.
func (r *Registry) List() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := make([]string, 0, len(r.registered))
	for key := range r.registered {
		keys = append(keys, key)
	}
	return keys
}

// TrayMenuState captures the tray menu items for a snapshot (P4-03).
type TrayMenuItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"` // "item" | "separator" | "submenu"
}

// TrayState is the serialisable tray menu definition.
type TrayState struct {
	Tooltip string         `json:"tooltip"`
	Items   []TrayMenuItem `json:"items"`
}

// BuildTrayState generates the default Netcatty tray menu structure.
func BuildTrayState(recentHosts []string) TrayState {
	items := []TrayMenuItem{
		{ID: "show", Label: "Show Netcatty", Kind: "item"},
		{ID: "sep-1", Label: "", Kind: "separator"},
	}
	for _, host := range recentHosts {
		items = append(items, TrayMenuItem{ID: "connect-" + host, Label: "Connect " + host, Kind: "item"})
	}
	if len(recentHosts) > 0 {
		items = append(items, TrayMenuItem{ID: "sep-2", Label: "", Kind: "separator"})
	}
	items = append(items,
		TrayMenuItem{ID: "toggle-port-forward", Label: "Toggle Port Forwarding", Kind: "item"},
		TrayMenuItem{ID: "sep-3", Label: "", Kind: "separator"},
		TrayMenuItem{ID: "quit", Label: "Quit", Kind: "item"},
	)
	return TrayState{Tooltip: "Netcatty", Items: items}
}
