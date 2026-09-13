package ssh

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// Each jump sees an otherwise unresolvable next-hop name. A direct dial can
// never accidentally pass this test on the developer's network.
func sshDialFixture(t *testing.T, routes map[string]string, authConfig ...*gossh.ServerConfig) (string, HostKeyPolicy, <-chan string) {
	t.Helper()
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := gossh.NewSignerFromKey(key)
	config := &gossh.ServerConfig{PasswordCallback: func(_ gossh.ConnMetadata, p []byte) (*gossh.Permissions, error) {
		if string(p) != "password" {
			return nil, errors.New("denied")
		}
		return nil, nil
	}}
	if len(authConfig) > 0 {
		config = authConfig[0]
	}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	requests := make(chan string, 10)
	var mu sync.Mutex
	connections := make(map[net.Conn]bool)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			connections[conn] = true
			mu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()
				server, channels, reqs, err := gossh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				defer server.Close()
				go gossh.DiscardRequests(reqs)
				for c := range channels {
					if c.ChannelType() != "direct-tcpip" {
						c.Reject(gossh.UnknownChannelType, "forward only")
						continue
					}
					var payload struct {
						Host       string
						Port       uint32
						Origin     string
						OriginPort uint32
					}
					if gossh.Unmarshal(c.ExtraData(), &payload) != nil {
						c.Reject(gossh.ConnectionFailed, "invalid")
						continue
					}
					address := net.JoinHostPort(payload.Host, strconv.Itoa(int(payload.Port)))
					requests <- address
					if address == "echo:7" {
						ch, rs, e := c.Accept()
						if e != nil {
							continue
						}
						go gossh.DiscardRequests(rs)
						wg.Add(1)
						go func() { defer wg.Done(); defer ch.Close(); io.Copy(ch, ch) }()
						continue
					}
					mapped := routes[address]
					if mapped == "" {
						c.Reject(gossh.ConnectionFailed, "unexpected destination")
						continue
					}
					remote, e := net.Dial("tcp", mapped)
					if e != nil {
						c.Reject(gossh.ConnectionFailed, "dial failed")
						continue
					}
					ch, rs, e := c.Accept()
					if e != nil {
						remote.Close()
						continue
					}
					go gossh.DiscardRequests(rs)
					wg.Add(1)
					go func() {
						defer wg.Done()
						defer remote.Close()
						defer ch.Close()
						done := make(chan struct{})
						go func() { io.Copy(remote, ch); remote.Close(); close(done) }()
						io.Copy(ch, remote)
						ch.Close()
						<-done
					}()
				}
			}()
		}
	}()
	t.Cleanup(func() {
		listener.Close()
		mu.Lock()
		for conn := range connections {
			conn.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	return listener.Addr().String(), func(_ string, _ net.Addr, key gossh.PublicKey) error {
		if string(key.Marshal()) != string(signer.PublicKey().Marshal()) {
			return errors.New("host key mismatch")
		}
		return nil
	}, requests
}

func configAt(address string, policy HostKeyPolicy) DialConfig {
	host, port, _ := net.SplitHostPort(address)
	number, _ := strconv.Atoi(port)
	return DialConfig{Hostname: host, Port: uint16(number), Username: "user", Auth: AuthMethod{Password: "password"}, HostKeyPolicy: policy, Timeout: time.Second, HandshakeTimeout: time.Second}
}

func TestDialUsesEveryJumpAsUpstream(t *testing.T) {
	target, targetPolicy, _ := sshDialFixture(t, nil)
	second, secondPolicy, secondRequests := sshDialFixture(t, map[string]string{"target.invalid:22": target})
	first, firstPolicy, firstRequests := sshDialFixture(t, map[string]string{"second.invalid:22": second})
	config := configAt("target.invalid:22", targetPolicy)
	hop := configAt("second.invalid:22", secondPolicy)
	hop.JumpHosts = []DialConfig{configAt(first, firstPolicy)}
	config.JumpHosts = []DialConfig{hop}
	transport, err := Dial(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	if len(transport.intermediates) != 2 {
		t.Fatal("missing jump ownership")
	}
	if <-firstRequests != "second.invalid:22" || <-secondRequests != "target.invalid:22" {
		t.Fatal("wrong chain")
	}
	stream, err := transport.Client.Dial("tcp", "echo:7")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	stream.Write([]byte("ok"))
	buf := make([]byte, 2)
	if _, err = io.ReadFull(stream, buf); err != nil || string(buf) != "ok" {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); transport.Close() }()
	}
	wg.Wait()
}

func TestDialCancellationInterruptsSSHHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() { conn, _ := listener.Accept(); accepted <- conn }()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := Dial(ctx, configAt(listener.Addr().String(), func(string, net.Addr, gossh.PublicKey) error { return nil }))
		result <- err
	}()
	conn := <-accepted
	defer conn.Close()
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("canceled handshake succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("handshake ignored cancellation")
	}
}

