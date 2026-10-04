package terminaluse

import (
	"context"
	"fmt"
	"runtime"

	gossh "golang.org/x/crypto/ssh"

	"github.com/lemon-casino/lemonssh/internal/platform/sshdebug"
	"github.com/lemon-casino/lemonssh/internal/terminal/ssh"
)

// SSHConnectRequest is the shell-facing SSH dial payload. JumpHosts nest;
// proxyCommand carries OpenSSH ProxyCommand semantics (%h/%p tokens).
// SessionID/BootEpoch carry the renderer session correlation echoed back on
// interactive prompts (passphrase, host-key confirmation); KeyPath labels the
// private key in passphrase prompts.
type SSHConnectRequest struct {
	Hostname     string `json:"hostname"`
	Port         uint16 `json:"port"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	PrivateKey   string `json:"privateKey"`
	Passphrase   string `json:"passphrase"`
	Certificate  string `json:"certificate"`
	ProxyURL     string `json:"proxyUrl"`
	ProxyCommand string `json:"proxyCommand"`
	EnableMFA    bool   `json:"enableMfa"`
	UseAgent     bool   `json:"useAgent"`
	// AgentForwarding exposes the local SSH agent to the remote host
	// (OpenSSH ForwardAgent yes). Each hop carries its own flag.
	AgentForwarding   bool                `json:"agentForwarding"`
	IdentityFilePaths []string            `json:"identityFilePaths"`
	Cols              uint16              `json:"cols"`
	Rows              uint16              `json:"rows"`
	Term              string              `json:"term"`
	VerifyHostKeys    *bool               `json:"verifyHostKeys"`
	KeepaliveInterval *int                `json:"keepaliveInterval"`
	KeepaliveCountMax *int                `json:"keepaliveCountMax"`
	ForwardX11        bool                `json:"forwardX11"`
	X11Display        string              `json:"x11Display"`
	SessionID         string              `json:"sessionId,omitempty"`
	BootEpoch         int                 `json:"bootEpoch,omitempty"`
	KeyPath           string              `json:"keyPath,omitempty"`
	JumpHosts         []SSHConnectRequest `json:"jumpHosts"`
	// SSHDebugLogs carries the renderer's SSH debug log setting so each dial
	// syncs the process-global ssh-debug.log toggle.
	SSHDebugLogs bool `json:"sshDebugLogs,omitempty"`
}

func sshConnectToInput(request SSHConnectRequest) ssh.ConnectInput {
	input := ssh.ConnectInput{
		Hostname:          request.Hostname,
		Port:              request.Port,
		Username:          request.Username,
		Password:          request.Password,
		PrivateKey:        request.PrivateKey,
		Passphrase:        request.Passphrase,
		KeyPath:           request.KeyPath,
		Certificate:       request.Certificate,
		ProxyURL:          request.ProxyURL,
		ProxyCommand:      request.ProxyCommand,
		EnableMFA:         request.EnableMFA,
		UseAgent:          request.UseAgent,
		AgentForwarding:   request.AgentForwarding,
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
func (s *Service) Connect(request SSHConnectRequest) (string, error) {
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
	config, err := terminalSSHDialConfig(request, s.knownHosts, s.DialInteractive(context.Background(), request))
	if err != nil {
		return "", err
	}
	// The renderer's settings toggle rides every dial: sync the process-global
	// debug state so the log trail starts with this connection.
	sshdebug.SetEnabled(request.SSHDebugLogs)
	sshdebug.Logf("ssh dial start host=%s port=%d jumpHosts=%d", request.Hostname, request.Port, len(request.JumpHosts))
	transport, err := ssh.Dial(context.Background(), config)
	if err != nil {
		s.notifyPassphraseRejected(err)
		sshdebug.LogError("ssh dial failed host=%s port=%d err=%v", request.Hostname, request.Port, err)
		return "", fmt.Errorf("ssh dial %s:%d: %w", request.Hostname, request.Port, err)
	}
	sshdebug.Logf("ssh transport established host=%s port=%d", request.Hostname, request.Port)

	// Agent forwarding has two halves (see ssh.ForwardAgentToClient): wire the
	// local agent onto the client transport, then request forwarding on the
	// session before the shell starts. A missing local agent degrades the
	// session to "no forwarding" instead of failing the dial — OpenSSH parity;
	// the host editor's agent check surfaces availability to the user.
	agentForwarded := false
	if config.ForwardAgent {
		if err := ssh.ForwardAgentToClient(transport.Client, ""); err != nil {
			sshdebug.LogError("ssh agent forwarding unavailable host=%s err=%v", request.Hostname, err)
		} else {
			agentForwarded = true
		}
	}

	sshSession, err := transport.Client.NewSession()
	if err != nil {
		transport.Close()
		return "", fmt.Errorf("new session: %w", err)
	}
	if err := sshSession.RequestPty(terminalTerm(request), int(request.Rows), int(request.Cols), gossh.TerminalModes{}); err != nil {
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
	var x11 *ssh.X11Forwarder
	if request.ForwardX11 {
		display, xerr := ssh.ParseX11Display(request.X11Display, runtime.GOOS)
		if xerr == nil {
			var cookie []byte
			cookie, xerr = ssh.ReadX11Cookie(context.Background(), display)
			if xerr == nil {
				x11, xerr = ssh.StartX11Forwarding(context.Background(), transport.Client, sshSession, display, cookie)
			}
		}
		if xerr != nil {
			sshSession.Close()
			transport.Close()
			return "", fmt.Errorf("X11 forwarding: %w", xerr)
		}
	}
	// auth-agent-req@openssh.com must precede the shell request to take effect.
	if agentForwarded {
		if err := ssh.RequestAgentForwarding(sshSession); err != nil {
			sshdebug.LogError("ssh agent forwarding request failed host=%s err=%v", request.Hostname, err)
		}
	}
	if err := sshSession.Shell(); err != nil {
		if x11 != nil {
			x11.Close()
		}
		sshSession.Close()
		transport.Close()
		return "", fmt.Errorf("shell request: %w", err)
	}

	bootstrap, err := s.controller.Open(fmt.Sprintf("%s@%s:%d", request.Username, request.Hostname, request.Port))
	if err != nil {
		if x11 != nil {
			x11.Close()
		}
		sshSession.Close()
		transport.Close()
		return "", fmt.Errorf("open route: %w", err)
	}
	sessionID := bootstrap.SessionID

	s.mu.Lock()
	s.counter++
	term := &terminalSession{x11: x11, transport: transport, session: sshSession, stdin: stdin, bootstrap: bootstrap, uiID: request.SessionID}
	s.sessions[sessionID] = term
	s.mu.Unlock()
	sshdebug.Logf("ssh session open session=%s host=%s", sessionID, request.Hostname)

	// Pump SSH stdout → data plane. The controller admits frames against the
	// renderer's credit, so the pump never needs its own backpressure.
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, readErr := stdout.Read(buf)
			if n > 0 {
				if !s.publishOutput(sessionID, buf[:n]) {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()

	// When the remote side closes the shell, tear the session down.
	go func() {
		status := terminalWaitExit(sessionID, sshSession.Wait())
		sshdebug.Logf("ssh session exit session=%s reason=%s exitCodeSet=%t err=%s",
			sessionID, status.Reason, status.ExitCode != nil, status.Error)
		_ = s.closeWithStatus(sessionID, status)
	}()

	return sessionID, nil
}
