package sftpuse

import (
	pkgsftp "github.com/pkg/sftp"

	"github.com/binaricat/netcatty/internal/terminal/sftp"
)

// Seed helpers for white-box fixtures that live outside this package (the
// download/temp seam tests in cmd/netcatty). They bypass the normal open paths
// by design; production wiring must never call them.

// SeedClientSessionForTest installs an established SFTP subsystem client, as
// if Open had completed.
func (s *Service) SeedClientSessionForTest(sessionID string, raw *pkgsftp.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = make(map[string]*client)
	}
	s.sessions[sessionID] = &client{raw: raw, fs: sftp.NewClientFS(raw), bounded: sftp.NewSession(4)}
}
