// Package terminaluse owns the shell-neutral terminal session use cases:
// SSH/telnet/serial/local-PTY/mosh/et session lifecycle, dial/auth/exec/write/
// resize/close, data-plane publishing, exit status tracking, and the
// keyboard-interactive challenge broker.
//
// Per the Wails v3 migration (W04), this package is the canonical owner of the
// terminal session pool and its rules. Shell facades (Wails today, capability
// dispatch later) adapt it to their transport and must not copy or re-implement
// its behavior. It must never import a shell (Wails, Electron, UI).
package terminaluse

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/lemon-casino/lemonssh/internal/platform/filesystem"
	"github.com/lemon-casino/lemonssh/internal/terminal/dataplane"
	"github.com/lemon-casino/lemonssh/internal/terminal/pty"
	"github.com/lemon-casino/lemonssh/internal/terminal/serialport"
	"github.com/lemon-casino/lemonssh/internal/terminal/ssh"
	"github.com/lemon-casino/lemonssh/internal/terminal/supervised"
	"github.com/lemon-casino/lemonssh/internal/terminal/telnet"
)

// Service owns terminal sessions end-to-end: SSH dial → auth → PTY channel →
// data plane streaming → renderer route bootstrap. Each session gets a route
// bootstrap (generation + one-use tokens) that the shell exchanges for the
// loopback data/urgent WebSocket connections.
type Service struct {
	mu                sync.Mutex
	sessions          map[string]*terminalSession
	exitStatuses      map[string]TerminalExitStatus
	exitOrder         []string
	pendingSnapshots  map[string]*pendingAttachReply[SnapshotResult]
	pendingApplies    map[string]*pendingAttachReply[bool]
	controller        *dataplane.RouteController
	dp                *dataplane.Server
	knownHosts        *ssh.KnownHosts
	interactive       *ssh.InteractiveBroker
	passphrase        *ssh.PassphraseBroker
	hostKeyConfirm    *ssh.HostKeyBroker
	emitChallenge     func(ssh.KeyboardChallenge)
	emitPassphrase    func(ssh.PassphraseRequest)
	emitHostKeyVerify func(ssh.HostKeyVerificationRequest)
	emitEvent         func(name string, payload any)
	observeOutput     func(sessionID string, data []byte)
	counter           int
	helperTemp        *filesystem.TempService
}

type terminalSession struct {
	closing            bool
	cwd                cwdOSC
	cwdProbing         bool
	completionQueries  int
	zmodemPrefix       []byte
	zmodem             *terminalZmodemStream
	transport          *ssh.Transport
	x11                *ssh.X11Forwarder
	session            *gossh.Session
	local              *pty.Session
	localConfig        pty.Config
	telnet             *telnet.Client
	serial             *serialport.Session
	serialID           string
	serialYmodemCancel context.CancelFunc
	runner             *supervised.Runner
	helper             *supervised.Terminal
	helperFactory      supervised.TerminalFactory
	helperState        HelperSessionState
	// pluginHooks marks a plugin-protocol connection session (see
	// plugin_session.go); Write falls through to the stdin adapter, while
	// Resize/Signal/close route through the hooks.
	pluginHooks *PluginSessionHooks
	stdin       io.WriteCloser
	bootstrap   dataplane.RouteBootstrap
	// uiID is the renderer session alias (SSHConnectRequest.SessionID) so
	// attach flows can address sessions the way the UI does.
	uiID string
	// encoding pins the terminal input charset ("utf-8" unless the renderer
	// switched the session, e.g. GB18030 devices); see encoding.go.
	encoding string
	// Attach (popup observe) state. flowPaused holds renderer-bound output in
	// flowBuffer instead of publishing it, so no live bytes fall into the gap
	// between the home renderer's snapshot and the popup route handoff.
	flowLeases          map[string]bool
	flowPaused          bool
	flowBuffer          []byte
	attachAuthorization string
	attachRebound       bool
	attachClosePrepared bool
}

const maxFlowPauseBufferBytes = 8 << 20

