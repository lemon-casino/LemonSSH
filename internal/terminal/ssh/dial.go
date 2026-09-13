package ssh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
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
	KeepaliveCountMax int
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
	closeOnce     sync.Once
	closeErr      error
}

// Close tears down the chain in order.
func (t *Transport) Close() error {
	t.closeOnce.Do(func() {
		if t.keepaliveStop != nil {
			close(t.keepaliveStop)
		}
		if t.Client != nil {
			t.closeErr = t.Client.Close()
		}
		for i := len(t.intermediates) - 1; i >= 0; i-- {
			if err := t.intermediates[i].Close(); err != nil && t.closeErr == nil {
				t.closeErr = err
			}
		}
	})
	return t.closeErr
}

// Dial returns an authenticated transport. All hops authenticate and verify
// host keys; any failure closes the hops established so far.
func Dial(ctx context.Context, config DialConfig) (*Transport, error) {
	var chain []DialConfig
	var appendHops func(DialConfig)
	appendHops = func(hop DialConfig) {
		for _, child := range hop.JumpHosts {
			appendHops(child)
		}
		hop.JumpHosts = nil
		chain = append(chain, hop)
	}
	appendHops(config)
	// A target proxy reaches the first hop, unless that hop specifies its own.
	if len(chain) > 1 {
		if chain[0].ProxyURL == "" && chain[0].ProxyCommand == "" {
			chain[0].ProxyURL, chain[0].ProxyCommand = config.ProxyURL, config.ProxyCommand
		}
		chain[len(chain)-1].ProxyURL, chain[len(chain)-1].ProxyCommand = "", ""
	}
	transport := &Transport{}
	for index, hop := range chain {
		if index > 0 && (hop.ProxyURL != "" || hop.ProxyCommand != "") {
			transport.Close()
			return nil, fmt.Errorf("hop %d: a proxy can only precede the first SSH hop", index)
		}
		if hop.HostKeyPolicy == nil {
			transport.Close()
			return nil, fmt.Errorf("hop %d (%s): %w", index, hop.Hostname, ErrHostKeyPolicyRequired)
		}
		client, err := dialOne(ctx, hop, transport.Client)
		if err != nil {
			transport.Close()
			return nil, fmt.Errorf("hop %d (%s): %w", index, hop.Hostname, err)
		}
		if transport.Client != nil {
			transport.intermediates = append(transport.intermediates, transport.Client)
		}
		transport.Client = client
	}
	if config.KeepaliveInterval > 0 {
		transport.keepaliveStop = make(chan struct{})
		go runKeepalive(transport.Client, config.KeepaliveInterval, config.KeepaliveCountMax, transport.keepaliveStop)
	}
	return transport, nil
}

func dialOne(ctx context.Context, config DialConfig, via *ssh.Client) (*ssh.Client, error) {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	dialCtx, cancelDial := context.WithTimeout(ctx, timeout)
	defer cancelDial()
	handshakeTimeout := config.HandshakeTimeout
	if handshakeTimeout == 0 {
		handshakeTimeout = 30 * time.Second
	}
	authMethods, err := buildAuthMethods(ctx, config.Auth)
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
		proxied, proxiedErr := dialVia(dialCtx, "tcp", address)
		if proxiedErr != nil {
			return nil, fmt.Errorf("proxy dial %s failed", address)
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
		tunnel, tunnelErr := via.DialContext(dialCtx, "tcp", address)
		if tunnelErr != nil {
			return nil, fmt.Errorf("jump tunnel to %s: %w", address, tunnelErr)
		}
		connection = tunnel
	} else if connection == nil {
		dialer := &net.Dialer{Timeout: timeout}
		direct, dialErr := dialer.DialContext(dialCtx, "tcp", address)
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
	// ClientConfig.Timeout is not applied by NewClientConn, and forwarded SSH
	// channels do not implement deadlines. Closing the stream bounds both paths.
	handshakeCtx, cancelHandshake := context.WithTimeout(ctx, handshakeTimeout)
	stopClose := context.AfterFunc(handshakeCtx, func() { _ = connection.Close() })
	clientConn, channels, requests, handshakeErr := ssh.NewClientConn(connection, address, sshConfig)
	stopped := stopClose()
	cancelHandshake()
	if !stopped && handshakeErr == nil {
		_ = clientConn.Close()
		return nil, context.DeadlineExceeded
	}
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
