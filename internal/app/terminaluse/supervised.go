package terminaluse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/binaricat/netcatty/internal/terminal/mosh"
	"github.com/binaricat/netcatty/internal/terminal/pty"
	"github.com/binaricat/netcatty/internal/terminal/ssh"
	"github.com/binaricat/netcatty/internal/terminal/supervised"
)

// HelperSessionState is queryable after Start/route rebind so early lifecycle
// events cannot be lost. Running/ready means the native client was launched;
// encrypted native protocol readiness is not exposed by the upstream CLI.
type HelperSessionState struct {
	supervised.TerminalEvent
	SessionID     string `json:"sessionId"`
	BootEpoch     uint64 `json:"bootEpoch"`
	Kind          string `json:"kind"`
	RecoveryMode  string `json:"recoveryMode"`
	Readiness     string `json:"readiness"`
	RecoveryLimit string `json:"recoveryLimit"`
}

// MoshStartRequest is the shell-facing Mosh/ET bootstrap payload. The union of
// SSH dial fields lets the handshake reuse the same auth path as Connect.
type MoshStartRequest struct {
	SSHConnectRequest
	SessionID  string `json:"sessionId"`
	BootEpoch  uint64 `json:"bootEpoch"`
	EtPort     uint16 `json:"etPort"`
	ServerFifo string `json:"serverFifo"`
	// ClientPath is the absolute path to the local mosh-client / et binary.
	ClientPath string `json:"clientPath"`
	// ServerPath overrides the remote mosh-server command (empty uses default).
	ServerPath string `json:"serverPath"`
	Cols       uint16 `json:"cols"`
	Rows       uint16 `json:"rows"`
}

func (s *Service) GetHelperSessionState(sessionID string) (HelperSessionState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	term := s.sessions[sessionID]
	if term == nil || term.closing || term.helper == nil {
		return HelperSessionState{}, fmt.Errorf("helper session not found")
	}
	return term.helperState, nil
}

// handleHelperLifecycle serializes supervised helper events into the session's
// queryable state and the shell-visible `<kind>:lifecycle` event. A "failed"
// helper keeps the terminal session alive so the renderer can offer a manual
// restart (RestartHelper); mosh/et protocol state cannot resume, so the restart
// opens a new remote shell under the same session. Only a clean user exit
// ("exited") closes the session.
func (s *Service) handleHelperLifecycle(sessionID, kind string, term *terminalSession) func(supervised.TerminalEvent) {
	return func(event supervised.TerminalEvent) {
		s.mu.Lock()
		if s.sessions[sessionID] != term || term.closing {
			s.mu.Unlock()
			return
		}
		term.helperState.TerminalEvent = event
		state := term.helperState
		s.mu.Unlock()
		s.emit(kind+":lifecycle", state)
		if event.State == "running" {
			s.emit(kind+":session-ready", state)
		} else if event.State == "exited" {
			status := TerminalExitStatus{SessionID: sessionID, Reason: event.State, ExitCode: event.ExitCode, Error: event.Error}
			go s.closeWithStatus(sessionID, status, term)
		}
	}
}

