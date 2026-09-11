package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/binaricat/netcatty/internal/terminal/dataplane"
	"github.com/binaricat/netcatty/internal/terminal/mosh"
	"github.com/binaricat/netcatty/internal/terminal/pty"
	"github.com/binaricat/netcatty/internal/terminal/serialport"
	"github.com/binaricat/netcatty/internal/terminal/ssh"
	"github.com/binaricat/netcatty/internal/terminal/supervised"
	"github.com/binaricat/netcatty/internal/terminal/telnet"
	"github.com/binaricat/netcatty/internal/terminal/ymodem"
)

// TerminalService owns SSH terminal sessions end-to-end: SSH dial → auth →
// PTY channel → data plane streaming → renderer WebSocket. Each session gets
// a route bootstrap (generation + one-use tokens) that the renderer exchanges
// for the loopback data/urgent WebSocket connections.
type TerminalService struct {
	mu            sync.Mutex
	sessions      map[string]*terminalSession
	controller    *dataplane.RouteController
	dp            *dataplane.Server
	knownHosts    *ssh.KnownHosts
	interactive   *ssh.InteractiveBroker
	emitChallenge func(ssh.KeyboardChallenge)
	emitEvent     func(name string, payload any)
	counter       int
}

// TelnetStartRequest is the Wails-facing telnet dial payload. Auto-login
// credentials are held in memory for the prompt exchange and never persisted.
type TelnetStartRequest struct {
	Hostname          string `json:"hostname"`
	Port              uint16 `json:"port"`
	Cols              uint16 `json:"cols"`
	Rows              uint16 `json:"rows"`
	Username          string `json:"username"`
	Password          string `json:"password"`
	AutoLogin         bool   `json:"autoLogin"`
	PromptTimeoutSecs int    `json:"promptTimeoutSecs"`
}

// SSHConnectRequest is the Wails-facing SSH dial payload. JumpHosts nest;
// command proxies and certificates remain fail-closed in the renderer mapper.
type SSHConnectRequest struct {
	Hostname          string              `json:"hostname"`
	Port              uint16              `json:"port"`
	Username          string              `json:"username"`
	Password          string              `json:"password"`
	PrivateKey        string              `json:"privateKey"`
	Passphrase        string              `json:"passphrase"`
	Certificate       string              `json:"certificate"`
	ProxyURL          string              `json:"proxyUrl"`
	EnableMFA         bool                `json:"enableMfa"`
	UseAgent          bool                `json:"useAgent"`
	IdentityFilePaths []string            `json:"identityFilePaths"`
	Cols              uint16              `json:"cols"`
	Rows              uint16              `json:"rows"`
	JumpHosts         []SSHConnectRequest `json:"jumpHosts"`
}

type terminalSession struct {
	transport          *ssh.Transport
	session            *gossh.Session
	local              *pty.Session
	telnet             *telnet.Client
	serial             *serialport.Session
	serialID           string
	serialYmodemCancel context.CancelFunc
	runner             *supervised.Runner
	stdin              io.WriteCloser
	bootstrap          dataplane.RouteBootstrap
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
	service.interactive = ssh.NewInteractiveBroker(func(challenge ssh.KeyboardChallenge) {
		if service.emitChallenge != nil {
			service.emitChallenge(challenge)
		}
	}, 2*time.Minute)
	dp.SetUrgentHandler(service.handleUrgent)
	return service
}

func (s *TerminalService) SetChallengeEmitter(emit func(ssh.KeyboardChallenge)) {
	s.emitChallenge = emit
}

// SetEventEmitter wires renderer-visible events (telnet echo mode, auto-login
// completion/cancellation) to the Wails event bus.
func (s *TerminalService) SetEventEmitter(emit func(name string, payload any)) {
	s.emitEvent = emit
}

func (s *TerminalService) emit(name string, payload any) {
	if s.emitEvent != nil {
		s.emitEvent(name, payload)
	}
}

