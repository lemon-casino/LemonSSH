package terminaluse

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/binaricat/lemonssh/internal/platform/monitoring"
)

// errSessionExecPending marks a supervised helper whose handshake transport is
// not ready yet — callers should retry shortly.
var errSessionExecPending = errors.New("terminal session is still connecting")

// Remote shell history (History side panel). One exec channel on the session's
// existing transport detects the login shell and tails only the matching
// history file(s). No second SSH connection is opened.

const defaultHistoryLimit = 1000

// RemoteHistoryResult mirrors the renderer bridge contract (readRemoteHistory).
type RemoteHistoryResult struct {
	Success bool   `json:"success"`
	Pending bool   `json:"pending,omitempty"`
	Error   string `json:"error,omitempty"`
	Shell   string `json:"shell,omitempty"`
	Bash    string `json:"bash,omitempty"`
	Zsh     string `json:"zsh,omitempty"`
	Fish    string `json:"fish,omitempty"`
}

// remoteHistoryProbe tails the history files guarded by markers so one
// round-trip carries shell detection plus file contents. Fish history is
// YAML-ish; parsing stays on the renderer (domain/remoteHistory).
const remoteHistoryProbe = `home=${HOME:-$(cd 2>/dev/null && pwd)}
shell=${SHELL:-}
if [ -z "$shell" ]; then
  shell=$(getent passwd "$(id -u 2>/dev/null)" 2>/dev/null | cut -d: -f7)
fi
shell_base=$(basename "${shell:-sh}" 2>/dev/null)
printf 'shell=%s\n' "$shell_base"
emit() {
  printf '__NC_BEGIN_%s__\n' "$1"
  tail -n "__NC_LIMIT__" "$2" 2>/dev/null
  printf '__NC_END_%s__\n' "$1"
}
case "$shell_base" in
  bash) emit bash "$home/.bash_history" ;;
  zsh) emit zsh "$home/.zsh_history" ;;
  fish) emit fish "$home/.config/fish/fish_history" ;;
  *)
    emit bash "$home/.bash_history"
    emit zsh "$home/.zsh_history"
    emit fish "$home/.config/fish/fish_history"
    ;;
esac`

// BuildRemoteHistoryProbe renders the probe command for one tail limit. The
// limit is validated here so it never interpolates shell-hostile input.
func BuildRemoteHistoryProbe(limit int) string {
	if limit <= 0 {
		limit = defaultHistoryLimit
	}
	if limit > 50000 {
		limit = 50000
	}
	return strings.Replace(remoteHistoryProbe, "__NC_LIMIT__", fmt.Sprintf("%d", limit), 1)
}

// ParseRemoteHistoryProbe splits probe output into the bridge fields.
func ParseRemoteHistoryProbe(output string) RemoteHistoryResult {
	result := RemoteHistoryResult{Success: true}
	sections := map[string]*string{
		"bash": &result.Bash,
		"zsh":  &result.Zsh,
		"fish": &result.Fish,
	}
	var open *string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if shell, ok := strings.CutPrefix(line, "shell="); ok {
			if result.Shell == "" {
				result.Shell = strings.TrimSpace(shell)
			}
			continue
		}
		if name, ok := strings.CutPrefix(line, "__NC_BEGIN_"); ok {
			name = strings.TrimSuffix(name, "__")
			open = sections[name]
			continue
		}
		if _, ok := strings.CutPrefix(line, "__NC_END_"); ok {
			open = nil
			continue
		}
		if open != nil {
			*open += line + "\n"
		}
	}
	return result
}

// sessionExecOutput runs one command on the session's existing transport (or
// locally for local PTY sessions) and returns stdout/stderr.
func (s *Service) sessionExecOutput(ctx context.Context, sessionID, command string) (string, string, error) {
	_, term, err := s.attachTerm(sessionID)
	if err != nil {
		return "", "", err
	}
	if term.local != nil {
		out, err := monitoring.ExecuteLocal(ctx, command)
		return out, "", err
	}
	if term.transport == nil || term.transport.Client == nil {
		// A supervised mosh/et helper reports pending until its handshake
		// connection replaces the transport; other transports are simply
		// unsupported for exec.
		if term.helper != nil || term.runner != nil {
			return "", "", errSessionExecPending
		}
		return "", "", fmt.Errorf("an SSH session is required for remote commands")
	}
	channel, err := term.transport.Client.NewSession()
	if err != nil {
		return "", "", err
	}
	defer channel.Close()
	var stdout, stderr strings.Builder
	channel.Stdout = &stdout
	channel.Stderr = &stderr
	runErr := make(chan error, 1)
	go func() { runErr <- channel.Run(command) }()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case err := <-runErr:
		return stdout.String(), stderr.String(), err
	case <-timer.C:
		_ = channel.Close()
		return "", "", fmt.Errorf("remote command timed out")
	case <-ctx.Done():
		_ = channel.Close()
		return "", "", ctx.Err()
	}
}

// ReadRemoteHistory reads the remote host's shell history via one exec channel
// on the existing session transport.
func (s *Service) ReadRemoteHistory(sessionID string, limit int) RemoteHistoryResult {
	stdout, _, err := s.sessionExecOutput(context.Background(), sessionID, BuildRemoteHistoryProbe(limit))
	if err != nil {
		if errors.Is(err, errSessionExecPending) {
			return RemoteHistoryResult{Pending: true, Error: err.Error()}
		}
		return RemoteHistoryResult{Error: err.Error()}
	}
	result := ParseRemoteHistoryProbe(stdout)
	if result.Shell == "" {
		return RemoteHistoryResult{Error: "could not detect the remote login shell"}
	}
	return result
}
