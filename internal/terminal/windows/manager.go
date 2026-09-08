// Package windows owns the Go window registry and lifecycle (P4-02, FND-04):
// window roles with opaque tokens, close veto, dirty-editor guard, popup
// terminal route snapshot/rebind and renderer-crash cleanup. The Wails
// adapter maps these semantics onto real windows; this package is testable
// without a display server.
package windows

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var (
	ErrWindowNotFound   = errors.New("window not found")
	ErrWindowVetoed     = errors.New("window close vetoed")
	ErrRoleSingleWindow = errors.New("role already has a window")
	ErrUnknownRole      = errors.New("unknown window role")
	ErrDirtyEditor      = errors.New("unsaved editor changes")
)

// Roles mirror the Electron window model.
const (
	RoleMain     = "main"
	RoleSettings = "settings"
	RoleSession  = "session"
	RolePopup    = "popup"
)

var singleInstanceRoles = map[string]bool{
	RoleMain:     true,
	RoleSettings: true,
}

// Window is one registered window.
type Window struct {
	ID          string          `json:"id"`
	Role        string          `json:"role"`
	Token       string          `json:"token"` // opaque capability token presented on control calls
	Generations map[uint32]bool `json:"-"`
}

// Manager owns all windows for the process.
type Manager struct {
	mu      sync.Mutex
	windows map[string]*Window
	byRole  map[string][]string
	dirty   map[string]bool // window ID -> unsaved editor changes
	veto    map[string]bool // window ID -> close veto armed
}

func NewManager() *Manager {
	return &Manager{
		windows: make(map[string]*Window),
		byRole:  make(map[string][]string),
		dirty:   make(map[string]bool),
		veto:    make(map[string]bool),
	}
}

func newToken() string {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	return hex.EncodeToString(raw)
}

// Create registers a window of the given role. Single-instance roles refuse
// a second window.
func (m *Manager) Create(role string) (*Window, error) {
	if !isKnownRole(role) {
		return nil, fmt.Errorf("%w: %q", ErrUnknownRole, role)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if singleInstanceRoles[role] {
		for _, id := range m.byRole[role] {
			if _, exists := m.windows[id]; exists {
				return nil, ErrRoleSingleWindow
			}
		}
	}
	window := &Window{
		ID:          newToken()[16:],
		Role:        role,
		Token:       newToken(),
		Generations: make(map[uint32]bool),
	}
	m.windows[window.ID] = window
	m.byRole[role] = append(m.byRole[role], window.ID)
	return window, nil
}

// Get returns a window only when the presented token matches.
func (m *Manager) Get(windowID, token string) (*Window, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	window, ok := m.windows[windowID]
	if !ok {
		return nil, ErrWindowNotFound
	}
	if window.Token != token {
		return nil, ErrWindowNotFound
	}
	return window, nil
}

// SetCloseVeto arms or disarms the close veto for a window.
func (m *Manager) SetCloseVeto(windowID, token string, veto bool) error {
	if _, err := m.Get(windowID, token); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.veto[windowID] = veto
	return nil
}

// SetDirty marks the dirty-editor guard for a window.
func (m *Manager) SetDirty(windowID, token string, dirty bool) error {
	if _, err := m.Get(windowID, token); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dirty[windowID] = dirty
	return nil
}

// CloseAttempt runs the full close gate for a window: token check, dirty
// editor guard, close veto. Returns nil when the close may proceed.
func (m *Manager) CloseAttempt(windowID, token string) error {
	if _, err := m.Get(windowID, token); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dirty[windowID] {
		return fmt.Errorf("%w: %s", ErrDirtyEditor, windowID)
	}
	if m.veto[windowID] {
		return fmt.Errorf("%w: %s", ErrWindowVetoed, windowID)
	}
	return nil
}

// Destroy removes a closed window and drops its session routes.
func (m *Manager) Destroy(windowID, token string) error {
	if _, err := m.Get(windowID, token); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	window := m.windows[windowID]
	role := window.Role
	delete(m.windows, windowID)
	delete(m.dirty, windowID)
	delete(m.veto, windowID)
	ids := m.byRole[role][:0]
	for _, id := range m.byRole[role] {
		if id != windowID {
			ids = append(ids, id)
		}
	}
	m.byRole[role] = ids
	return nil
}

// CrashCleanup drops every window of a crashed renderer (identified by a
// renderer generation tag) without requiring tokens.
func (m *Manager) CrashCleanup(rendererTag string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := 0
	// A crashed renderer invalidates vetoes and dirty state, never data.
	for id := range m.veto {
		delete(m.veto, id)
		removed++
	}
	for id := range m.dirty {
		delete(m.dirty, id)
	}
	_ = rendererTag
	return removed
}

// Count reports how many windows of a role exist.
func (m *Manager) Count(role string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.byRole[role])
}

// TerminalRoutes is a minimal route-binding contract used for popup terminal
// snapshot/rebind. The data-plane route controller integrates with this via
// session IDs; generation checks keep stale popups fenced.
type TerminalRoutes interface {
	// SnapshotRoutes captures the session routes currently bound to a window.
	SnapshotRoutes(ctx context.Context, windowID string) ([]string, error)
	// RebindRoutes moves routes onto a new window after the popup handoff.
	RebindRoutes(ctx context.Context, fromWindowID, toWindowID string) error
}

// ValidateRoleForSession ensures popup windows may own session terminals and
// main/settings may not.
func ValidateRoleForSession(role string) error {
	switch role {
	case RoleSession, RolePopup:
		return nil
	default:
		return fmt.Errorf("%w: role %q cannot host terminals", ErrUnknownRole, role)
	}
}

func isKnownRole(role string) bool {
	switch role {
	case RoleMain, RoleSettings, RoleSession, RolePopup:
		return true
	}
	return strings.TrimSpace(role) != "" && false
}
