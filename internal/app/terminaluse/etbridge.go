package terminaluse

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/binaricat/netcatty/internal/platform/filesystem"
	"github.com/binaricat/netcatty/internal/terminal/mosh"
	terminalssh "github.com/binaricat/netcatty/internal/terminal/ssh"
	"golang.org/x/crypto/ssh"
)

func etUsesGoSSH(request MoshStartRequest) bool {
	return request.Password != "" || request.PrivateKey != "" || request.Passphrase != "" || request.Certificate != "" || request.UseAgent || request.EnableMFA || len(request.IdentityFilePaths) > 0 || request.ProxyURL != "" || request.ProxyCommand != "" || len(request.JumpHosts) > 0
}

// etBridge owns a fixed loopback tunnel and a one-use authenticated SSH reply.
// Native et still owns its wire protocol. Native SSH only sees an ephemeral
// local identity; remote credentials and the actual ET passkey never enter argv.
type etBridge struct {
	ctx            context.Context
	cancel         context.CancelFunc
	transport      *terminalssh.Transport
	transportMu    sync.Mutex
	dialMu         sync.Mutex
	redial         func(context.Context) (*terminalssh.Transport, error)
	tcp, bootstrap net.Listener
	mu             sync.Mutex
	connections    map[net.Conn]struct{}
	wg             sync.WaitGroup
	closeOnce      sync.Once
	args           []string
	env            map[string]string
	directory      string
	temp           *filesystem.TempService
	ready          chan struct{}
	stopTransport  func() bool
}

func (s *Service) prepareEt(ctx context.Context, request MoshStartRequest) (*etBridge, error) {
	if s.helperTemp == nil {
		return nil, errors.New("et: managed temp service unavailable")
	}
	config, err := sshBootstrapConfig(ctx, s, request)
	if err != nil {
		return nil, err
	}
	transport, err := terminalssh.Dial(ctx, config)
	if err != nil {
		return nil, err
	}
	b, err := newEtBridge(ctx, transport, request, s.helperTemp, func(ctx context.Context) (*terminalssh.Transport, error) {
		config, err := sshBootstrapConfig(ctx, s, request)
		if err != nil {
			return nil, err
		}
		return terminalssh.Dial(ctx, config)
	})
	if err != nil {
		transport.Close()
	}
	return b, err
}

func newEtBridge(ctx context.Context, transport *terminalssh.Transport, request MoshStartRequest, temp *filesystem.TempService, redial func(context.Context) (*terminalssh.Transport, error)) (_ *etBridge, err error) {
	ctx, cancel := context.WithCancel(ctx)
	b := &etBridge{ctx: ctx, cancel: cancel, transport: transport, redial: redial, temp: temp, connections: make(map[net.Conn]struct{}), ready: make(chan struct{})}
	b.stopTransport = context.AfterFunc(ctx, b.closeTransport)
	defer func() {
		if err != nil {
			b.Close()
		}
	}()
	b.directory, err = temp.CreateDir("et-bootstrap-*")
	if err != nil {
		return nil, err
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	identity, err := ssh.NewSignerFromKey(private)
	if err != nil {
		return nil, err
	}
	block, err := ssh.MarshalPrivateKey(private, "netcatty-et-loopback")
	if err != nil {
		return nil, err
	}
	keyPath := filepath.Join(b.directory, "identity")
	if err = os.WriteFile(keyPath, pem.EncodeToMemory(block), 0600); err != nil {
		return nil, err
	}
	if err = restrictPrivateFile(keyPath); err != nil {
		return nil, err
	}
	_, hostPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	hostSigner, err := ssh.NewSignerFromKey(hostPrivate)
	if err != nil {
		return nil, err
	}
	b.bootstrap, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	b.tcp, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	knownPath := filepath.Join(b.directory, "known_hosts")
	_, sshPort, _ := net.SplitHostPort(b.bootstrap.Addr().String())
	if err = os.WriteFile(knownPath, append([]byte("[127.0.0.1]:"+sshPort+" "), ssh.MarshalAuthorizedKey(hostSigner.PublicKey())...), 0600); err != nil {
		return nil, err
	}
	if err = restrictPrivateFile(knownPath); err != nil {
		return nil, err
	}
	_, etPort, _ := net.SplitHostPort(b.tcp.Addr().String())
	b.args = []string{"netcatty@127.0.0.1", "--port", etPort, "--terminal-path", "netcatty-et-bootstrap", "--silent", "--telemetry=false"}
	// ET and its SSH child run in directory. Relative artifact names avoid
	// nested quoting in ET's Windows subprocess parser (including spaces in HOME).
	options := []string{"Port=" + sshPort, "HostName=127.0.0.1", "User=netcatty", "IdentityFile=identity", "IdentitiesOnly=yes", "IdentityAgent=none", "UserKnownHostsFile=known_hosts", "GlobalKnownHostsFile=known_hosts", "StrictHostKeyChecking=yes", "BatchMode=yes", "ProxyCommand=none", "ProxyJump=none", "ForwardAgent=no", "ClearAllForwardings=yes", "LogLevel=ERROR"}
	for _, option := range options {
		b.args = append(b.args, "--ssh-option", option)
	}
	b.env = map[string]string{"TERM": "xterm-256color", "HOME": b.directory, "USERPROFILE": b.directory, "TMPDIR": b.directory, "TEMP": b.directory, "TMP": b.directory}
	serverConfig := &ssh.ServerConfig{PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if conn.User() != "netcatty" || !bytes.Equal(key.Marshal(), identity.PublicKey().Marshal()) {
			return nil, errors.New("invalid bootstrap identity")
		}
		return nil, nil
	}}
	serverConfig.AddHostKey(hostSigner)
	// Go runs etterminal with input on its SSH channel, never via echo argv.
	pair, err := bootstrapET(ctx, transport.Client, request)
	if err != nil {
		return nil, err
	}
	b.wg.Add(2)
	go b.serveBootstrap(serverConfig, pair)
	go b.serveTunnel(request.EtPort)
	return b, nil
}

