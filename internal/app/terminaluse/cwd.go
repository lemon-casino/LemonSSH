package terminaluse

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// cwdOSC is stream state, owned by one terminal and protected by Service.mu.
// Keep bytes until BEL or ST so split UTF-8 and split escape sequences are intact.
type cwdOSC struct {
	state   byte
	payload []byte
	cwd     string
}

func (p *cwdOSC) feed(data []byte) {
	for _, b := range data {
		switch p.state {
		case 0:
			if b == 27 {
				p.state = 1
			}
		case 1:
			if b == ']' {
				p.state = 2
				p.payload = nil
			} else if b != 27 {
				p.state = 0
			}
		case 2:
			if b == 7 {
				p.finish()
				p.state = 0
			} else if b == 27 {
				p.state = 3
			} else if len(p.payload) < 65536 {
				p.payload = append(p.payload, b)
			} else {
				p.payload = nil
				p.state = 0
			}
		case 3:
			if b == '\\' {
				p.finish()
				p.state = 0
			} else {
				p.payload = nil
				p.state = 0
			}
		}
	}
}
func (p *cwdOSC) finish() {
	raw := string(p.payload)
	if !strings.HasPrefix(raw, "7;") {
		return
	}
	path := strings.TrimPrefix(raw, "7;")
	if strings.HasPrefix(path, "file://") {
		parsed, err := url.Parse(path)
		if err != nil {
			return
		}
		path = parsed.Path
	}
	if strings.HasPrefix(path, "/") && !strings.ContainsAny(path, "\x00\r\n\x1b") {
		p.cwd = path
	}
}

// TerminalPwdResult reports the tracked foreground working directory.
type TerminalPwdResult struct {
	Success bool   `json:"success"`
	Cwd     string `json:"cwd,omitempty"`
	Error   string `json:"error,omitempty"`
}

// TerminalRemoteInfo reports the remote SSH banner of a session's transport.
type TerminalRemoteInfo struct {
	Success          bool   `json:"success"`
	RemoteSSHVersion string `json:"remoteSshVersion,omitempty"`
}

func (s *Service) GetSessionRemoteInfo(sessionID string) TerminalRemoteInfo {
	term, ok := s.lookup(sessionID)
	if !ok || term.transport == nil {
		return TerminalRemoteInfo{}
	}
	return TerminalRemoteInfo{Success: true, RemoteSSHVersion: string(term.transport.Client.ServerVersion())}
}

// Each SSH terminal owns a dedicated connection. Discover only an unambiguous
// interactive shell directly under this connection's sshd, then read the deepest
// foreground shell on that exact tty. Never use an exec channel's own pwd, a
// guessed home directory, or an arbitrary user's newest shell.
const terminalCwdProbe = `SELF=$$
rows=$(ps -e -o pid=,ppid=,tty=,stat=,comm= 2>/dev/null) || exit 1
selected=$(printf '%s\n' "$rows" | awk -v self="$SELF" '
function shell(c) { sub(/^.*\//,"",c); sub(/^-/,"",c); return c ~ /^(ba|z|fi|k|da|a|c|tc)?sh$/ }
{ pp[$1]=$2; tty[$1]=$3; st[$1]=$4; cm[$1]=$5 }
END {
 parent=pp[self]
 if (parent == "") exit 1
 for (p in pp) if (p != self && pp[p] == parent && tty[p] !~ /^\?+$/ && shell(cm[p])) { root=p; count++ }
 if (count != 1) exit 1
 best=-1
 for (p in pp) {
  if (tty[p] != tty[root] || !shell(cm[p]) || index(st[p], "+") == 0) continue
  q=p; d=0
  while (q != root && q in pp && d < 64) { q=pp[q]; d++ }
  if (q == root && d > best) { best=d; active=p }
 }
 if (active == "") exit 1
 print active
}') || exit 1
case "$selected" in ''|*[!0-9]*) exit 1;; esac
readlink "/proc/$selected/cwd" 2>/dev/null`

// TerminalPwdOptions bounds the best-effort foreground directory probe.
type TerminalPwdOptions struct {
	AllowHomeFallback       bool `json:"allowHomeFallback"`
	AllowLoginShellFallback bool `json:"allowLoginShellFallback"`
	TimeoutMs               int  `json:"timeoutMs"`
}

func (s *Service) GetSessionPwd(sessionID string, options TerminalPwdOptions) TerminalPwdResult {
	s.mu.Lock()
	term := s.sessions[sessionID]
	if term == nil {
		s.mu.Unlock()
		return TerminalPwdResult{Error: "Terminal session is not connected"}
	}
	cwd := term.cwd.cwd
	if cwd == "" && term.cwdProbing {
		s.mu.Unlock()
		return TerminalPwdResult{Error: "Terminal directory lookup already in progress"}
	}
	if cwd == "" && term.transport != nil {
		term.cwdProbing = true
	}
	s.mu.Unlock()
	if cwd != "" {
		return TerminalPwdResult{Success: true, Cwd: cwd}
	}
	if term.transport == nil {
		return TerminalPwdResult{Error: "Terminal has not reported its directory"}
	}
	// Bound the best-effort channel without closing the user's SSH transport.
	type result struct {
		path string
		err  error
	}
	done := make(chan result, 1)
	expired := make(chan struct{})
	defer close(expired)
	go func() {
		defer func() { s.mu.Lock(); term.cwdProbing = false; s.mu.Unlock() }()
		channel, err := term.transport.Client.NewSession()
		if err != nil {
			done <- result{err: err}
			return
		}
		defer channel.Close()
		select {
		case <-expired:
			return
		default:
		}
		go func() { <-expired; _ = channel.Close() }()
		out, err := channel.Output("exec sh -c '" + strings.ReplaceAll(terminalCwdProbe, "'", "'\"'\"'") + "'")
		done <- result{path: strings.TrimSuffix(string(out), "\n"), err: err}
	}()
	timeout := options.TimeoutMs
	if timeout <= 0 {
		timeout = 2000
	}
	if timeout < 100 {
		timeout = 100
	}
	if timeout > 5000 {
		timeout = 5000
	}
	timer := time.NewTimer(time.Duration(timeout) * time.Millisecond)
	defer timer.Stop()
	select {
	case result := <-done:
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.sessions[sessionID] != term {
			return TerminalPwdResult{Error: "Terminal session changed during directory lookup"}
		}
		// An OSC report received while the probe ran is newer and authoritative.
		if term.cwd.cwd != "" {
			return TerminalPwdResult{Success: true, Cwd: term.cwd.cwd}
		}
		if result.err == nil && strings.HasPrefix(result.path, "/") && !strings.ContainsAny(result.path, "\x00\r\n\x1b") {
			return TerminalPwdResult{Success: true, Cwd: result.path}
		}
		return TerminalPwdResult{Error: fmt.Sprintf("Interactive terminal directory unavailable: %v", result.err)}
	case <-timer.C:
		return TerminalPwdResult{Error: "Terminal directory lookup timed out"}
	}
}