func TestDialProxyPrecedesJumpChain(t *testing.T) {
	target, targetPolicy, _ := sshDialFixture(t, nil)
	first, firstPolicy, _ := sshDialFixture(t, map[string]string{"target.invalid:22": target})
	helper := buildProxyHelper(t)
	command := fmt.Sprintf("\"%s\" %s", helper, first)
	for _, override := range []bool{false, true} {
		config := configAt("target.invalid:22", targetPolicy)
		config.JumpHosts = []DialConfig{configAt("jump.invalid:22", firstPolicy)}
		config.ProxyCommand = command
		if override {
			config.ProxyCommand = "must-not-be-executed"
			config.JumpHosts[0].ProxyCommand = command
		}
		transport, err := Dial(context.Background(), config)
		if err != nil {
			t.Fatal(err)
		}
		transport.Close()
	}
}

func TestDialProxyAuthAndStrictHostKey(t *testing.T) {
	address, policy, _ := sshDialFixture(t, nil)
	for _, kind := range []string{"http", "socks5", "command"} {
		t.Run(kind, func(t *testing.T) {
			config := configAt("target.invalid:22", policy)
			if kind == "command" {
				helper := buildProxyHelper(t)
				config.ProxyCommand = fmt.Sprintf("\"%s\" %s", helper, address)
			} else {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				done := make(chan error, 1)
				go func() {
					conn, err := listener.Accept()
					if err != nil {
						done <- err
						return
					}
					defer conn.Close()
					reader := bufio.NewReader(conn)
					if kind == "http" {
						req, e := http.ReadRequest(reader)
						if e != nil {
							done <- e
							return
						}
						if req.Method != "CONNECT" || req.Host != "target.invalid:22" || req.Header.Get("Proxy-Authorization") != "Basic dXNlcjpwYXNz" {
							done <- errors.New("bad HTTP proxy request")
							return
						}
						io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
					} else {
						header := make([]byte, 2)
						io.ReadFull(reader, header)
						methods := make([]byte, int(header[1]))
						io.ReadFull(reader, methods)
						conn.Write([]byte{5, 2})
						io.ReadFull(reader, header)
						user := make([]byte, int(header[1]))
						io.ReadFull(reader, user)
						length, _ := reader.ReadByte()
						password := make([]byte, int(length))
						io.ReadFull(reader, password)
						if string(user) != "user" || string(password) != "pass" {
							done <- errors.New("bad SOCKS credentials")
							return
						}
						conn.Write([]byte{1, 0})
						request := make([]byte, 5)
						io.ReadFull(reader, request)
						host := make([]byte, int(request[4]))
						io.ReadFull(reader, host)
						port := make([]byte, 2)
						io.ReadFull(reader, port)
						if string(host) != "target.invalid" {
							done <- errors.New("bad SOCKS target")
							return
						}
						conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 22})
					}
					remote, e := net.Dial("tcp", address)
					if e != nil {
						done <- e
						return
					}
					defer remote.Close()
					copied := make(chan struct{})
					go func() { io.Copy(remote, reader); remote.Close(); close(copied) }()
					io.Copy(conn, remote)
					conn.Close()
					<-copied
					done <- nil
				}()
				config.ProxyURL = kind + "://user:pass@" + listener.Addr().String()
				t.Cleanup(func() {
					select {
					case err := <-done:
						if err != nil {
							t.Error(err)
						}
					case <-time.After(3 * time.Second):
						t.Error("proxy leaked connection")
					}
				})
			}
			transport, err := Dial(context.Background(), config)
			if err != nil {
				t.Fatal(err)
			}
			transport.Close()
		})
	}
	config := configAt(address, StrictPolicy(NewKnownHosts(filepath.Join(t.TempDir(), "known_hosts"))))
	transport, err := Dial(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	transport.Close()
	config.HostKeyPolicy = func(string, net.Addr, gossh.PublicKey) error { return errors.New("host key mismatch") }
	if _, err = Dial(context.Background(), config); err == nil || !strings.Contains(err.Error(), "host key mismatch") {
		t.Fatal("strict policy bypassed", err)
	}
}
