package main

import (
	"fmt"

	"github.com/binaricat/netcatty/internal/terminal/sftp"
	pkgsftp "github.com/pkg/sftp"
)

func (s *SFTPService) setTerminalService(terminal *TerminalService) { s.terminal = terminal }

// OpenForTerminal opens only a subsystem on the exact authenticated transport.
// It owns the SFTP channel, never the terminal's SSH connection or credentials.
func (s *SFTPService) OpenForTerminal(sessionID string) (string, error) {
	if s.terminal == nil {
		return "", fmt.Errorf("terminal service unavailable")
	}
	s.terminal.mu.Lock()
	terminal := s.terminal.sessions[sessionID]
	if terminal == nil || terminal.transport == nil || terminal.transport.Client == nil {
		s.terminal.mu.Unlock()
		return "", fmt.Errorf("active SSH terminal %q not found", sessionID)
	}
	transport := terminal.transport
	s.terminal.mu.Unlock()
	raw, err := pkgsftp.NewClient(transport.Client)
	if err != nil {
		return "", fmt.Errorf("terminal sftp subsystem: %w", err)
	}
	s.terminal.mu.Lock()
	defer s.terminal.mu.Unlock()
	if s.terminal.sessions[sessionID] != terminal {
		_ = raw.Close()
		return "", fmt.Errorf("terminal session closed while opening SFTP")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter++
	id := fmt.Sprintf("sftp-%d", s.counter)
	s.sessions[id] = &sftpClient{raw: raw, fs: sftp.NewClientFS(raw), bounded: sftp.NewSession(4)}
	return id, nil
}