// New wires the route controller, transport and known-hosts store together.
// The urgent channel (Ctrl-C et al.) is handled in-process by writing the
// payload to the session's stdin.
func New(controller *dataplane.RouteController, dp *dataplane.Server, knownHosts *ssh.KnownHosts) *Service {
	service := &Service{
		sessions:   make(map[string]*terminalSession),
		controller: controller,
		dp:         dp,
		knownHosts: knownHosts,
	}
	service.interactive = ssh.NewInteractiveBroker(func(challenge ssh.KeyboardChallenge) {
		if service.emitChallenge != nil {
			service.emitChallenge(challenge)
		}
	}, 2*time.Minute)
	service.passphrase = ssh.NewPassphraseBroker(func(request ssh.PassphraseRequest) {
		if service.emitPassphrase != nil {
			service.emitPassphrase(request)
		}
	}, func(name string, payload any) {
		service.emit(name, payload)
	}, 2*time.Minute)
	service.hostKeyConfirm = ssh.NewHostKeyBroker(func(request ssh.HostKeyVerificationRequest) {
		if service.emitHostKeyVerify != nil {
			service.emitHostKeyVerify(request)
		}
	}, 2*time.Minute)
	dp.SetUrgentHandler(service.handleUrgent)
	return service
}

// SetChallengeEmitter wires the keyboard-interactive challenge callback.
func (s *Service) SetChallengeEmitter(emit func(ssh.KeyboardChallenge)) {
	s.emitChallenge = emit
}

// SetPassphraseEmitter wires the encrypted-key passphrase prompt callback.
func (s *Service) SetPassphraseEmitter(emit func(ssh.PassphraseRequest)) {
	s.emitPassphrase = emit
}

// SetHostKeyVerificationEmitter wires the changed host-key prompt callback.
func (s *Service) SetHostKeyVerificationEmitter(emit func(ssh.HostKeyVerificationRequest)) {
	s.emitHostKeyVerify = emit
}

// RespondPassphrase completes or cancels a pending passphrase prompt.
func (s *Service) RespondPassphrase(requestID, passphrase string, cancelled bool) error {
	return s.passphrase.Respond(requestID, passphrase, cancelled)
}

// RespondHostKeyVerification completes or rejects a pending changed host-key
// confirmation. accept+store rotates the pinned key; accept without store
// allows exactly this connection.
func (s *Service) RespondHostKeyVerification(requestID string, accept, store bool) error {
	return s.hostKeyConfirm.Respond(requestID, accept, store)
}

// DialInteractive bundles the renderer callbacks for one dial: the MFA
// challenge factory (per-hop hostnames, bound to ctx so cancelled dials cancel
// pending prompts), passphrase prompts bound to the request's session
// correlation, and the changed host-key confirmer.
func (s *Service) DialInteractive(ctx context.Context, request SSHConnectRequest) ssh.DialInteractive {
	interactive := ssh.DialInteractive{
		Passphrase:     s.passphrase.Requester(request.SessionID, request.BootEpoch),
		ConfirmHostKey: s.hostKeyConfirm.Confirmer(request.SessionID, request.BootEpoch),
	}
	if request.EnableMFA {
		interactive.Challenge = func(hostname string) func(string, string, []string, []bool) ([]string, error) {
			return s.interactive.HandlerContext(ctx, hostname)
		}
	}
	return interactive
}

// notifyPassphraseRejected surfaces exhausted passphrase attempts so the
// renderer can clear remembered passphrases for the key.
func (s *Service) notifyPassphraseRejected(err error) {
	var rejected *ssh.PassphraseRejectedError
	if !errors.As(err, &rejected) {
		return
	}
	s.emit(ssh.EventPassphraseRejected, map[string]any{
		"keyPaths": []string{rejected.KeyPath},
	})
}

// SetEventEmitter wires session-visible events (telnet echo mode, auto-login
// completion/cancellation, zmodem, exit status) to the shell's event bus.
func (s *Service) SetEventEmitter(emit func(name string, payload any)) {
	s.emitEvent = emit
}

// SetOutputObserver taps the raw output stream (script runner, session log).
func (s *Service) SetOutputObserver(observe func(sessionID string, data []byte)) {
	s.observeOutput = observe
}

// SetTempService wires the managed temp directory used by the ET bootstrap.
func (s *Service) SetTempService(temp *filesystem.TempService) { s.helperTemp = temp }

