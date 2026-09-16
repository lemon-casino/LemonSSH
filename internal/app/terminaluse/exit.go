package terminaluse

import (
	"errors"
	"os/exec"

	"github.com/binaricat/netcatty/internal/terminal/pty"
	gossh "golang.org/x/crypto/ssh"
)

// TerminalExitStatus is the reliable side channel for binary Complete frames.
// Records are published before route teardown and bounded to the last 256 exits.
type TerminalExitStatus struct {
	SessionID   string `json:"sessionId"`
	Intentional bool   `json:"intentional,omitempty"`
	Reason      string `json:"reason"`
	ExitCode    *int   `json:"exitCode,omitempty"`
	Error       string `json:"error,omitempty"`
}

func terminalWaitExit(id string, err error) TerminalExitStatus {
	status := TerminalExitStatus{SessionID: id, Reason: "exited"}
	code := 0
	var sshExit *gossh.ExitError
	var processExit *exec.ExitError
	var ptyExit pty.ExitError
	switch {
	case err == nil:
	case errors.As(err, &sshExit):
		code = sshExit.ExitStatus()
	case errors.As(err, &processExit):
		code = processExit.ExitCode()
	case errors.As(err, &ptyExit):
		code = ptyExit.Code
	default:
		status.Reason = "error"
		status.Error = err.Error()
		return status
	}
	status.ExitCode = &code
	return status
}

// GetExitStatus is the reliable side channel for binary Complete frames. Records
// are published before route teardown and bounded to the last 256 exits.
func (s *Service) GetExitStatus(id string) *TerminalExitStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, ok := s.exitStatuses[id]
	if !ok {
		return nil
	}
	return &status
}
