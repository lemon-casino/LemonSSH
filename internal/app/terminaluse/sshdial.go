package terminaluse

import (
	"github.com/binaricat/netcatty/internal/terminal/ssh"
	gossh "golang.org/x/crypto/ssh"
	"time"
)

func terminalTerm(request SSHConnectRequest) string {
	if request.Term != "" {
		return request.Term
	}
	return "xterm-256color"
}

// TermName reports the PTY term type requested for a session.
func TermName(request SSHConnectRequest) string { return terminalTerm(request) }

// SSHDialConfig builds the transport dial config with the strict known-host
// policy and keepalive/verify options applied. Exported for facades that share
// the same SSH auth path (SFTP, port forward).
func SSHDialConfig(request SSHConnectRequest, hosts *ssh.KnownHosts, challenge func(string, string, []string, []bool) ([]string, error)) (ssh.DialConfig, error) {
	return terminalSSHDialConfig(request, hosts, challenge)
}

// ConnectInputFromRequest maps the shell-facing request DTO to the transport
// dial input, including nested jump hosts.
func ConnectInputFromRequest(request SSHConnectRequest) ssh.ConnectInput {
	return sshConnectToInput(request)
}

func terminalSSHDialConfig(request SSHConnectRequest, hosts *ssh.KnownHosts, challenge func(string, string, []string, []bool) ([]string, error)) (ssh.DialConfig, error) {
	config, err := ssh.BuildDialConfigErr(sshConnectToInput(request), ssh.StrictPolicy(hosts), challenge)
	if err != nil {
		return config, err
	}
	applyTerminalSSHOptions(&config, request, hosts)
	return config, nil
}

func applyTerminalSSHOptions(config *ssh.DialConfig, request SSHConnectRequest, hosts *ssh.KnownHosts) {
	config.HostKeyPolicy = ssh.StrictPolicy(hosts)
	if request.VerifyHostKeys != nil && !*request.VerifyHostKeys {
		config.HostKeyPolicy = ssh.HostKeyPolicy(gossh.InsecureIgnoreHostKey())
	}
	if request.KeepaliveInterval != nil {
		seconds := *request.KeepaliveInterval
		if seconds < 0 {
			seconds = 0
		}
		config.KeepaliveInterval = time.Duration(seconds) * time.Second
	}
	config.KeepaliveCountMax = 3
	if request.KeepaliveCountMax != nil && *request.KeepaliveCountMax > 0 {
		config.KeepaliveCountMax = *request.KeepaliveCountMax
	}
	for i := range config.JumpHosts {
		applyTerminalSSHOptions(&config.JumpHosts[i], request.JumpHosts[i], hosts)
	}
}
