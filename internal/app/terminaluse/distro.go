package terminaluse

import (
	"context"
	"errors"
)

// Post-connect distro detection (step 2 of OS identification). Step 1 is the
// SSH banner vendor classification (GetSessionRemoteInfo); step 2 reuses the
// existing session transport to run the /etc/os-release probe — never a new
// SSH client. The renderer maps the output to a distro icon/category.

// distroProbe mirrors the renderer bridge contract comment: os-release first,
// uname as the fallback for hosts without an os-release file.
const distroProbe = `cat /etc/os-release 2>/dev/null || uname -a`

// SessionDistroResult mirrors the renderer bridge contract (getSessionDistroInfo).
type SessionDistroResult struct {
	Success bool   `json:"success"`
	Stdout  string `json:"stdout,omitempty"`
	Stderr  string `json:"stderr,omitempty"`
	Error   string `json:"error,omitempty"`
}

// GetSessionDistroInfo probes the remote (or local) OS identity over the
// session's existing exec channel.
func (s *Service) GetSessionDistroInfo(sessionID string) SessionDistroResult {
	stdout, stderr, err := s.sessionExecOutput(context.Background(), sessionID, distroProbe)
	if err != nil {
		if errors.Is(err, errSessionExecPending) {
			return SessionDistroResult{Error: err.Error()}
		}
		return SessionDistroResult{Error: err.Error()}
	}
	return SessionDistroResult{Success: true, Stdout: stdout, Stderr: stderr}
}
