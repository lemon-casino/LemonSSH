package main

import (
	"context"
	"sync"

	"github.com/binaricat/netcatty/internal/terminal/pty"
)

// PTYService is the Wails-facing facade over the local PTY session owner
// (P3-02). Sessions are addressed by opaque session IDs with generation
// fencing; output is delivered to the renderer through the terminal data
// plane (P3-01), not through JSON events.
type PTYService struct {
	mu       sync.Mutex
	sessions map[string]*pty.Session
}

func newPTYService() *PTYService {
	return &PTYService{sessions: make(map[string]*pty.Session)}
}

// Start launches a local PTY session and returns its generation.
func (s *PTYService) Start(sessionID, shell, cwd string, args, env []string, cols, rows uint16) (uint32, error) {
	session := pty.NewSession(pty.BuildConfig(sessionID, shell, cwd, args, env, cols, rows))
	if err := session.Start(context.Background(), pty.NewPlatformBackend()); err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.sessions[sessionID]; ok {
		// A reconnect superseded this session; fence and reap the old one.
		_, _ = existing.Reconnect()
		_ = existing.Close()
	}
	s.sessions[sessionID] = session
	return session.Generation(), nil
}

// Resize resizes the live PTY for the given generation.
func (s *PTYService) Resize(sessionID string, generation uint32, cols, rows uint16) error {
	session, err := s.get(sessionID)
	if err != nil {
		return err
	}
	return session.Resize(generation, cols, rows)
}

// Write sends renderer input into the PTY.
func (s *PTYService) Write(sessionID string, generation uint32, data []byte) (int, error) {
	session, err := s.get(sessionID)
	if err != nil {
		return 0, err
	}
	return session.Write(generation, data)
}

// Interrupt sends Ctrl+C into the PTY.
func (s *PTYService) Interrupt(sessionID string, generation uint32) error {
	session, err := s.get(sessionID)
	if err != nil {
		return err
	}
	return session.Interrupt(generation)
}

// Close terminates and reaps the session.
func (s *PTYService) Close(sessionID string) error {
	s.mu.Lock()
	session, ok := s.sessions[sessionID]
	delete(s.sessions, sessionID)
	s.mu.Unlock()
	if !ok {
		return pty.ErrSessionNotFound
	}
	return session.Close()
}

func (s *PTYService) get(sessionID string) (*pty.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, pty.ErrSessionNotFound
	}
	return session, nil
}