func bootstrapET(ctx context.Context, client *ssh.Client, request MoshStartRequest) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = session.Close() })
	defer stop()
	input, err := mosh.ETBootstrapInput()
	if err != nil {
		return "", err
	}
	session.Stdin = strings.NewReader(input)
	stdout, err := session.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err = session.Start(mosh.ETServerCommand(request.ServerPath, request.ServerFifo)); err != nil {
		return "", err
	}
	return mosh.ReadETConnect(stdout)
}

func (b *etBridge) track(conn net.Conn) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ctx.Err() != nil {
		conn.Close()
		return false
	}
	b.connections[conn] = struct{}{}
	return true
}
func (b *etBridge) untrack(conn net.Conn) {
	conn.Close()
	b.mu.Lock()
	delete(b.connections, conn)
	b.mu.Unlock()
}

func (b *etBridge) serveBootstrap(config *ssh.ServerConfig, pair string) {
	defer b.wg.Done()
	// Only one authenticated exec can consume the bootstrap response.
	var consumed bool
	var consumeMu sync.Mutex
	for {
		conn, err := b.bootstrap.Accept()
		if err != nil {
			return
		}
		if !b.track(conn) {
			return
		}
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			defer b.untrack(conn)
			_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
			server, channels, requests, err := ssh.NewServerConn(conn, config)
			if err != nil {
				return
			}
			defer server.Close()
			go ssh.DiscardRequests(requests)
			for channel := range channels {
				if channel.ChannelType() != "session" {
					channel.Reject(ssh.UnknownChannelType, "bootstrap only")
					continue
				}
				stream, requests, err := channel.Accept()
				if err != nil {
					return
				}
				for request := range requests {
					if request.Type != "exec" {
						_ = request.Reply(false, nil)
						continue
					}
					// The command contains native ET's disposable credentials.
					// It is never executed, logged, or forwarded to the remote host.
					consumeMu.Lock()
					allowed := !consumed
					consumed = true
					consumeMu.Unlock()
					_ = request.Reply(allowed, nil)
					if allowed {
						_, err = io.WriteString(stream, "IDPASSKEY:"+pair+"\n")
						_, _ = stream.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
					}
					stream.Close()
					server.Close()
					if allowed && err == nil {
						close(b.ready)
					}
					return
				}
				stream.Close()
			}
		}()
	}
}

func (b *etBridge) serveTunnel(port uint16) {
	defer b.wg.Done()
	if port == 0 {
		port = 2022
	}
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port)))
	for {
		conn, err := b.tcp.Accept()
		if err != nil {
			return
		}
		if !b.track(conn) {
			return
		}
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			defer b.untrack(conn)
			dialCtx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
			defer cancel()
			remote, err := b.dialRemote(dialCtx, address)
			if err != nil {
				return
			}
			if !b.track(remote) {
				return
			}
			defer b.untrack(remote)
			done := make(chan struct{})
			go func() { _, _ = io.Copy(remote, conn); remote.Close(); conn.Close(); close(done) }()
			_, _ = io.Copy(conn, remote)
			conn.Close()
			remote.Close()
			<-done
		}()
	}
}

func (b *etBridge) closeTransport() {
	b.transportMu.Lock()
	transport := b.transport
	b.transportMu.Unlock()
	if transport != nil {
		transport.Close()
	}
}

func (b *etBridge) dialRemote(ctx context.Context, address string) (net.Conn, error) {
	b.dialMu.Lock()
	defer b.dialMu.Unlock()
	b.transportMu.Lock()
	transport := b.transport
	b.transportMu.Unlock()
	conn, err := transport.Client.DialContext(ctx, "tcp", address)
	if err == nil || b.redial == nil || ctx.Err() != nil {
		return conn, err
	}
	// Native et retains its replay state during network roaming. Rebuild only
	// the SSH carrier; do not run etterminal or change its id/passkey here.
	replacement, err := b.redial(ctx)
	if err != nil {
		return nil, err
	}
	b.transportMu.Lock()
	if b.ctx.Err() != nil {
		b.transportMu.Unlock()
		replacement.Close()
		return nil, b.ctx.Err()
	}
	b.transport = replacement
	b.transportMu.Unlock()
	transport.Close()
	return replacement.Client.DialContext(ctx, "tcp", address)
}

func (b *etBridge) Close() {
	b.closeOnce.Do(func() {
		if b.stopTransport != nil {
			b.stopTransport()
		}
		b.cancel()
		if b.tcp != nil {
			b.tcp.Close()
		}
		if b.bootstrap != nil {
			b.bootstrap.Close()
		}
		b.mu.Lock()
		for conn := range b.connections {
			conn.Close()
		}
		b.mu.Unlock()
		b.closeTransport()
		b.wg.Wait()
		if b.directory != "" {
			_ = b.temp.Remove(filepath.Base(b.directory))
		}
	})
}