func (s *Service) publishOutput(sessionID string, data []byte) bool {
	s.mu.Lock()
	var stream *terminalZmodemStream
	if term := s.sessions[sessionID]; term != nil {
		stream = term.zmodem
	}
	s.mu.Unlock()
	if stream != nil {
		_, _ = stream.writer.Write(data)
		return true
	}
	if s.detectZmodem(sessionID, data) {
		return true
	}
	return s.publishTerminalBytes(sessionID, data)
}

func (s *Service) publishTerminalBytes(sessionID string, data []byte) bool {
	s.mu.Lock()
	term := s.sessions[sessionID]
	if term != nil {
		term.cwd.feed(data)
	}
	observe := s.observeOutput
	if term != nil && term.flowPaused {
		term.flowBuffer = append(term.flowBuffer, data...)
		overflow := len(term.flowBuffer) > maxFlowPauseBufferBytes
		s.mu.Unlock()
		// Script runner / session log observers keep seeing the bytes even
		// while the renderer display is paused.
		if observe != nil {
			observe(sessionID, data)
		}
		// Overflow guard: a vanished attach popup must never pin the session's
		// output forever — release the pause so the session keeps flowing.
		if overflow {
			s.forceReleaseFlowPause(sessionID)
		}
		return true
	}
	s.mu.Unlock()
	if observe != nil {
		observe(sessionID, data)
	}
	if err := s.dp.Publish(sessionID, data); err != nil {
		s.emit("terminal:error", map[string]any{"sessionId": sessionID, "error": err.Error()})
		// A supervised pump must return before its lifecycle can join it.
		_ = s.beginSessionClose(sessionID, TerminalExitStatus{SessionID: sessionID, Reason: "error", Error: err.Error()}, true)
		return false
	}
	return true
}

func (s *Service) emit(name string, payload any) {
	if s.emitEvent != nil {
		s.emitEvent(name, payload)
	}
}

// RespondKeyboardInteractive completes or cancels a pending MFA challenge.
func (s *Service) RespondKeyboardInteractive(requestID string, responses []string, cancelled bool) error {
	return s.interactive.Respond(requestID, responses, cancelled)
}

// Bootstrap returns the route credentials the renderer needs to attach its
// data/urgent WebSockets for a session.
func (s *Service) Bootstrap(sessionID string) (dataplane.RouteBootstrap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	term, ok := s.sessions[sessionID]
	if !ok || term.closing {
		return dataplane.RouteBootstrap{}, fmt.Errorf("session %q not found", sessionID)
	}
	return term.bootstrap, nil
}

// errRouteHandedOff is returned by Reconnect while an attach popup owns the
// display route: the previous owner must suspend its reconnect loop instead of
// stealing the route back.
var errRouteHandedOff = fmt.Errorf("terminal route handed off to another window")

// Reconnect rotates only the renderer route. Native Mosh/ET processes retain
// their protocol keys and roaming state; no new remote server is bootstrapped.
func (s *Service) Reconnect(sessionID string) (dataplane.RouteBootstrap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	term, ok := s.sessions[sessionID]
	if !ok || term.closing {
		for id, candidate := range s.sessions {
			if candidate.uiID != "" && candidate.uiID == sessionID && !candidate.closing {
				term, ok = candidate, true
				sessionID = id
				break
			}
		}
	}
	if !ok || term.closing {
		return dataplane.RouteBootstrap{}, fmt.Errorf("session not found")
	}
	if term.attachRebound {
		return dataplane.RouteBootstrap{}, errRouteHandedOff
	}
	bootstrap, err := s.controller.Open(sessionID)
	if err != nil {
		return dataplane.RouteBootstrap{}, err
	}
	term.bootstrap = bootstrap
	return bootstrap, nil
}

// ListenAddr exposes the bound loopback data plane address (host:port).
func (s *Service) ListenAddr() string { return s.dp.Addr() }

