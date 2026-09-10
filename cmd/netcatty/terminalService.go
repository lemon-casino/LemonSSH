package main

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/binaricat/netcatty/internal/terminal/dataplane"
	"github.com/binaricat/netcatty/internal/terminal/pty"
	"github.com/binaricat/netcatty/internal/terminal/serialport"
	"github.com/binaricat/netcatty/internal/terminal/ssh"
	"github.com/binaricat/netcatty/internal/terminal/telnet"
)

// TerminalService owns SSH terminal sessions end-to-end: SSH dial → auth →
// PTY channel → data plane streaming → renderer WebSocket. Each session gets
// a route bootstrap (generation + one-use tokens) that the renderer exchanges
// for the loopback data/urgent WebSocket connections.
type TerminalService struct {
	mu         sync.Mutex
	sessions   map[string]*terminalSession
	controller *dataplane.RouteController
	dp         *dataplane.Server
	knownHosts *ssh.KnownHosts
	counter    int
}

type terminalSession struct {
	transport *ssh.Transport
	session   *gossh.Session
	local     *pty.Session
	telnet    *telnet.Client
	serial    *serialport.Session
	serialID  string
	stdin     io.WriteCloser
	bootstrap dataplane.RouteBootstrap
}

// NewTerminalService wires the route controller, transport and known-hosts
// store together. The urgent channel (Ctrl-C et al.) is handled in-process by
// writing the payload to the session's stdin.
func NewTerminalService(controller *dataplane.RouteController, dp *dataplane.Server, knownHosts *ssh.KnownHosts) *TerminalService {
	service := &TerminalService{
		sessions:   make(map[string]*terminalSession),
		controller: controller,
		dp:         dp,
		knownHosts: knownHosts,
	}
	dp.SetUrgentHandler(service.handleUrgent)
	return service
}

// Connect dials SSH, authenticates, opens a PTY shell and starts streaming
// output into the data plane. It returns the session ID; call Bootstrap to
// get the route credentials for the renderer WebSocket.
func (s *TerminalService) Connect(host string, port uint16, username, password, privateKey, passphrase string, cols, rows uint16) (string, error) {
	if host == "" || username == "" {
		return "", fmt.Errorf("host and username are required")
	}
	if port == 0 {
		port = 22
	}
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}
	config := ssh.DialConfig{
		Hostname:          host,
		Port:              port,
		Username:          username,
		Auth: ssh.AuthMethod{
			Password:      password,
			PrivateKeyPEM: []byte(privateKey),
			Passphrase:    passphrase,
		},
		HostKeyPolicy:     ssh.StrictPolicy(s.knownHosts),
		Timeout:           15 * time.Second,
		HandshakeTimeout:  15 * time.Second,
		KeepaliveInterval: 30 * time.Second,
	}
	transport, err := ssh.Dial(context.Background(), config)
	if err != nil {
		return "", fmt.Errorf("ssh dial %s:%d: %w", host, port, err)
	}

	sshSession, err := transport.Client.NewSession()
	if err != nil {
		transport.Close()
		return "", fmt.Errorf("new session: %w", err)
	}
	if err := sshSession.RequestPty("xterm-256color", int(rows), int(cols), gossh.TerminalModes{}); err != nil {
		sshSession.Close()
		transport.Close()
		return "", fmt.Errorf("pty request: %w", err)
	}
	stdin, err := sshSession.StdinPipe()
	if err != nil {
		sshSession.Close()
		transport.Close()
		return "", fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := sshSession.StdoutPipe()
	if err != nil {
		sshSession.Close()
		transport.Close()
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	if err := sshSession.Shell(); err != nil {
		sshSession.Close()
		transport.Close()
		return "", fmt.Errorf("shell request: %w", err)
	}

	bootstrap, err := s.controller.Open(fmt.Sprintf("%s@%s:%d", username, host, port))
	if err != nil {
		sshSession.Close()
		transport.Close()
		return "", fmt.Errorf("open route: %w", err)
	}
	sessionID := bootstrap.SessionID

	s.mu.Lock()
	s.counter++
	term := &terminalSession{transport: transport, session: sshSession, stdin: stdin, bootstrap: bootstrap}
	s.sessions[sessionID] = term
	s.mu.Unlock()

	// Pump SSH stdout → data plane. The controller admits frames against the
	// renderer's credit, so the pump never needs its own backpressure.
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, readErr := stdout.Read(buf)
			if n > 0 {
				s.dp.Publish(sessionID, buf[:n])
			}
			if readErr != nil {
				return
			}
		}
	}()

	// When the remote side closes the shell, tear the session down.
	go func() {
		_ = sshSession.Wait()
		s.Close(sessionID)
	}()

	return sessionID, nil
}