// RespondKeyboardInteractive completes or cancels a pending MFA challenge.
func (s *TerminalService) RespondKeyboardInteractive(requestID string, responses []string, cancelled bool) error {
	return s.interactive.Respond(requestID, responses, cancelled)
}

func sshConnectToInput(request SSHConnectRequest) ssh.ConnectInput {
	input := ssh.ConnectInput{
		Hostname:          request.Hostname,
		Port:              request.Port,
		Username:          request.Username,
		Password:          request.Password,
		PrivateKey:        request.PrivateKey,
		Passphrase:        request.Passphrase,
		Certificate:       request.Certificate,
		ProxyURL:          request.ProxyURL,
		EnableMFA:         request.EnableMFA,
		UseAgent:          request.UseAgent,
		IdentityFilePaths: request.IdentityFilePaths,
	}
	if len(request.JumpHosts) > 0 {
		input.JumpHosts = make([]ssh.ConnectInput, 0, len(request.JumpHosts))
		for _, hop := range request.JumpHosts {
			input.JumpHosts = append(input.JumpHosts, sshConnectToInput(hop))
		}
	}
	return input
}

// Connect dials SSH, authenticates, opens a PTY shell and starts streaming
// output into the data plane. It returns the session ID; call Bootstrap to
// get the route credentials for the renderer WebSocket.
func (s *TerminalService) Connect(request SSHConnectRequest) (string, error) {
	if request.Hostname == "" || request.Username == "" {
		return "", fmt.Errorf("host and username are required")
	}
	if request.Port == 0 {
		request.Port = 22
	}
	if request.Cols == 0 {
		request.Cols = 80
	}
	if request.Rows == 0 {
		request.Rows = 24
	}
	policy := ssh.StrictPolicy(s.knownHosts)
	config, err := ssh.BuildDialConfigErr(sshConnectToInput(request), policy, s.interactive.Handler(request.Hostname))
	if err != nil {
		return "", err
	}
	transport, err := ssh.Dial(context.Background(), config)
	if err != nil {
		return "", fmt.Errorf("ssh dial %s:%d: %w", request.Hostname, request.Port, err)
	}

	sshSession, err := transport.Client.NewSession()
	if err != nil {
		transport.Close()
		return "", fmt.Errorf("new session: %w", err)
	}
	if err := sshSession.RequestPty("xterm-256color", int(request.Rows), int(request.Cols), gossh.TerminalModes{}); err != nil {
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

	bootstrap, err := s.controller.Open(fmt.Sprintf("%s@%s:%d", request.Username, request.Hostname, request.Port))
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

// MoshStartRequest is the Wails-facing Mosh/ET bootstrap payload. The union of
// SSH dial fields lets the handshake reuse the same auth path as Connect.
type MoshStartRequest struct {
	SSHConnectRequest
	// ClientPath is the absolute path to the local mosh-client / et binary.
	ClientPath string `json:"clientPath"`
	// ServerPath overrides the remote mosh-server command (empty uses default).
	ServerPath string `json:"serverPath"`
	Cols       uint16 `json:"cols"`
	Rows       uint16 `json:"rows"`
}

// StartMosh runs the mosh bootstrap over SSH and supervises the local
// mosh-client. The remote handshake scrapes the MOSH CONNECT line, then the
// client process streams through the same data plane as other sessions.
func (s *TerminalService) StartMosh(request MoshStartRequest) (string, error) {
	return s.startSupervisedTerminal(request, "mosh")
}

// StartEt runs the Eternal Terminal bootstrap. ET uses the same supervised
// process model; the remote command differs but the local supervision and
// data-plane wiring are identical to Mosh.
func (s *TerminalService) StartEt(request MoshStartRequest) (string, error) {
	return s.startSupervisedTerminal(request, "et")
}

// startSupervisedTerminal performs the shared handshake/supervision sequence.
func (s *TerminalService) startSupervisedTerminal(request MoshStartRequest, kind string) (string, error) {
	if request.Hostname == "" || request.Username == "" {
		return "", fmt.Errorf("host and username are required")
	}
	exeDir := ""
	if exePath, exeErr := os.Executable(); exeErr == nil {
		exeDir = filepath.Dir(exePath)
	}
	repoRoot, _ := os.Getwd()
	resolved, err := resolveHelperBinary(kind, request.ClientPath, os.Getenv(helperRootEnv), exeDir, repoRoot)
	if err != nil {
		return "", err
	}
	request.ClientPath = resolved
	s.mu.Lock()
	s.counter++
	sessionID := fmt.Sprintf("%s-%d", kind, s.counter)
	s.mu.Unlock()

	bootstrap, err := s.controller.Open(sessionID)
	if err != nil {
		return "", fmt.Errorf("open route: %w", err)
	}

	// The handshake needs the remote command's stdout, so it runs its own SSH
	// session rather than reusing the interactive Connect path.
	connect, err := s.runHandshake(request, kind)
	if err != nil {
		_ = s.controller.Close(sessionID)
		return "", err
	}

	info, err := os.Stat(request.ClientPath)
	if err != nil {
		_ = s.controller.Close(sessionID)
		return "", fmt.Errorf("%s: client binary %s: %w", kind, request.ClientPath, err)
	}
	if info.IsDir() {
		_ = s.controller.Close(sessionID)
		return "", fmt.Errorf("%s: client binary %s is a directory", kind, request.ClientPath)
	}
	digest, err := hashFile(request.ClientPath)
	if err != nil {
		_ = s.controller.Close(sessionID)
		return "", fmt.Errorf("%s: hash client binary: %w", kind, err)
	}
	manifest := supervised.Manifest{
		Name:   filepath.Base(request.ClientPath),
		Path:   filepath.Base(request.ClientPath),
		SHA256: digest,
		OS:     runtime.GOOS,
		Arch:   runtime.GOARCH,
	}
	runner, err := supervised.NewRunner(manifest, filepath.Dir(request.ClientPath), 3)
	if err != nil {
		_ = s.controller.Close(sessionID)
		return "", fmt.Errorf("%s: verify client binary: %w", kind, err)
	}
	runner.SetOutputHandler(func(data []byte) {
		if len(data) > 0 {
			s.dp.Publish(sessionID, data)
		}
	})
	env := map[string]string{"MOSH_KEY": connect.Key}
	if err := runner.StartWithEnv(context.Background(), mosh.ClientArgs(request.Hostname, connect), env); err != nil {
		_ = s.controller.Close(sessionID)
		return "", fmt.Errorf("%s: start client: %w", kind, err)
	}

	s.mu.Lock()
	s.sessions[sessionID] = &terminalSession{runner: runner, bootstrap: bootstrap}
	s.mu.Unlock()
	s.emit(kind+":session-ready", map[string]any{"sessionId": sessionID})
	return sessionID, nil
}

// runHandshake dials SSH and runs the remote mosh-server, returning the parsed
// MOSH CONNECT announcement.
func (s *TerminalService) runHandshake(request MoshStartRequest, kind string) (mosh.Connect, error) {
	if request.Port == 0 {
		request.Port = 22
	}
	policy := ssh.StrictPolicy(s.knownHosts)
	config, err := ssh.BuildDialConfigErr(sshConnectToInput(request.SSHConnectRequest), policy, nil)
	if err != nil {
		return mosh.Connect{}, err
	}
	transport, err := ssh.Dial(context.Background(), config)
	if err != nil {
		return mosh.Connect{}, fmt.Errorf("ssh dial %s:%d: %w", request.Hostname, request.Port, err)
	}
	defer transport.Close()
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
		return mosh.Connect{}, fmt.Errorf("start %s-server: %w", kind, err)
	}
	connect, err := scanForConnect(stdout, 30*time.Second)
	// The remote command exits after announcing; a non-zero exit here is still
	// usable when the announcement already arrived.
	_ = sshSession.Wait()
	if err != nil {
		return mosh.Connect{}, err
	}
	return connect, nil
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

// CancelZmodem honours the existing CRC/safety cancellation boundary. A
// cancellation is only meaningful for a live receiver, so it is reported as a
// no-op success when nothing is in flight rather than as a spurious failure.
func (s *TerminalService) CancelZmodem(sessionID string) error {
	s.mu.Lock()
	term, ok := s.sessions[sessionID]
	var cancel context.CancelFunc
	if ok && term.serialYmodemCancel != nil {
		cancel = term.serialYmodemCancel
	}
	s.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	return nil
}

// SendSerialYmodem uploads the file at filePath over the session's serial port
// using the YMODEM block protocol.
func (s *TerminalService) SendSerialYmodem(sessionID, filePath string) (ymodem.SendResult, error) {
	term, ok := s.lookup(sessionID)
	if !ok {
		return ymodem.SendResult{}, fmt.Errorf("session %q not found", sessionID)
	}
	if term.serial == nil {
		return ymodem.SendResult{}, fmt.Errorf("session %q is not a serial session", sessionID)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ymodem.SendResult{}, fmt.Errorf("read transfer source: %w", err)
	}
	ctx, cancel := s.beginSerialTransfer(sessionID, term)
	defer cancel()
	stream := serialStream{session: term.serial, port: term.serialID}
	return ymodem.Send(ctx, stream, ymodem.SendOptions{
		FileName: filepath.Base(filePath),
		Data:     data,
	})
}

// ReceiveSerialYmodem downloads files from the serial peer into destinationDir.
func (s *TerminalService) ReceiveSerialYmodem(sessionID, destinationDir string) ([]ymodem.ReceiveResult, error) {
	term, ok := s.lookup(sessionID)
	if !ok {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	if term.serial == nil {
		return nil, fmt.Errorf("session %q is not a serial session", sessionID)
	}
	ctx, cancel := s.beginSerialTransfer(sessionID, term)
	defer cancel()
	stream := serialStream{session: term.serial, port: term.serialID}
	return ymodem.Receive(ctx, stream, ymodem.ReceiveOptions{DestinationDir: destinationDir})
}

// beginSerialTransfer registers a cancellation handle for one serial transfer
// so CancelZmodem can abort it, and returns the transfer context.
func (s *TerminalService) beginSerialTransfer(sessionID string, term *terminalSession) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	term.serialYmodemCancel = cancel
	s.sessions[sessionID] = term
	s.mu.Unlock()
	return ctx, func() {
		cancel()
		s.mu.Lock()
		if current, ok := s.sessions[sessionID]; ok && current == term {
			term.serialYmodemCancel = nil
		}
		s.mu.Unlock()
	}
}

// serialStream adapts an open serial session to the YMODEM ByteStream. Reads
// return the bounded-timeout behaviour the engine expects; a timeout surfaces
// as an error so the engine's retry loop can re-NACK or give up.
type serialStream struct {
	session *serialport.Session
	port    string
}

func (s serialStream) Read(p []byte) (int, error) {
	n, err := s.session.Read(s.port, p)
	if err != nil {
		return n, err
	}
	if n == 0 {
		// The serial backend's read timeout yields a zero-byte read. Report it
		// as a timeout so the engine does not spin on a busy loop.
		return 0, os.ErrDeadlineExceeded
	}
	return n, nil
}

func (s serialStream) Write(p []byte) (int, error) { return s.session.Write(s.port, p) }

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
// data plane as SSH/local PTY. Echo mode transitions are surfaced as events so
// the renderer can disable local echo when the server takes over; when
// auto-login is requested the prompt exchange runs against the live stream.
func (s *TerminalService) StartTelnet(request TelnetStartRequest) (string, error) {
	host := request.Hostname
	port := request.Port
	cols := request.Cols
	rows := request.Rows
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
		switch event.Kind {
		case telnet.EventData:
			if len(event.Data) > 0 {
				s.dp.Publish(sessionID, event.Data)
			}
		case telnet.EventEchoMode:
			remote := string(event.Data) == "remote"
			s.emit("telnet:echo-mode", map[string]any{
				"sessionId":  sessionID,
				"remoteEcho": remote,
				"localEcho":  !remote,
			})
		case telnet.EventAutoLoginDone:
			s.emit("telnet:auto-login-complete", map[string]any{"sessionId": sessionID})
		case telnet.EventAutoLoginFail:
			s.emit("telnet:auto-login-cancelled", map[string]any{"sessionId": sessionID})
		case telnet.EventClosed:
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
	if request.AutoLogin && request.Username != "" {
		timeout := time.Duration(request.PromptTimeoutSecs) * time.Second
		if timeout <= 0 {
			timeout = 20 * time.Second
		}
		go func() {
			// AutoLogin emits the complete/cancelled event itself; the returned
			// error only carries the reason for diagnostics.
			_ = telnet.AutoLogin(context.Background(), client, request.Username, request.Password, timeout)
		}()
	}
	return sessionID, nil
}

// GetTelnetEchoMode reports whether the server currently echoes input.
func (s *TerminalService) GetTelnetEchoMode(sessionID string) (map[string]any, error) {
	term, ok := s.lookup(sessionID)
	if !ok {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	if term.telnet == nil {
		return nil, fmt.Errorf("session %q is not a telnet session", sessionID)
	}
	remote := term.telnet.RemoteEcho()
	return map[string]any{
		"success":    true,
		"sessionId":  sessionID,
		"remoteEcho": remote,
		"localEcho":  !remote,
	}, nil
}

// ListSerialPorts enumerates OS serial devices.
func (s *TerminalService) ListSerialPorts() ([]serialport.Info, error) {
	return serialport.NewOSBackend().List()
}

// SerialStartRequest is the Wails-facing serial open payload. The renderer
// forwards the full line configuration; the serial owner validates it and fails
// closed on combinations the backend cannot honour.
type SerialStartRequest struct {
	Path        string `json:"path"`
	BaudRate    int    `json:"baudRate"`
	DataBits    int    `json:"dataBits"`
	StopBits    string `json:"stopBits"`
	Parity      string `json:"parity"`
	FlowControl string `json:"flowControl"`
}

// StartSerial opens a serial port and streams bytes onto the data plane.
func (s *TerminalService) StartSerial(request SerialStartRequest) (string, error) {
	if request.Path == "" {
		return "", fmt.Errorf("serial path is required")
	}
	config := serialport.DefaultConfig(request.Path)
	if request.BaudRate > 0 {
		config.BaudRate = request.BaudRate
	}
	if request.DataBits != 0 {
		config.DataBits = request.DataBits
	}
	if request.StopBits != "" {
		config.StopBits = request.StopBits
	}
	if request.Parity != "" {
		config.Parity = request.Parity
	}
	if request.FlowControl != "" {
		config.FlowControl = request.FlowControl
	}
	backend := serialport.NewOSBackend()
	if err := backend.Open(config); err != nil {
		return "", err
	}
	s.mu.Lock()
	s.counter++
	sessionID := fmt.Sprintf("serial-%d", s.counter)
	s.mu.Unlock()
	bootstrap, err := s.controller.Open(sessionID)
	if err != nil {
		_ = backend.Close(request.Path)
		return "", err
	}
	s.mu.Lock()
	s.sessions[sessionID] = &terminalSession{serial: backend, serialID: request.Path, bootstrap: bootstrap}
	s.mu.Unlock()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := backend.Read(request.Path, buf)
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
	if term.serialYmodemCancel != nil {
		term.serialYmodemCancel()
	}
	if term.runner != nil {
		_ = term.runner.Stop()
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