// RestartHelper relaunches the supervised helper inside the same terminal
// session after a "failed" lifecycle event. The follow-up "running" event
// arrives through the existing lifecycle callback. Rejected for closing or
// missing sessions and for helpers whose run loop is still alive.
func (s *Service) RestartHelper(sessionID string) (HelperSessionState, error) {
	s.mu.Lock()
	term := s.sessions[sessionID]
	if term == nil || term.closing || term.helper == nil {
		s.mu.Unlock()
		return HelperSessionState{}, fmt.Errorf("helper session not found")
	}
	helper := term.helper
	s.mu.Unlock()
	if err := helper.Restart(); err != nil {
		return HelperSessionState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if term.closing || s.sessions[sessionID] != term {
		return HelperSessionState{}, fmt.Errorf("helper session not found")
	}
	return term.helperState, nil
}

func sshBootstrapConfig(ctx context.Context, s *Service, request MoshStartRequest) (ssh.DialConfig, error) {
	// Bootstrap always uses the strict known-host owner, even if a client
	// sends a weaker verifyHostKeys setting intended for another transport.
	request.VerifyHostKeys = new(bool)
	*request.VerifyHostKeys = true
	config, err := terminalSSHDialConfig(request.SSHConnectRequest, s.knownHosts, nil)
	var bindBootstrapPolicy func(*ssh.DialConfig, SSHConnectRequest)
	bindBootstrapPolicy = func(hop *ssh.DialConfig, input SSHConnectRequest) {
		hop.HostKeyPolicy = ssh.StrictPolicy(s.knownHosts)
		if input.EnableMFA {
			hop.Auth.Challenge = s.interactive.HandlerContext(ctx, input.Hostname)
		}
		for i := range hop.JumpHosts {
			bindBootstrapPolicy(&hop.JumpHosts[i], input.JumpHosts[i])
		}
	}
	bindBootstrapPolicy(&config, request.SSHConnectRequest)
	return config, err
}

// StartMosh runs the mosh bootstrap over SSH and supervises the local
// mosh-client. The remote handshake scrapes the MOSH CONNECT line, then the
// client process streams through the same data plane as other sessions.
func (s *Service) StartMosh(request MoshStartRequest) (string, error) {
	return s.startSupervisedTerminal(request, "mosh")
}

// StartEt runs the Eternal Terminal bootstrap. ET uses the same supervised
// process model; the remote command differs but the local supervision and
// data-plane wiring are identical to Mosh.
func (s *Service) StartEt(request MoshStartRequest) (string, error) {
	return s.startSupervisedTerminal(request, "et")
}

// startSupervisedTerminal resolves and verifies the pinned native client, then
// supervises it through the shared data plane with restart/recovery policy.
func (s *Service) startSupervisedTerminal(request MoshStartRequest, kind string) (string, error) {
	if request.Hostname == "" || request.Username == "" {
		return "", fmt.Errorf("host and username are required")
	}
	exePath, _ := os.Executable()
	repoRoot, _ := os.Getwd()
	resolved, err := resolveHelperBinary(kind, request.ClientPath, os.Getenv(helperRootEnv), filepath.Dir(exePath), repoRoot)
	if err != nil {
		return "", err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	request.ClientPath = resolved
	pin, err := os.ReadFile(resolved + ".manifest.json")
	if err != nil {
		return "", fmt.Errorf("%s: pinned helper manifest: %w", kind, err)
	}
	var manifest supervised.Manifest
	if err = json.Unmarshal(pin, &manifest); err != nil {
		return "", err
	}
	manifest.Path = filepath.Base(resolved)
	if err = supervised.Verify(manifest, filepath.Dir(resolved)); err != nil {
		return "", err
	}
	s.mu.Lock()
	sessionID := request.SessionID
	if sessionID == "" {
		s.counter++
		sessionID = fmt.Sprintf("%s-%d", kind, s.counter)
	}
	if s.sessions[sessionID] != nil {
		s.mu.Unlock()
		return "", fmt.Errorf("session %q already exists", sessionID)
	}
	bootstrap, err := s.controller.Open(sessionID)
	if err != nil {
		s.mu.Unlock()
		return "", err
	}
	mode := "new-shell"
	readiness := "client-started"
	if kind == "et" && etUsesGoSSH(request) {
		readiness = "bootstrap-complete"
	}
	limit := "Native mosh-client loses sequence and AES-OCB nonce state on death; reusing MOSH_KEY is unsafe. Recovery starts a new remote shell."
	if kind == "et" {
		limit = "Native et loses replay and encryption state on death and has no resume CLI. Recovery starts a new remote shell."
	}
	term := &terminalSession{bootstrap: bootstrap, helperState: HelperSessionState{SessionID: sessionID, BootEpoch: request.BootEpoch, Kind: kind, RecoveryMode: mode, RecoveryLimit: limit, Readiness: readiness, TerminalEvent: supervised.TerminalEvent{State: "starting"}}}
	factory := func(ctx context.Context, cols, rows uint16) (*pty.Session, func(), error) {
		if err := supervised.Verify(manifest, filepath.Dir(resolved)); err != nil {
			return nil, nil, err
		}
		release := func() {}
		var args []string
		var cwd string
		var handoff <-chan struct{}
		var extraEnv map[string]string
		var err error
		if kind == "et" && etUsesGoSSH(request) {
			bridge, bridgeErr := s.prepareEt(ctx, request)
			if bridgeErr != nil {
				return nil, nil, bridgeErr
			}
			release = bridge.Close
			args, extraEnv = bridge.args, bridge.env
			cwd = bridge.directory
			handoff = bridge.ready
		} else {
			var connect mosh.Connect
			// Stock mosh and pinned MoshCatty restart their nonce counter at zero
			// (crypto.cc unique / transport.rs NEXT_PACKET_SEQUENCE).
			// Reusing a key after process death breaks AES-OCB nonce uniqueness.
			if kind == "mosh" {
				connect, err = s.runHandshake(ctx, request)
				if err != nil {
					return nil, nil, err
				}
			}
			args, extraEnv, err = mosh.ClientLaunch(kind, request.Hostname, request.Username, request.Port, connect)
			if err != nil {
				return nil, nil, err
			}
			if kind == "et" {
				if request.EtPort != 0 {
					args = append(args, "--port", strconv.Itoa(int(request.EtPort)))
				}
				if request.ServerPath != "" {
					args = append(args, "--terminal-path", request.ServerPath)
				}
				if request.ServerFifo != "" {
					args = append(args, "--serverfifo", request.ServerFifo)
				}
				args = append(args, "--silent", "--telemetry=false")
			}
		}
		env := helperEnvironment(extraEnv)
		local := pty.NewSession(pty.Config{SessionID: sessionID, Shell: resolved, Args: args, Env: env, CWD: cwd, Cols: cols, Rows: rows})
		if err = local.Start(ctx, pty.NewPlatformBackend()); err != nil {
			return nil, release, err
		}
		if handoff != nil {
			// Native SSH temporarily inherits the ET PTY. Expose input only once
			// that bootstrap channel closes, so startup commands reach et itself.
			exit := make(chan error, 1)
			go func() { exit <- local.Wait() }()
			timer := time.NewTimer(30 * time.Second)
			defer timer.Stop()
			select {
			case <-handoff:
			case <-ctx.Done():
				err = ctx.Err()
			case <-timer.C:
				err = fmt.Errorf("et: native SSH bootstrap handoff timed out")
			case <-exit:
				err = fmt.Errorf("et: client exited before native SSH bootstrap handoff")
			}
			if err != nil {
				_ = local.Close()
				return nil, release, err
			}
		}
		return local, release, nil
	}
	term.helperFactory = factory
	term.helper = supervised.NewTerminal(context.Background(), request.Cols, request.Rows, supervised.RestartPolicy{MaxRestarts: 3, Backoff: 250 * time.Millisecond}, factory,
		func(data []byte) bool { return s.publishOutput(sessionID, data) },
		s.handleHelperLifecycle(sessionID, kind, term))
	s.sessions[sessionID] = term
	s.mu.Unlock()
	if err = term.helper.Start(); err != nil {
		_ = s.closeWithStatus(sessionID, TerminalExitStatus{SessionID: sessionID, Reason: "error", Error: err.Error()}, term)
		return "", err
	}
	return sessionID, nil
}

// runHandshake dials SSH and runs the remote mosh-server, returning the parsed
// MOSH CONNECT announcement.
func (s *Service) runHandshake(ctx context.Context, request MoshStartRequest) (mosh.Connect, error) {
	if request.Port == 0 {
		request.Port = 22
	}
	config, err := sshBootstrapConfig(ctx, s, request)
	if err != nil {
		return mosh.Connect{}, err
	}
	transport, err := ssh.Dial(ctx, config)
	if err != nil {
		return mosh.Connect{}, fmt.Errorf("ssh dial %s:%d: %w", request.Hostname, request.Port, err)
	}
	defer transport.Close()
	stop := context.AfterFunc(ctx, func() { _ = transport.Close() })
	defer stop()
	// mosh-client accepts numeric UDP destinations only. SSH jumps do not
	// tunnel UDP; the target must remain directly reachable from this machine.
	ip := net.ParseIP(request.Hostname)
	if ip == nil {
		addresses, resolveErr := net.DefaultResolver.LookupIPAddr(ctx, request.Hostname)
		if resolveErr != nil || len(addresses) == 0 {
			return mosh.Connect{}, fmt.Errorf("mosh: cannot resolve direct UDP destination")
		}
		ip = addresses[0].IP
	}
	sshSession, err := transport.Client.NewSession()
	if err != nil {
		return mosh.Connect{}, fmt.Errorf("new session: %w", err)
	}
	defer sshSession.Close()
	if err := sshSession.RequestPty("xterm-256color", 24, 80, gossh.TerminalModes{}); err != nil {
		return mosh.Connect{}, fmt.Errorf("pty request: %w", err)
	}
	stdout, err := sshSession.StdoutPipe()
	if err != nil {
		return mosh.Connect{}, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := sshSession.Start(mosh.ServerCommand(request.ServerPath)); err != nil {
		return mosh.Connect{}, fmt.Errorf("start mosh-server: %w", err)
	}
	connect, err := scanConnectDeadline(stdout, 30*time.Second, func() { _ = sshSession.Close() })
	if err != nil {
		return mosh.Connect{}, err
	}
	if connect.IP == "" {
		connect.IP = ip.String()
	}
	return connect, nil
}

func scanConnectDeadline(source io.Reader, timeout time.Duration, closeChannel func()) (mosh.Connect, error) {
	timer := time.AfterFunc(timeout, closeChannel)
	defer timer.Stop()
	return scanForConnect(source, timeout)
}

// scanForConnect reads the handshake stream until the MOSH CONNECT line
// arrives, buffering a trailing partial marker across reads.
func scanForConnect(source io.Reader, timeout time.Duration) (mosh.Connect, error) {
	deadline := time.Now().Add(timeout)
	buffer := make([]byte, 4096)
	var pending []byte
	for time.Now().Before(deadline) {
		n, err := source.Read(buffer)
		if n > 0 {
			pending = append(pending, buffer[:n]...)
			if connect, _, ok, parseErr := mosh.ParseConnect(pending); parseErr != nil {
				return mosh.Connect{}, parseErr
			} else if ok {
				return connect, nil
			}
			// Keep only a tail that could still complete a marker; the rest is
			// user-visible noise the handshake does not surface.
			if len(pending) > 4096 && !mosh.TrailingMarkerPartial(string(pending)) {
				pending = pending[len(pending)-128:]
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return mosh.Connect{}, mosh.ErrNoConnectLine
			}
			return mosh.Connect{}, err
		}
	}
	return mosh.Connect{}, fmt.Errorf("mosh: handshake timed out: %w", mosh.ErrNoConnectLine)
}

// hashFile returns the SHA-256 hex digest of a file.
func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func helperEnvironment(extra map[string]string) []string {
	// exec.Cmd deduplicates env on Unix; ConPTY passes it through verbatim.
	// Keep one value per key so MOSH_KEY and bootstrap HOME cannot be shadowed.
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		replaced := false
		for override := range extra {
			if key == override || runtime.GOOS == "windows" && strings.EqualFold(key, override) {
				replaced = true
				break
			}
		}
		if !replaced {
			env = append(env, entry)
		}
	}
	for key, value := range extra {
		env = append(env, key+"="+value)
	}
	return env
}
