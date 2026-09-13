package ssh

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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
	return agentAuthWithCertificate(context.Background(), agentAddr, nil)
}

func agentAuthWithCertificate(ctx context.Context, agentAddr string, certificate []byte) (ssh.AuthMethod, error) {
	var cert *ssh.Certificate
	if len(certificate) > 0 {
		parsed, _, _, _, err := ssh.ParseAuthorizedKey(certificate)
		if err != nil {
			return nil, errors.New("invalid agent certificate")
		}
		var ok bool
		cert, ok = parsed.(*ssh.Certificate)
		if !ok {
			return nil, errors.New("not an OpenSSH certificate")
		}
	}
	address := agentAddr
	if address == "" {
		address = agentAddressFromEnv()
	}
	if address == "" {
		return nil, ErrAgentUnavailable
	}
	connection, err := openAuthAgent(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAgentUnavailable, err)
	}
	client := agent.NewClient(connection)
	_, err = client.List()
	_ = connection.Close()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAgentUnavailable, err)
	}
	return ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
		conn, err := openAuthAgent(ctx, address)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		keys, err := agent.NewClient(conn).List()
		if err != nil {
			return nil, err
		}
		result := make([]ssh.Signer, 0, len(keys))
		for _, key := range keys {
			var signer ssh.Signer = &agentSigner{ctx: ctx, address: address, key: key}
			if cert != nil {
				if !bytes.Equal(cert.Key.Marshal(), key.Marshal()) {
					continue
				}
				signer, err = ssh.NewCertSigner(cert, signer)
				if err != nil {
					return nil, err
				}
			}
			result = append(result, signer)
		}
		if len(result) == 0 && cert != nil {
			return nil, errors.New("certificate key not available in SSH agent")
		}
		return result, nil
	}), nil
}

type agentSigner struct {
	ctx     context.Context
	address string
	key     ssh.PublicKey
}

func (s *agentSigner) PublicKey() ssh.PublicKey { return s.key }
func (s *agentSigner) Sign(random io.Reader, data []byte) (*ssh.Signature, error) {
	return s.SignWithAlgorithm(random, data, "")
}
func (s *agentSigner) SignWithAlgorithm(_ io.Reader, data []byte, algorithm string) (*ssh.Signature, error) {
	conn, err := openAuthAgent(s.ctx, s.address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	flags := agent.SignatureFlags(0)
	switch algorithm {
	case ssh.KeyAlgoRSASHA256:
		flags = agent.SignatureFlagRsaSha256
	case ssh.KeyAlgoRSASHA512:
		flags = agent.SignatureFlagRsaSha512
	case "", s.key.Type():
	default:
		return nil, fmt.Errorf("unsupported agent signature algorithm %q", algorithm)
	}
	return agent.NewClient(conn).SignWithFlags(s.key, data, flags)
}

type authAgentConn struct {
	net.Conn
	stop func() bool
}

func (c *authAgentConn) Close() error { c.stop(); return c.Conn.Close() }
func openAuthAgent(ctx context.Context, address string) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := dialAgent(address)
	if err != nil {
		return nil, err
	}
	return &authAgentConn{Conn: conn, stop: context.AfterFunc(ctx, func() { _ = conn.Close() })}, nil
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
		return nil, errors.New("invalid proxy url")
	}
	if parsedURL.Scheme == "http" {
		return func(ctx context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" {
				return nil, errors.New("HTTP proxy requires TCP")
			}
			endpoint := parsedURL.Host
			if parsedURL.Port() == "" {
				endpoint = net.JoinHostPort(parsedURL.Hostname(), "80")
			}
			conn, err := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, "tcp", endpoint)
			if err != nil {
				return nil, err
			}
			stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
			defer stop()
			req := &http.Request{Method: "CONNECT", URL: &url.URL{Opaque: address}, Host: address, Header: make(http.Header)}
			if parsedURL.User != nil {
				password, _ := parsedURL.User.Password()
				req.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(parsedURL.User.Username()+":"+password)))
			}
			if err = req.Write(conn); err != nil {
				conn.Close()
				return nil, err
			}
			reader := bufio.NewReader(conn)
			response, err := http.ReadResponse(reader, req)
			if err != nil {
				conn.Close()
				return nil, errors.New("HTTP CONNECT response failed")
			}
			if response.StatusCode != http.StatusOK {
				conn.Close()
				return nil, fmt.Errorf("HTTP CONNECT status %d", response.StatusCode)
			}
			return &proxyBufferedConn{Conn: conn, reader: reader}, nil
		}, nil
	}
	parsed, err := xnetproxy.FromURL(parsedURL, xnetproxy.Direct)
	if err != nil {
		return nil, errors.New("unsupported or invalid proxy url")
	}
	contextDialer, ok := parsed.(xnetproxy.ContextDialer)
	if !ok {
		return func(_ context.Context, network, address string) (net.Conn, error) {
			return parsed.Dial(network, address)
		}, nil
	}
	return contextDialer.DialContext, nil
}

type proxyBufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *proxyBufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