// Write sends raw stdin bytes to the remote shell. The payload arrives as
// UTF-8 from the renderer and is transcoded when the session uses a legacy
// input charset (SetSessionEncoding).
func (s *Service) Write(sessionID string, data []byte) (int, error) {
	term, ok := s.lookup(sessionID)
	if !ok {
		return 0, fmt.Errorf("session %q not found", sessionID)
	}
	if term.encoding != "" {
		data = encodeInput(data, term.encoding)
	}
	if term.helper != nil {
		return term.helper.Write(data)
	}
	if term.local != nil {
		return term.local.Write(term.local.Generation(), data)
	}
	if term.telnet != nil {
		if err := term.telnet.Send(data); err != nil {
			return 0, err
		}
		return len(data), nil
	}
	if term.serial != nil {
		return term.serial.Write(term.serialID, data)
	}
	if term.runner != nil {
		writer := term.runner.Writer()
		if writer == nil {
			return 0, fmt.Errorf("session %q input is closed", sessionID)
		}
		return writer.Write(data)
	}
	return term.stdin.Write(data)
}

// Resize updates the remote PTY window size.
func (s *Service) Resize(sessionID string, cols, rows uint16) error {
	term, ok := s.lookup(sessionID)
	if !ok {
		return fmt.Errorf("session %q not found", sessionID)
	}
	if term.pluginHooks != nil {
		if term.pluginHooks.Resize == nil {
			return nil
		}
		return term.pluginHooks.Resize(cols, rows)
	}
	if term.helper != nil {
		return term.helper.Resize(cols, rows)
	}
	if term.local != nil {
		return term.local.Resize(term.local.Generation(), cols, rows)
	}
	if term.telnet != nil {
		return term.telnet.Resize(cols, rows)
	}
	return term.session.WindowChange(int(rows), int(cols))
}

// Signal delivers a POSIX signal name (e.g. "KILL") to the remote shell.
func (s *Service) Signal(sessionID, signal string) error {
	term, ok := s.lookup(sessionID)
	if !ok {
		return fmt.Errorf("session %q not found", sessionID)
	}
	if term.pluginHooks != nil {
		if term.pluginHooks.Signal == nil {
			return nil
		}
		return term.pluginHooks.Signal(signal)
	}
	if term.helper != nil {
		_, err := term.helper.Write([]byte{3})
		return err
	}
	if term.local != nil {
		return term.local.Interrupt(term.local.Generation())
	}
	if term.telnet != nil {
		return term.telnet.Send([]byte{3})
	}
	_, err := term.session.SendRequest("signal", false, gossh.Marshal(struct{ Name string }{signal}))
	return err
}

// Close tears the PTY, transport and data plane route down.
func (s *Service) Close(sessionID string) error {
	return s.closeWithStatus(sessionID, TerminalExitStatus{SessionID: sessionID, Reason: "closed", Intentional: true})
}

func (s *Service) closeWithStatus(sessionID string, status TerminalExitStatus, expected ...*terminalSession) error {
	return s.beginSessionClose(sessionID, status, false, expected...)
}

func (s *Service) beginSessionClose(sessionID string, status TerminalExitStatus, async bool, expected ...*terminalSession) error {
	s.mu.Lock()
	term, ok := s.sessions[sessionID]
	if !ok || term.closing || (len(expected) > 0 && expected[0] != term) {
		s.mu.Unlock()
		return nil
	}
	term.closing = true
	if term.helper != nil {
		term.helper.Cancel()
	}
	if ok {
		if s.exitStatuses == nil {
			s.exitStatuses = make(map[string]TerminalExitStatus)
		}
		s.exitStatuses[sessionID] = status
		s.exitOrder = append(s.exitOrder, sessionID)
		if len(s.exitOrder) > 256 {
			delete(s.exitStatuses, s.exitOrder[0])
			s.exitOrder = s.exitOrder[1:]
		}
	}
	s.mu.Unlock()
	if !ok {
		return nil
	}
	if async {
		go s.finishSessionClose(sessionID, term, status)
		return nil
	}
	return s.finishSessionClose(sessionID, term, status)
}

