package ssh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	xnetproxy "golang.org/x/net/proxy"
)

// Agent support. On Windows the OpenSSH agent listens on a named pipe
// (\\.\pipe\openssh-ssh-agent); on Unix SSH_AUTH_SOCK points at the socket.
const windowsAgentPipe = `\\.\pipe\openssh-ssh-agent`

var ErrAgentUnavailable = errors.New("ssh agent unreachable")

// AgentAuthMethod builds an auth method backed by the local SSH agent. The
// agent connection is re-dialed lazily by x/crypto on each auth attempt.
func AgentAuthMethod(agentAddr string) (ssh.AuthMethod, error) {
	address := agentAddr
	if address == "" {
		address = agentAddressFromEnv()
	}
	if address == "" {
		return nil, ErrAgentUnavailable
	}
	connection, err := dialAgent(address)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAgentUnavailable, err)
	}
	client := agent.NewClient(connection)
	if _, err := client.List(); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("%w: %v", ErrAgentUnavailable, err)
	}
	return ssh.PublicKeysCallback(client.Signers), nil
}

func dialAgent(address string) (net.Conn, error) {
	connection, err := net.DialTimeout("unix", address, 3*time.Second)
	if err == nil {
		return connection, nil
	}
	if strings.HasPrefix(address, `\.\pipe\`) {
		return net.DialTimeout("pipe", address, 3*time.Second)
	}
	return nil, err
}

func agentAddressFromEnv() string {
	if socket := os.Getenv("SSH_AUTH_SOCK"); socket != "" {
		return socket
	}
	if runtime.GOOS == "windows" {
		return windowsAgentPipe
	}
	return ""
}

// ForwardAgentToClient wires the local agent to the remote session so
// agent-forwarding requests from the server succeed. Call before serving the
// session; then call Session-level RequestAgentForwarding.
func ForwardAgentToClient(client *ssh.Client, agentAddr string) error {
	address := agentAddr
	if address == "" {
		address = agentAddressFromEnv()
	}
	if address == "" {
		return ErrAgentUnavailable
	}
	connection, err := net.Dial("unix", address)
	if err != nil && strings.HasPrefix(address, `\\.\pipe\`) {
		connection, err = net.Dial("pipe", address)
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAgentUnavailable, err)
	}
	return agent.ForwardToAgent(client, agent.NewClient(connection))
}

// ProxyDial returns a proxy dial func for the given proxy URL
// (socks5://host:port or http://host:port). nil, nil means direct dial.
func ProxyDial(ctx context.Context, proxyURL string) (func(context.Context, string, string) (net.Conn, error), error) {
	if strings.TrimSpace(proxyURL) == "" {
		return nil, nil
	}
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy url: %w", err)
	}
	parsed, err := xnetproxy.FromURL(parsedURL, xnetproxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy url: %w", err)
	}
	contextDialer, ok := parsed.(xnetproxy.ContextDialer)
	if !ok {
		return func(_ context.Context, network, address string) (net.Conn, error) {
			return parsed.Dial(network, address)
		}, nil
	}
	return contextDialer.DialContext, nil
}
