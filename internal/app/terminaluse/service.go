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
	"fmt"
	"io"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/binaricat/netcatty/internal/platform/filesystem"
	"github.com/binaricat/netcatty/internal/terminal/dataplane"
	"github.com/binaricat/netcatty/internal/terminal/pty"
	"github.com/binaricat/netcatty/internal/terminal/serialport"
	"github.com/binaricat/netcatty/internal/terminal/ssh"
	"github.com/binaricat/netcatty/internal/terminal/supervised"
	"github.com/binaricat/netcatty/internal/terminal/telnet"
)

// Service owns terminal sessions end-to-end: SSH dial → auth → PTY channel →
// data plane streaming → renderer route bootstrap. Each session gets a route
// bootstrap (generation + one-use tokens) that the shell exchanges for the
// loopback data/urgent WebSocket connections.
type Service struct {
	mu            sync.Mutex
	sessions      map[string]*terminalSession
	exitStatuses  map[string]TerminalExitStatus
	exitOrder     []string
	controller    *dataplane.RouteController
	dp            *dataplane.Server
	knownHosts    *ssh.KnownHosts
	interactive   *ssh.InteractiveBroker
	emitChallenge func(ssh.KeyboardChallenge)
	emitEvent     func(name string, payload any)
	observeOutput func(sessionID string, data []byte)
	counter       int
	helperTemp    *filesystem.TempService
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
	telnet             *telnet.Client
	serial             *serialport.Session
	serialID           string
	serialYmodemCancel context.CancelFunc
	runner             *supervised.Runner
	helper             *supervised.Terminal
	helperFactory      supervised.TerminalFactory
	helperState        HelperSessionState
	stdin              io.WriteCloser
	bootstrap          dataplane.RouteBootstrap
}

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
	dp.SetUrgentHandler(service.handleUrgent)
	return service
}

// SetChallengeEmitter wires the keyboard-interactive challenge callback.
func (s *Service) SetChallengeEmitter(emit func(ssh.KeyboardChallenge)) {
	s.emitChallenge = emit
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
	if term := s.sessions[sessionID]; term != nil {
		term.cwd.feed(data)
	}
	observe := s.observeOutput
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

// Reconnect rotates only the renderer route. Native Mosh/ET processes retain
// their protocol keys and roaming state; no new remote server is bootstrapped.
func (s *Service) Reconnect(sessionID string) (dataplane.RouteBootstrap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	term, ok := s.sessions[sessionID]
	if !ok || term.closing {
		return dataplane.RouteBootstrap{}, fmt.Errorf("session not found")
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

// Write sends raw stdin bytes to the remote shell.
func (s *Service) Write(sessionID string, data []byte) (int, error) {
	term, ok := s.lookup(sessionID)
	if !ok {
		return 0, fmt.Errorf("session %q not found", sessionID)
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