func (s *Service) finishSessionClose(sessionID string, term *terminalSession, status TerminalExitStatus) error {
	// An attached popup must tear its observe route down first; tell it to
	// prepare for close before the route disappears.
	if term.attachAuthorization != "" || term.attachClosePrepared {
		s.emit("terminal:popup-prepare-close", map[string]any{
			"sessionId":     attachEventSessionID(term, sessionID),
			"authorization": term.attachAuthorization,
		})
	}
	if term.helper != nil {
		_ = term.helper.Close()
	}
	if term.local != nil {
		_ = term.local.Close()
	}
	if term.telnet != nil {
		_ = term.telnet.Close()
	}
	if term.serial != nil {
		_ = term.serial.Close(term.serialID)
	}
	if term.zmodem != nil {
		term.zmodem.cancel()
	}
	if term.serialYmodemCancel != nil {
		term.serialYmodemCancel()
	}
	// Plugin-protocol connections tear their plugin-side state down through
	// the hook; it must be idempotent (plugin-initiated closes already ran
	// it) and bounded (the plugin host caps every dispatch).
	if term.pluginHooks != nil && term.pluginHooks.Close != nil {
		term.pluginHooks.Close(status.Reason)
	}
	if term.runner != nil {
		_ = term.runner.Stop()
	}
	if term.x11 != nil {
		term.x11.Close()
	}
	if term.session != nil {
		term.session.Close()
	}
	if term.transport != nil {
		term.transport.Close()
	}
	s.emit("terminal:exit", status)
	s.mu.Lock()
	delete(s.sessions, sessionID)
	_ = s.controller.Close(sessionID)
	s.dp.DropOutput(sessionID)
	s.mu.Unlock()
	return nil
}

func (s *Service) lookup(sessionID string) (*terminalSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	term, ok := s.sessions[sessionID]
	return term, ok && !term.closing
}

// resolveSessionID maps a native or renderer session id to the live native id.
func (s *Service) resolveSessionID(idOrAlias string) (string, *terminalSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if term, ok := s.sessions[idOrAlias]; ok && !term.closing {
		return idOrAlias, term, true
	}
	for id, term := range s.sessions {
		if term.uiID != "" && term.uiID == idOrAlias && !term.closing {
			return id, term, true
		}
	}
	return "", nil, false
}

// attachEventSessionID picks the id the renderer knows the session by.
func attachEventSessionID(term *terminalSession, nativeID string) string {
	if term != nil && term.uiID != "" {
		return term.uiID
	}
	return nativeID
}

// handleUrgent writes urgent payloads (Ctrl-C et al.) straight to stdin.
func (s *Service) handleUrgent(sessionID string, payload []byte) []byte {
	term, ok := s.lookup(sessionID)
	if !ok {
		return nil
	}
	if term.helper != nil {
		if _, err := term.helper.Write(payload); err != nil {
			return nil
		}
		return []byte("ok")
	}
	if term.local != nil {
		if _, err := term.local.Write(term.local.Generation(), payload); err != nil {
			return nil
		}
		return []byte("ok")
	}
	if term.telnet != nil {
		if err := term.telnet.Send(payload); err != nil {
			return nil
		}
		return []byte("ok")
	}
	if term.serial != nil {
		if _, err := term.serial.Write(term.serialID, payload); err != nil {
			return nil
		}
		return []byte("ok")
	}
	if term.runner != nil {
		writer := term.runner.Writer()
		if writer == nil {
			return nil
		}
		if _, err := writer.Write(payload); err != nil {
			return nil
		}
		return []byte("ok")
	}
	if term.stdin == nil {
		return nil
	}
	if _, err := term.stdin.Write(payload); err != nil {
		return nil
	}
	return []byte("ok")
}

// TransportFor exposes a live session's authenticated SSH client for the SFTP
// subsystem seam. The returned validator reports whether the exact same
// session is still attached, so SFTP never opens a second authenticated
// connection behind the terminal's back.
func (s *Service) TransportFor(sessionID string) (*gossh.Client, func() bool, error) {
	s.mu.Lock()
	term := s.sessions[sessionID]
	if term == nil || term.transport == nil || term.transport.Client == nil {
		s.mu.Unlock()
		return nil, nil, fmt.Errorf("active SSH terminal %q not found", sessionID)
	}
	client := term.transport.Client
	s.mu.Unlock()
	return client, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.sessions[sessionID] == term
	}, nil
}
