package terminaluse

import (
	"github.com/binaricat/netcatty/internal/terminal/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// Seed helpers for white-box fixtures that live outside this package (the SFTP
// and clipboard seam tests in cmd/netcatty). They bypass the normal start paths
// by design; production wiring must never call them.

// SeedSessionForTest installs an empty live session.
func (s *Service) SeedSessionForTest(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = make(map[string]*terminalSession)
	}
	s.sessions[sessionID] = &terminalSession{}
}

// SeedTransportSessionForTest installs a live session backed by an established
// SSH client, as if Connect had completed.
func (s *Service) SeedTransportSessionForTest(sessionID string, client *gossh.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = make(map[string]*terminalSession)
	}
	s.sessions[sessionID] = &terminalSession{transport: &ssh.Transport{Client: client}}
}
