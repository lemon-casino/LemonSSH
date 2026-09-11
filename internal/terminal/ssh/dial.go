package ssh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

// DialConfig is one authenticated dial attempt. JumpHosts nest: hops are
// dialed in order and each subsequent hop rides the previous transport.
type DialConfig struct {
	Hostname string
	Port     uint16
	Username string
	Auth     AuthMethod
	// HostKeyPolicy must be non-nil for every hop; nil fails closed.
	HostKeyPolicy HostKeyPolicy
	// Timeout bounds the TCP dial; HandshakeTimeout bounds the SSH handshake.
	Timeout          time.Duration
	HandshakeTimeout time.Duration
	// KeepaliveInterval enables periodic keepalive requests; 0 disables.
	KeepaliveInterval time.Duration
	// JumpHosts are dialed in order before Hostname.
	JumpHosts []DialConfig
	// ProxyURL optionally routes the TCP dial (socks5:// or http://).
	ProxyURL string
	// ProxyCommand optionally routes the transport through a user-supplied
	// shell command (OpenSSH ProxyCommand semantics, %h/%p tokens).
	ProxyCommand string
	// ForwardAgent marks transports that expose the local SSH agent to the
	// remote host. Such transports are never pooled for reuse (asymmetric
	// reuse policy, P3-04).
	ForwardAgent bool
}

var ErrHostKeyPolicyRequired = errors.New("ssh dial requires a host key policy")

// Transport is one authenticated SSH chain. Closing the transport closes the
// final client and every intermediate jump client.
type Transport struct {
	Client        *ssh.Client
	intermediates []*ssh.Client
	keepaliveStop chan struct{}
}

// Close tears down the chain in order.
func (t *Transport) Close() error {
	if t.keepaliveStop != nil {
		close(t.keepaliveStop)
		t.keepaliveStop = nil
	}
	var firstErr error
	for i := len(t.intermediates) - 1; i >= 0; i-- {
		if err := t.intermediates[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if t.Client != nil {
		if err := t.Client.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Dial returns an authenticated transport. All hops authenticate and verify
// host keys; any failure closes the hops established so far.
func Dial(ctx context.Context, config DialConfig) (*Transport, error) {
	chain := append(append([]DialConfig(nil), config.JumpHosts...), config)
	transport := &Transport{}
	for index, hop := range chain {
		if hop.HostKeyPolicy == nil {
			transport.Close()
			return nil, fmt.Errorf("hop %d (%s): %w", index, hop.Hostname, ErrHostKeyPolicyRequired)
		}
		client, err := dialOne(ctx, hop, transport.Client)
		if err != nil {
			transport.Close()
			return nil, fmt.Errorf("hop %d (%s): %w", index, hop.Hostname, err)
		}
		if index == len(chain)-1 {
			transport.Client = client
		} else {
			transport.intermediates = append(transport.intermediates, client)
		}
	}
	if config.KeepaliveInterval > 0 {
		transport.keepaliveStop = make(chan struct{})
		startKeepalive(transport.Client, config.KeepaliveInterval, transport.keepaliveStop)
	}
	return transport, nil
}

func dialOne(ctx context.Context, config DialConfig, via *ssh.Client) (*ssh.Client, error) {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	handshakeTimeout := config.HandshakeTimeout
	if handshakeTimeout == 0 {
		handshakeTimeout = 30 * time.Second
	}
	authMethods, err := BuildAuthMethods(config.Auth)
	if err != nil {
		return nil, err
	}
	address := net.JoinHostPort(config.Hostname, fmt.Sprintf("%d", portOrDefault(config.Port)))

	var connection net.Conn
	if via == nil && config.ProxyURL != "" {
		dialVia, proxyErr := ProxyDial(ctx, config.ProxyURL)
		if proxyErr != nil {
			return nil, proxyErr
		}
		proxied, proxiedErr := dialVia(ctx, "tcp", address)
		if proxiedErr != nil {
			return nil, fmt.Errorf("proxy dial %s via %s: %w", address, config.ProxyURL, proxiedErr)
		}
		connection = proxied
	}
	if connection == nil && via == nil && config.ProxyCommand != "" {
		proxied, proxiedErr := DialCommandProxy(ctx, config.ProxyCommand, address)
		if proxiedErr != nil {
			return nil, fmt.Errorf("proxy command dial %s: %w", address, proxiedErr)
		}
		connection = proxied
	}
	if via != nil {
		tunnel, tunnelErr := via.Dial("tcp", address)
		if tunnelErr != nil {
			return nil, fmt.Errorf("jump tunnel to %s: %w", address, tunnelErr)
		}
		connection = tunnel
	} else if connection == nil {
		dialer := &net.Dialer{Timeout: timeout}
		direct, dialErr := dialer.DialContext(ctx, "tcp", address)
		if dialErr != nil {
			return nil, fmt.Errorf("dial %s: %w", address, dialErr)
		}
		connection = direct
	}

	sshConfig := &ssh.ClientConfig{
		User:            config.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.HostKeyCallback(config.HostKeyPolicy),
		Timeout:         handshakeTimeout,
	}
	clientConn, channels, requests, handshakeErr := ssh.NewClientConn(connection, address, sshConfig)
	if handshakeErr != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("ssh handshake %s: %w", address, handshakeErr)
	}
	return ssh.NewClient(clientConn, channels, requests), nil
}

func portOrDefault(port uint16) uint16 {
	if port == 0 {
		return 22
	}
	return port
}

func startKeepalive(client *ssh.Client, interval time.Duration, stop chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if _, _, err := client.SendRequest("keepalive@netcatty", true, nil); err != nil {
					return
				}
			}
		}
	}()
}
