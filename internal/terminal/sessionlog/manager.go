// Package sessionlog streams terminal session bytes into per-session log
// files. Streams survive across script runs and are closed explicitly or
// when the terminal service shuts down.
package sessionlog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Manager owns one open log file per session.
type Manager struct {
	mu         sync.Mutex
	defaultDir string
	streams    map[string]*os.File
}

func NewManager(defaultDir string) *Manager {
	return &Manager{
		defaultDir: defaultDir,
		streams:    make(map[string]*os.File),
	}
}

// Start opens a log file for the session. An empty filePath picks
// <defaultDir>/netcatty-script-<timestamp>.log. Returns the resolved path.
func (m *Manager) Start(sessionID, filePath string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("sessionId required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.streams[sessionID]; ok {
		return "", fmt.Errorf("session log already started")
	}
	resolved := filePath
	if resolved == "" {
		resolved = filepath.Join(m.defaultDir, fmt.Sprintf("netcatty-script-%d.log", time.Now().UnixMilli()))
	}
	if dir := filepath.Dir(resolved); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("create log directory: %w", err)
		}
	}
	file, err := os.OpenFile(resolved, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return "", fmt.Errorf("open session log: %w", err)
	}
	m.streams[sessionID] = file
	return resolved, nil
}

func (m *Manager) Append(sessionID string, data []byte) {
	if len(data) == 0 {
		return
	}
	m.mu.Lock()
	file := m.streams[sessionID]
	m.mu.Unlock()
	if file == nil {
		return
	}
	// Best effort: session logs never block or fail the terminal.
	_, _ = file.Write(data)
}

func (m *Manager) Stop(sessionID string) error {
	m.mu.Lock()
	file, ok := m.streams[sessionID]
	delete(m.streams, sessionID)
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("no session log started")
	}
	return file.Close()
}

// Status reports whether one session is currently being logged and the active file path.
func (m *Manager) Status(sessionID string) (bool, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	file := m.streams[sessionID]
	if file == nil {
		return false, ""
	}
	return true, file.Name()
}

func (m *Manager) CloseAll() {
	m.mu.Lock()
	files := m.streams
	m.streams = make(map[string]*os.File)
	m.mu.Unlock()
	for _, file := range files {
		_ = file.Close()
	}
}
