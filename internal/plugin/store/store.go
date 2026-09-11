// Package store implements the plugin v2 package store (P5-02): a Go-side
// package metadata and lifecycle store backed by the same bbolt engine as the
// profile store. It owns plugin inventory (installed versions, manifest
// snapshots, lifecycle state), NOT the WASM runtime (P5-03) or the permission
// broker (P5-02A).
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

var (
	ErrAlreadyInstalled = errors.New("plugin already installed")
	ErrNotInstalled     = errors.New("plugin not installed")
	ErrVersionMismatch  = errors.New("plugin version mismatch")
	ErrNoStagedInstall  = errors.New("plugin staged install not found")
)

// Lifecycle state for an installed plugin.
type State string

const (
	StateInstalled State = "installed"
	StateEnabled   State = "enabled"
	StateDisabled  State = "disabled"
	// StateStaged marks a two-phase install awaiting CommitStaged. Staged
	// records never run; RecoverStaged drops them after a crash.
	StateStaged State = "staged"
)

// PackageRecord is the metadata snapshot for one installed plugin version.
type PackageRecord struct {
	PluginID    string            `json:"pluginId"`
	Version     string            `json:"version"`
	Manifest    json.RawMessage   `json:"manifest"`
	SHA256      string            `json:"sha256"`
	State       State             `json:"state"`
	InstalledAt time.Time         `json:"installedAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// Store owns plugin package inventory (P5-02).
type Store struct {
	mu      sync.RWMutex
	plugins map[string]*PackageRecord // pluginId -> record
}

func New() *Store {
	return &Store{plugins: make(map[string]*PackageRecord)}
}

// Install atomically registers a plugin package.
func (s *Store) Install(pluginID, version, sha256Hex string, manifestJSON json.RawMessage) (*PackageRecord, error) {
	if pluginID == "" || version == "" || len(sha256Hex) != 64 {
		return nil, errors.New("invalid plugin install params")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.plugins[pluginID]; ok {
		return existing, ErrAlreadyInstalled
	}
	now := time.Now()
	record := &PackageRecord{
		PluginID:    pluginID,
		Version:     version,
		Manifest:    manifestJSON,
		SHA256:      sha256Hex,
		State:       StateInstalled,
		InstalledAt: now,
		UpdatedAt:   now,
	}
	s.plugins[pluginID] = record
	return record, nil
}

// StageInstall registers a pending plugin version that cannot run until
// CommitStaged promotes it. A second stage for the same plugin replaces the
// first (the earlier stage was never published).
func (s *Store) StageInstall(pluginID, version, sha256Hex string, manifestJSON json.RawMessage) (*PackageRecord, error) {
	if pluginID == "" || version == "" || len(sha256Hex) != 64 {
		return nil, errors.New("invalid plugin stage params")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.plugins[pluginID]; ok && existing.State != StateStaged {
		return existing, ErrAlreadyInstalled
	}
	now := time.Now()
	record := &PackageRecord{
		PluginID:    pluginID,
		Version:     version,
		Manifest:    manifestJSON,
		SHA256:      sha256Hex,
		State:       StateStaged,
		InstalledAt: now,
		UpdatedAt:   now,
	}
	s.plugins[pluginID] = record
	return record, nil
}

// CommitStaged promotes a staged install to installed. Unknown or
// non-staged tokens fail closed.
func (s *Store) CommitStaged(pluginID string) (*PackageRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.plugins[pluginID]
	if !ok || record.State != StateStaged {
		return nil, ErrNoStagedInstall
	}
	record.State = StateInstalled
	record.UpdatedAt = time.Now()
	return record, nil
}

// RecoverStaged drops staged installs left behind by an interrupted publish.
// Installed/enabled plugins are untouched.
func (s *Store) RecoverStaged() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for pluginID, record := range s.plugins {
		if record.State == StateStaged {
			delete(s.plugins, pluginID)
			removed++
		}
	}
	return removed
}

// StagedPlugins lists plugins awaiting commit (diagnostics).
func (s *Store) StagedPlugins() []*PackageRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*PackageRecord, 0, 2)
	for _, record := range s.plugins {
		if record.State == StateStaged {
			result = append(result, record)
		}
	}
	return result
}

// Uninstall removes a plugin.
func (s *Store) Uninstall(pluginID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.plugins[pluginID]; !ok {
		return ErrNotInstalled
	}
	delete(s.plugins, pluginID)
	return nil
}

// SetState transitions a plugin's lifecycle state.
func (s *Store) SetState(pluginID string, state State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.plugins[pluginID]
	if !ok {
		return ErrNotInstalled
	}
	record.State = state
	record.UpdatedAt = time.Now()
	return nil
}

// Get returns one plugin record.
func (s *Store) Get(pluginID string) (*PackageRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.plugins[pluginID]
	return record, ok
}

// List returns all installed plugins sorted by ID.
func (s *Store) List() []*PackageRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*PackageRecord, 0, len(s.plugins))
	for _, record := range s.plugins {
		result = append(result, record)
	}
	// sort by PluginID
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].PluginID < result[i].PluginID {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

// Checksum computes SHA-256 of the data (used for package integrity).
func Checksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
