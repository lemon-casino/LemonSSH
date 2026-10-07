// Package telnet owns the Telnet protocol (P3-08.1, TERM-03.1): TCP transport
// with IAC escaping, WILL/WONT/DO/DONT option negotiation, NAWS window size,
// echo-mode tracking and simple auto-login prompting.
package telnet

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"
)

// Telnet protocol bytes.
const (
	IAC  byte = 255
	DONT byte = 254
	DO   byte = 253
	WONT byte = 252
	WILL byte = 251
	SB   byte = 250
	SE   byte = 240

	OptEcho byte = 1
	OptSGA  byte = 3
	OptNAWS byte = 31
)

var (
	ErrNotConnected  = errors.New("telnet not connected")
	ErrClosed        = errors.New("telnet connection closed")
	ErrPromptTimeout = errors.New("telnet auto-login prompt timeout")
)

// EventKind classifies client-visible events.
type EventKind string

const (
	EventData          EventKind = "data"
	EventEchoMode      EventKind = "echoMode" // payload: "remote" or "local"
	EventConnected     EventKind = "connected"
	EventClosed        EventKind = "closed"
	EventAutoLoginDone EventKind = "autoLoginComplete"
	EventAutoLoginFail EventKind = "autoLoginCancelled"
)

// Event is delivered to the handler on the reader goroutine.
type Event struct {
	Kind EventKind
	Data []byte
}

// Client is one telnet session.
type Client struct {
	mu      sync.Mutex
	writeMu sync.Mutex

	conn       net.Conn
	handler    func(Event)
	width      uint16
	height     uint16
	remoteEcho bool // server echoes; client must not local-echo
	closed     bool
}

// Connect dials and starts negotiation + the read loop.
func Connect(ctx context.Context, addr string, handler func(Event)) (*Client, error) {
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	return Start(conn, handler), nil
}

// Start wraps an existing connection.
func Start(conn net.Conn, handler func(Event)) *Client {
	client := &Client{conn: conn, handler: handler, width: 80, height: 24}
	if handler != nil {
		handler(Event{Kind: EventConnected})
	}
	go client.readLoop()
	return client
}

// Send writes user input, escaping IAC bytes.
func (c *Client) Send(data []byte) error {
	c.mu.Lock()
	conn := c.conn
	closed := c.closed
	c.mu.Unlock()
	if closed || conn == nil {
		return ErrNotConnected
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err := conn.Write(escapeIAC(data))
	return err
}

// SendLine writes one line terminated by CRLF (telnet convention).
func (c *Client) SendLine(line string) error {
	return c.Send([]byte(line + "\r\n"))
}

// Resize sends NAWS with the new window size.
func (c *Client) Resize(width, height uint16) error {
	c.mu.Lock()
	c.width, c.height = width, height
	conn := c.conn
	closed := c.closed
	c.mu.Unlock()
	if closed || conn == nil {
		return ErrNotConnected
	}
	payload := nawsPayload(width, height)
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err := conn.Write([]byte{IAC, SB, OptNAWS, payload[0], payload[1], payload[2], payload[3], IAC, SE})
	return err
}

// RemoteEcho reports whether the server is responsible for echoing input.
func (c *Client) RemoteEcho() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.remoteEcho
}

// Close terminates the connection.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()
	if conn == nil {
		return nil
	}
	if c.handler != nil {
		c.handler(Event{Kind: EventClosed})
	}
	return conn.Close()
}

// currentHandler snapshots the event handler under the mutex; the handler may
// be swapped concurrently (e.g. AutoLogin chaining).
func (c *Client) currentHandler() func(Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.handler
}

// readLoop parses the inbound stream: plain data is dispatched; IAC sequences
// drive negotiation and are never forwarded as data. Buffered data is flushed
// after a short idle deadline so newline-less payloads (prompts, banners)
// reach the handler promptly.
func (c *Client) readLoop() {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return
	}
	reader := bufio.NewReader(conn)
	var dataBuffer []byte
	flush := func() {
		if len(dataBuffer) > 0 {
			if handler := c.currentHandler(); handler != nil {
				handler(Event{Kind: EventData, Data: append([]byte(nil), dataBuffer...)})
			}
			dataBuffer = dataBuffer[:0]
		}
	}
	for {
		// Idle deadline: only armed while data is buffered awaiting flush.
		if len(dataBuffer) > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		} else {
			_ = conn.SetReadDeadline(time.Time{})
		}
		b, err := reader.ReadByte()
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() && len(dataBuffer) > 0 {
				flush()
				_ = conn.SetReadDeadline(time.Time{})
				continue
			}
			flush()
			c.Close()
			return
		}
		if b != IAC {
			dataBuffer = append(dataBuffer, b)
			continue
		}
		next, err := reader.ReadByte()
		if err != nil {
			flush()
			c.Close()
			return
		}
		switch next {
		case IAC: // escaped 255 in data
			dataBuffer = append(dataBuffer, IAC)
		case DO, DONT, WILL, WONT:
			option, optErr := reader.ReadByte()
			if optErr != nil {
				flush()
				c.Close()
				return
			}
			c.handleNegotiation(next, option)
		case SB:
			if err := c.consumeSubnegotiation(reader); err != nil {
				flush()
				c.Close()
				return
			}
		default:
			// other commands (NOP, GO AHEAD, ...) are consumed
		}
	}
}

