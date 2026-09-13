package ssh

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

func x11Peer(t *testing.T, accept bool) (*gossh.Client, *gossh.Session, <-chan *gossh.ServerConn, <-chan []byte) {
	t.Helper()
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := gossh.NewSignerFromKey(key)
	cfg := &gossh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	peers := make(chan *gossh.ServerConn, 1)
	requests := make(chan []byte, 1)
	go func() {
		c, e := ln.Accept()
		if e != nil {
			return
		}
		server, chans, reqs, e := gossh.NewServerConn(c, cfg)
		if e != nil {
			return
		}
		peers <- server
		go gossh.DiscardRequests(reqs)
		for incoming := range chans {
			ch, rs, e := incoming.Accept()
			if e != nil {
				return
			}
			go func() {
				defer ch.Close()
				for r := range rs {
					if r.Type == "x11-req" {
						requests <- r.Payload
						r.Reply(accept, nil)
					} else {
						r.Reply(true, nil)
					}
				}
			}()
		}
	}()
	client, err := gossh.Dial("tcp", ln.Addr().String(), &gossh.ClientConfig{User: "test", HostKeyCallback: gossh.InsecureIgnoreHostKey(), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return client, session, peers, requests
}

// This crosses real SSH and TCP boundaries: fake credentials are sent remotely,
// only validated clients reach the fixed display, and Close terminates relays.
func TestX11ForwardingPeer(t *testing.T) {
	client, session, peers, requests := x11Peer(t, true)
	server := <-peers
	defer server.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	real := bytes.Repeat([]byte{0x42}, 16)
	f, err := StartX11Forwarding(context.Background(), client, session, X11Display{Network: "tcp", Address: ln.Addr().String(), Screen: 2}, real)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var request struct {
		Single           bool
		Protocol, Cookie string
		Screen           uint32
	}
	if err := gossh.Unmarshal(<-requests, &request); err != nil {
		t.Fatal(err)
	}
	fake, err := hex.DecodeString(request.Cookie)
	if err != nil || len(fake) != 16 || bytes.Equal(fake, real) || request.Protocol != "MIT-MAGIC-COOKIE-1" || request.Screen != 2 || request.Single {
		t.Fatalf("invalid request %+v", request)
	}
	payload := gossh.Marshal(struct {
		Host string
		Port uint32
	}{"203.0.113.50", 1234})
	bad, rs, err := server.OpenChannel("x11", payload)
	if err != nil {
		t.Fatal(err)
	}
	go gossh.DiscardRequests(rs)
	tampered := x11Setup(binary.LittleEndian, real)
	bad.Write(tampered)
	done := make(chan struct{})
	go func() { io.Copy(io.Discard, bad); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("tampered channel not closed")
	}
	ln.(*net.TCPListener).SetDeadline(time.Now().Add(50 * time.Millisecond))
	if c, e := ln.Accept(); e == nil {
		c.Close()
		t.Fatal("invalid cookie reached local server")
	}
	ln.(*net.TCPListener).SetDeadline(time.Time{})
	ch, rs, err := server.OpenChannel("x11", payload)
	if err != nil {
		t.Fatal(err)
	}
	go gossh.DiscardRequests(rs)
	setup := x11Setup(binary.BigEndian, fake)
	go func() {
		for _, p := range setup {
			ch.Write([]byte{p})
		}
		ch.Write([]byte("PING"))
	}()
	ln.(*net.TCPListener).SetDeadline(time.Now().Add(2 * time.Second))
	local, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	local.SetDeadline(time.Now().Add(2 * time.Second))
	got := make([]byte, 52)
	if _, err := io.ReadFull(local, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got[32:48], real) || string(got[48:]) != "PING" {
		t.Fatalf("bad local bytes %x", got)
	}
	local.Write([]byte("PONG"))
	reply := make([]byte, 4)
	if _, err := io.ReadFull(ch, reply); err != nil || string(reply) != "PONG" {
		t.Fatalf("reply %s %v", reply, err)
	}
	f.Close()
	if _, err := local.Read(reply); err == nil {
		t.Fatal("local connection survived close")
	}
	if late, _, err := server.OpenChannel("x11", payload); err == nil {
		late.Close()
		t.Fatal("accepted channel after close")
	}
}

func TestX11RejectedRequest(t *testing.T) {
	client, session, _, _ := x11Peer(t, false)
	f, err := StartX11Forwarding(context.Background(), client, session, X11Display{Network: "tcp", Address: "127.0.0.1:6000"}, bytes.Repeat([]byte{1}, 16))
	if err == nil {
		f.Close()
		t.Fatal("reported success on denied x11-req")
	}
}