// StartLocal launches a local PTY and streams it on the same data plane as SSH.
func (s *TerminalService) StartLocal(shell, cwd string, cols, rows uint16) (string, error) {
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}
	s.mu.Lock()
	s.counter++
	sessionID := fmt.Sprintf("local-%d", s.counter)
	s.mu.Unlock()

	local := pty.NewSession(pty.BuildConfig(sessionID, shell, cwd, nil, nil, cols, rows))
	if err := local.Start(context.Background(), pty.NewPlatformBackend()); err != nil {
		return "", fmt.Errorf("local pty: %w", err)
	}
	bootstrap, err := s.controller.Open(sessionID)
	if err != nil {
		_ = local.Close()
		return "", fmt.Errorf("open route: %w", err)
	}

	s.mu.Lock()
	s.sessions[sessionID] = &terminalSession{local: local, bootstrap: bootstrap}
	s.mu.Unlock()

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, readErr := local.ReadOnce(buf)
			if n > 0 {
				s.dp.Publish(sessionID, buf[:n])
			}
			if readErr != nil {
				s.Close(sessionID)
				return
			}
		}
	}()
		return sessionID, nil
	}

	// StartTelnet dials a Telnet host and streams IAC-decoded data onto the same
	// data plane as SSH/local PTY.
	func (s *TerminalService) StartTelnet(host string, port uint16, cols, rows uint16) (string, error) {
		if host == "" {
			return "", fmt.Errorf("host is required")
		}
		if port == 0 {
			port = 23
		}
		if cols == 0 {
			cols = 80
		}
		if rows == 0 {
			rows = 24
		}
		s.mu.Lock()
		s.counter++
		sessionID := fmt.Sprintf("telnet-%d", s.counter)
		s.mu.Unlock()
		bootstrap, err := s.controller.Open(sessionID)
		if err != nil {
			return "", fmt.Errorf("open route: %w", err)
		}
		client, err := telnet.Connect(context.Background(), fmt.Sprintf("%s:%d", host, port), func(event telnet.Event) {
			if event.Kind == telnet.EventData && len(event.Data) > 0 {
				s.dp.Publish(sessionID, event.Data)
			}
			if event.Kind == telnet.EventClosed {
				s.Close(sessionID)
			}
		})
		if err != nil {
			_ = s.controller.Close(sessionID)
			return "", err
		}
		_ = client.Resize(cols, rows)
		s.mu.Lock()
		s.sessions[sessionID] = &terminalSession{telnet: client, bootstrap: bootstrap}
		s.mu.Unlock()
		return sessionID, nil
	}

	// ListSerialPorts enumerates OS serial devices.
	func (s *TerminalService) ListSerialPorts() ([]serialport.Info, error) {
		return serialport.NewOSBackend().List()
	}

	// StartSerial opens a serial port and streams bytes onto the data plane.
	func (s *TerminalService) StartSerial(path string, baudRate int) (string, error) {
		if path == "" {
			return "", fmt.Errorf("serial path is required")
		}
		backend := serialport.NewOSBackend()
		config := serialport.DefaultConfig(path)
		if baudRate > 0 {
			config.BaudRate = baudRate
		}
		if err := backend.Open(config); err != nil {
			return "", err
		}
		s.mu.Lock()
		s.counter++
		sessionID := fmt.Sprintf("serial-%d", s.counter)
		s.mu.Unlock()
		bootstrap, err := s.controller.Open(sessionID)
		if err != nil {
			_ = backend.Close(path)
			return "", err
		}
		s.mu.Lock()
		s.sessions[sessionID] = &terminalSession{serial: backend, serialID: path, bootstrap: bootstrap}
		s.mu.Unlock()
		go func() {
			buf := make([]byte, 4096)
			for {
				n, readErr := backend.Read(path, buf)
				if n > 0 {
					s.dp.Publish(sessionID, buf[:n])
				}
				if readErr != nil {
					s.Close(sessionID)
					return
				}
			}
		}()
		return sessionID, nil
	}

	// Bootstrap returns the route credentials the renderer needs to attach its
// data/urgent WebSockets for a session.
func (s *TerminalService) Bootstrap(sessionID string) (dataplane.RouteBootstrap, error) {
	s.mu.Lock()
	term, ok := s.sessions[sessionID]
	s.mu.Unlock()
	if !ok {
		return dataplane.RouteBootstrap{}, fmt.Errorf("session %q not found", sessionID)
	}
	return term.bootstrap, nil
}

// ListenAddr exposes the bound loopback data plane address (host:port).
func (s *TerminalService) ListenAddr() string { return s.dp.Addr() }

// Write sends raw stdin bytes to the remote shell.
func (s *TerminalService) Write(sessionID string, data []byte) (int, error) {
	term, ok := s.lookup(sessionID)
	if !ok {
		return 0, fmt.Errorf("session %q not found", sessionID)
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
		return term.stdin.Write(data)
	}

// Resize updates the remote PTY window size.
func (s *TerminalService) Resize(sessionID string, cols, rows uint16) error {
	term, ok := s.lookup(sessionID)
	if !ok {
		return fmt.Errorf("session %q not found", sessionID)
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
func (s *TerminalService) Signal(sessionID, signal string) error {
	term, ok := s.lookup(sessionID)
	if !ok {
		return fmt.Errorf("session %q not found", sessionID)
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
func (s *TerminalService) Close(sessionID string) error {
	s.mu.Lock()
	term, ok := s.sessions[sessionID]
	delete(s.sessions, sessionID)
	s.mu.Unlock()
	if !ok {
		return nil
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
		if term.session != nil {
		term.session.Close()
	}
	if term.transport != nil {
		term.transport.Close()
	}
	_ = s.controller.Close(sessionID)
	s.dp.DropOutput(sessionID)
	return nil
}

func (s *TerminalService) lookup(sessionID string) (*terminalSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	term, ok := s.sessions[sessionID]
	return term, ok
}

// handleUrgent writes urgent payloads (Ctrl-C et al.) straight to stdin.
func (s *TerminalService) handleUrgent(sessionID string, payload []byte) []byte {
	term, ok := s.lookup(sessionID)
	if !ok {
		return nil
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
	if term.stdin == nil {
		return nil
	}
	if _, err := term.stdin.Write(payload); err != nil {
		return nil
	}
	return []byte("ok")
}
