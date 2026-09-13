package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

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

func (s *TerminalService) GetHelperSessionState(sessionID string) (HelperSessionState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	term := s.sessions[sessionID]
	if term == nil || term.closing || term.helper == nil {
		return HelperSessionState{}, fmt.Errorf("helper session not found")
	}
	return term.helperState, nil
}

// handleHelperLifecycle serializes supervised helper events into the session's
// queryable state and the renderer-visible `<kind>:lifecycle` event. A "failed"
// helper keeps the terminal session alive so the renderer can offer a manual
// restart (RestartHelper); mosh/et protocol state cannot resume, so the restart
// opens a new remote shell under the same session. Only a clean user exit
// ("exited") closes the session.
func (s *TerminalService) handleHelperLifecycle(sessionID, kind string, term *terminalSession) func(supervised.TerminalEvent) {
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
func (s *TerminalService) RestartHelper(sessionID string) (HelperSessionState, error) {
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

func sshBootstrapConfig(ctx context.Context, s *TerminalService, request MoshStartRequest) (ssh.DialConfig, error) {
	// Bootstrap always uses the strict known-host owner, even if a renderer
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

func (s *TerminalService) startSupervisedTerminal(request MoshStartRequest, kind string) (string, error) {
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
