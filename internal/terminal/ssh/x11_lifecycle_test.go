package ssh

import (
	"bytes"
	"context"
	gossh "golang.org/x/crypto/ssh"
	"io"
	"testing"
	"time"
)

// Resource exhaustion and cancellation must bound even unauthenticated clients.
func TestX11ChannelCapAndCancellation(t *testing.T) {
	client, session, peers, _ := x11Peer(t, true)
	server := <-peers
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f, err := StartX11Forwarding(ctx, client, session, X11Display{Network: "tcp", Address: "127.0.0.1:6000"}, bytes.Repeat([]byte{1}, 16))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	payload := gossh.Marshal(struct {
		Host string
		Port uint32
	}{"127.0.0.1", 123})
	var channels []gossh.Channel
	for i := 0; i < 16; i++ {
		ch, rs, err := server.OpenChannel("x11", payload)
		if err != nil {
			t.Fatal(err)
		}
		go gossh.DiscardRequests(rs)
		channels = append(channels, ch)
	}
	if ch, _, err := server.OpenChannel("x11", payload); err == nil {
		ch.Close()
		t.Fatal("unbounded channels accepted")
	}
	cancel()
	done := make(chan struct{})
	go func() {
		for _, ch := range channels {
			io.Copy(io.Discard, ch)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation did not close pending setups")
	}
}

func TestX11StalledSetupDeadline(t *testing.T) {
	client, session, peers, _ := x11Peer(t, true)
	server := <-peers
	defer server.Close()
	f, err := StartX11Forwarding(context.Background(), client, session, X11Display{Network: "tcp", Address: "127.0.0.1:6000"}, bytes.Repeat([]byte{1}, 16))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ch, rs, err := server.OpenChannel("x11", gossh.Marshal(struct {
		Host string
		Port uint32
	}{"127.0.0.1", 1}))
	if err != nil {
		t.Fatal(err)
	}
	go gossh.DiscardRequests(rs)
	done := make(chan struct{})
	go func() { io.Copy(io.Discard, ch); close(done) }()
	select {
	case <-done:
	case <-time.After(7 * time.Second):
		t.Fatal("stalled setup did not expire")
	}
}
