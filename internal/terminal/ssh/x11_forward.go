package ssh

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// X11Forwarder belongs to one dedicated terminal SSH client. Close cancels
// active relays; the registered handler rejects further opens until client exit.
type X11Forwarder struct {
	cancel context.CancelFunc
	once   sync.Once
}

func (f *X11Forwarder) Close() {
	if f != nil {
		f.once.Do(f.cancel)
	}
}

func StartX11Forwarding(parent context.Context, client *gossh.Client, session *gossh.Session, display X11Display, realCookie []byte) (*X11Forwarder, error) {
	if len(realCookie) != 16 {
		return nil, errors.New("X11: local MIT cookie required")
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	fake := make([]byte, 16)
	if _, err := rand.Read(fake); err != nil {
		return nil, err
	}
	real := append([]byte(nil), realCookie...)
	incoming := client.HandleChannelOpen("x11")
	if incoming == nil {
		return nil, errors.New("X11: handler already registered")
	}
	ctx, cancel := context.WithCancel(parent)
	f := &X11Forwarder{cancel: cancel}
	slots := make(chan struct{}, 16)
	go func() {
		defer cancel()
		for ch := range incoming {
			if ctx.Err() != nil {
				_ = ch.Reject(gossh.Prohibited, "X11 forwarding closed")
				continue
			}
			var origin struct {
				Host string
				Port uint32
			}
			if err := gossh.Unmarshal(ch.ExtraData(), &origin); err != nil || origin.Port > 65535 {
				_ = ch.Reject(gossh.Prohibited, "invalid X11 origin")
				continue
			}
			// Origin metadata is never used as a destination.
			select {
			case slots <- struct{}{}:
				go func(ch gossh.NewChannel) { defer func() { <-slots }(); relayX11(ctx, ch, display, fake, real) }(ch)
			default:
				_ = ch.Reject(gossh.ResourceShortage, "X11 channel limit")
			}
		}
	}()
	result := make(chan error, 1)
	go func() {
		ok, err := session.SendRequest("x11-req", true, gossh.Marshal(struct {
			Single           bool
			Protocol, Cookie string
			Screen           uint32
		}{false, x11AuthProtocol, hex.EncodeToString(fake), display.Screen}))
		if err == nil && !ok {
			err = errors.New("server denied x11-req")
		}
		result <- err
	}()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case err := <-result:
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("X11: %w", err)
		}
		return f, nil
	case <-ctx.Done():
		f.Close()
		_ = session.Close()
		return nil, ctx.Err()
	case <-timer.C:
		f.Close()
		_ = session.Close()
		return nil, errors.New("X11: request timed out")
	}
}

func relayX11(ctx context.Context, incoming gossh.NewChannel, display X11Display, fake, real []byte) {
	ch, requests, err := incoming.Accept()
	if err != nil {
		return
	}
	defer ch.Close()
	go gossh.DiscardRequests(requests)
	stop := context.AfterFunc(ctx, func() { ch.Close() })
	defer stop()
	// SSH channels have no read deadlines. Closing the channel bounds a stalled
	// setup; it never opens the local socket until authentication has succeeded.
	timer := time.AfterFunc(5*time.Second, func() { ch.Close() })
	setup, err := readX11Setup(ch, fake, real)
	if !timer.Stop() || err != nil || ctx.Err() != nil {
		return
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	local, err := dialer.DialContext(ctx, display.Network, display.Address)
	if err != nil {
		return
	}
	defer local.Close()
	stopLocal := context.AfterFunc(ctx, func() { local.Close() })
	defer stopLocal()
	local.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := local.Write(setup); err != nil {
		return
	}
	local.SetWriteDeadline(time.Time{})
	// Bound stalled streams without imposing a lifetime limit on active clients.
	conn := x11IdleConn{Conn: local}
	done := make(chan struct{})
	go func() { io.Copy(ch, conn); ch.Close(); local.Close(); close(done) }()
	io.Copy(conn, ch)
	local.Close()
	ch.Close()
	<-done
}

type x11IdleConn struct{ net.Conn }

func (c x11IdleConn) Read(p []byte) (int, error) {
	c.SetReadDeadline(time.Now().Add(30 * time.Minute))
	return c.Conn.Read(p)
}
func (c x11IdleConn) Write(p []byte) (int, error) {
	c.SetWriteDeadline(time.Now().Add(30 * time.Second))
	return c.Conn.Write(p)
}