func (c *Client) handleNegotiation(command, option byte) {
	var reply []byte
	switch command {
	case DO:
		switch option {
		case OptNAWS:
			reply = []byte{IAC, WILL, OptNAWS}
			go func() { _ = c.Resize(c.currentWidth(), c.currentHeight()) }()
		case OptSGA:
			reply = []byte{IAC, WILL, OptSGA}
		case OptEcho:
			// The client never echoes; the server should.
			reply = []byte{IAC, WONT, OptEcho}
		default:
			reply = []byte{IAC, WONT, option}
		}
	case WILL:
		switch option {
		case OptEcho:
			c.mu.Lock()
			changed := !c.remoteEcho
			c.remoteEcho = true
			handler := c.handler
			c.mu.Unlock()
			reply = []byte{IAC, DO, OptEcho}
			if changed && handler != nil {
				handler(Event{Kind: EventEchoMode, Data: []byte("remote")})
			}
		case OptSGA:
			reply = []byte{IAC, DO, OptSGA}
		default:
			reply = []byte{IAC, DONT, option}
		}
	}
	if reply != nil {
		c.writeMu.Lock()
		defer c.writeMu.Unlock()
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn != nil {
			_, _ = conn.Write(reply)
		}
	}
}

// consumeSubnegotiation reads SB payload through SE, dispatching known
// options (NAWS from the server is unusual but tolerated).
func (c *Client) consumeSubnegotiation(reader *bufio.Reader) error {
	option, err := reader.ReadByte()
	if err != nil {
		return err
	}
	var payload []byte
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return err
		}
		if b == IAC {
			terminator, err := reader.ReadByte()
			if err != nil {
				return err
			}
			if terminator == SE {
				break
			}
			payload = append(payload, b, terminator)
			continue
		}
		payload = append(payload, b)
	}
	_ = option
	return nil
}

func (c *Client) currentWidth() uint16 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.width
}

func (c *Client) currentHeight() uint16 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.height
}

// nawsPayload encodes width/height with IAC escaping of 255 bytes.
func nawsPayload(width, height uint16) []byte {
	encode := func(v uint16) []byte {
		hi := byte(v >> 8)
		lo := byte(v & 0xFF)
		out := []byte{hi, lo}
		if hi == IAC {
			out = []byte{IAC, IAC, lo}
		}
		if lo == IAC {
			out = append(out, IAC)
		}
		return out
	}
	payload := encode(width)
	return append(payload, encode(height)...)
}

// escapeIAC doubles every IAC byte in user data.
func escapeIAC(data []byte) []byte {
	if !containsIAC(data) {
		return data
	}
	out := make([]byte, 0, len(data)+4)
	for _, b := range data {
		if b == IAC {
			out = append(out, IAC)
		}
		out = append(out, b)
	}
	return out
}

func containsIAC(data []byte) bool {
	for _, b := range data {
		if b == IAC {
			return true
		}
	}
	return false
}

// AutoLogin watches the stream for login/password prompts and answers them.
// It returns once connected data starts flowing after the password step, after
// emitting a completion event. A timeout or abort emits the cancellation event
// so the renderer can restore the manual prompt state.
func AutoLogin(ctx context.Context, client *Client, username, password string, timeout time.Duration) error {
	deadline := time.After(timeout)
	promptedUser, promptedPass := false, false
	events := make(chan Event, 32)
	previous := client.handler
	client.mu.Lock()
	client.handler = func(event Event) {
		select {
		case events <- event:
		default:
		}
		if previous != nil {
			previous(event)
		}
	}
	client.mu.Unlock()
	defer func() {
		client.mu.Lock()
		client.handler = previous
		client.mu.Unlock()
	}()
	notify := func(kind EventKind) {
		if previous != nil {
			previous(Event{Kind: kind})
		}
	}
	for {
		select {
		case <-ctx.Done():
			notify(EventAutoLoginFail)
			return ctx.Err()
		case <-deadline:
			notify(EventAutoLoginFail)
			return ErrPromptTimeout
		case event := <-events:
			if event.Kind != EventData {
				continue
			}
			text := strings.ToLower(string(event.Data))
			// The login prompt must be tested before the username prompt:
			// "login:" also contains the "login" that a naive check would
			// otherwise match twice, and "password:" must win over "username:".
			switch {
			case !promptedPass && strings.Contains(text, "password:"):
				promptedPass = true
				_ = client.SendLine(password)
			case !promptedUser && (strings.Contains(text, "login:") || strings.Contains(text, "username:")):
				promptedUser = true
				_ = client.SendLine(username)
			}
			if promptedUser && promptedPass && len(strings.TrimSpace(text)) > 0 && !strings.Contains(text, "password:") {
				notify(EventAutoLoginDone)
				return nil
			}
		}
	}
}
