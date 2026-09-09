// Package lifecycle wires the plugin store (P5-02) and permission broker
// (P5-02A) into a single lifecycle manager (P5-06): enabling a plugin
// activates its grants, disabling or uninstalling revokes them.
package lifecycle

import (
	"sync"

	"github.com/binaricat/netcatty/internal/plugin/permissions"
	pluginstore "github.com/binaricat/netcatty/internal/plugin/store"
)

// GrantSpec declares one permission grant for a plugin.
type GrantSpec struct {
	Resource string
	Lifetime permissions.Lifetime
}

// Manager connects the plugin store to the permission broker.
type Manager struct {
	mu     sync.Mutex
	store  *pluginstore.Store
	broker *permissions.Broker
}

func NewManager(store *pluginstore.Store, broker *permissions.Broker) *Manager {
	return &Manager{store: store, broker: broker}
}

// Enable activates a plugin and provisions its default grants.
func (m *Manager) Enable(pluginID string, grants []GrantSpec) error {
	if err := m.store.SetState(pluginID, pluginstore.StateEnabled); err != nil {
		return err
	}
	for _, grant := range grants {
		if _, err := m.broker.Grant(pluginID, grant.Resource, grant.Lifetime, 0); err != nil {
			// Roll back on partial failure.
			m.broker.RevokeAll(pluginID)
			_ = m.store.SetState(pluginID, pluginstore.StateDisabled)
			return err
		}
	}
	return nil
}

// Disable deactivates a plugin and revokes all its grants.
func (m *Manager) Disable(pluginID string) error {
	if err := m.store.SetState(pluginID, pluginstore.StateDisabled); err != nil {
		return err
	}
	m.broker.RevokeAll(pluginID)
	return nil
}

// Uninstall removes a plugin, its grants and its store record.
func (m *Manager) Uninstall(pluginID string) error {
	if err := m.store.SetState(pluginID, pluginstore.StateDisabled); err != nil {
		return err
	}
	m.broker.RevokeAll(pluginID)
	return m.store.Uninstall(pluginID)
}

// IsEnabled reports whether a plugin is currently enabled.
func (m *Manager) IsEnabled(pluginID string) bool {
	record, ok := m.store.Get(pluginID)
	return ok && record.State == pluginstore.StateEnabled
}
