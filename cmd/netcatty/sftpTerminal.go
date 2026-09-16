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
	client, sameSession, err := s.terminal.TransportFor(sessionID)
	if err != nil {
		return "", err
	}
	raw, err := pkgsftp.NewClient(client)
	if err != nil {
		return "", fmt.Errorf("terminal sftp subsystem: %w", err)
	}
	if !sameSession() {
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
