package telnet

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// scriptedServer simulates a telnet server: a negotiation script plus an echo
// and prompt phase, recording NAWS subnegotiations it receives.
type scriptedServer struct {
	mu       sync.Mutex
	listener net.Listener
	received []byte
	naws     [][]byte
	conn     net.Conn
}

func startScriptedServer(t *testing.T, script func(s *scriptedServer, w *bufio.Writer)) *scriptedServer {
	t.Helper()
	s := &scriptedServer{}
	var err error
	s.listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.listener.Close() })
	go func() {
		conn, acceptErr := s.listener.Accept()
		if acceptErr != nil {
			return
		}
		s.mu.Lock()
		s.conn = conn
		s.mu.Unlock()
		// Record inbound bytes for the whole connection lifetime, including
		// while the scripted phase is still running.
		go func() {
			reader := bufio.NewReader(conn)
			for {
				b, readErr := reader.ReadByte()
				if readErr != nil {
					return
				}
				s.mu.Lock()
				s.received = append(s.received, b)
				s.mu.Unlock()
			}
		}()
		w := bufio.NewWriter(conn)
		script(s, w)
		_ = w.Flush()
	}()
	return s
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal(message)
}

func TestNegotiationNAWSAndEchoMode(t *testing.T) {
	server := startScriptedServer(t, func(s *scriptedServer, w *bufio.Writer) {
		// Server will echo and asks for NAWS + SGA.
		_, _ = w.Write([]byte{IAC, WILL, OptEcho})
		_, _ = w.Write([]byte{IAC, DO, OptNAWS})
		_, _ = w.Write([]byte{IAC, DO, OptSGA})
		_ = w.Flush()
		time.Sleep(500 * time.Millisecond)
		_, _ = w.WriteString("Welcome!\r\n")
		_ = w.Flush()
	})

	var mu sync.Mutex
	var echoEvents int
	var dataText strings.Builder
	client, err := Connect(context.Background(), server.listener.Addr().String(), func(event Event) {
		if event.Kind == EventEchoMode {
			mu.Lock()
			echoEvents++
			mu.Unlock()
		}
		if event.Kind == EventData {
			mu.Lock()
			dataText.WriteString(string(event.Data))
			mu.Unlock()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	waitFor(t, 3*time.Second, func() bool { return client.RemoteEcho() }, "remote echo mode not set")
	waitFor(t, 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return echoEvents >= 1 && strings.Contains(dataText.String(), "Welcome!")
	}, "welcome data missing")

	// Client must have answered WILL NAWS with a NAWS subnegotiation carrying
	// the default 80x24 window.
	waitFor(t, 3*time.Second, func() bool {
		server.mu.Lock()
		defer server.mu.Unlock()
		received := server.received
		for i := 0; i+8 < len(received); i++ {
			if received[i] == IAC && received[i+1] == SB && received[i+2] == OptNAWS {
				width := uint16(received[i+3])<<8 | uint16(received[i+4])
				if width == 80 {
					return true
				}
			}
		}
		return false
	}, "NAWS subnegotiation with width 80 missing")

	// Resize must send a fresh NAWS with the new dimensions.
	if err := client.Resize(200, 50); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool {
		server.mu.Lock()
		defer server.mu.Unlock()
		received := server.received
		for i := 0; i+8 < len(received); i++ {
			if received[i] == IAC && received[i+1] == SB && received[i+2] == OptNAWS {
				width := uint16(received[i+3])<<8 | uint16(received[i+4])
				if width == 200 {
					return true
				}
			}
		}
		return false
	}, "NAWS resize (width 200) missing")
}

func TestSendEscapesIACAndServerSeesPlain(t *testing.T) {
	server := startScriptedServer(t, func(s *scriptedServer, w *bufio.Writer) {
		time.Sleep(200 * time.Millisecond)
	})
	client, err := Connect(context.Background(), server.listener.Addr().String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	payload := []byte("user\xffpass\r\n")
	if err := client.Send(payload); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool {
		server.mu.Lock()
		defer server.mu.Unlock()
		// The server must see the doubled IAC (ff ff), not a single 255.
		return len(server.received) >= len(payload)+1 && containsDoubledIAC(server.received)
	}, "IAC escaping missing on the wire")
}

func containsDoubledIAC(data []byte) bool {
	for i := 0; i+1 < len(data); i++ {
		if data[i] == IAC && data[i+1] == IAC {
			return true
		}
	}
	return false
}

func TestAutoLoginAnswersPrompts(t *testing.T) {
	prompts := make(chan string, 4)
	server := startScriptedServer(t, func(s *scriptedServer, w *bufio.Writer) {
		_, _ = w.WriteString("Welcome to router\r\nlogin: ")
		_ = w.Flush()
		time.Sleep(300 * time.Millisecond)
		_, _ = w.WriteString("Password: ")
		_ = w.Flush()
		time.Sleep(300 * time.Millisecond)
		_, _ = w.WriteString("router> ")
		_ = w.Flush()
	})

	client, err := Connect(context.Background(), server.listener.Addr().String(), func(event Event) {
		if event.Kind == EventData {
			prompts <- string(event.Data)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := AutoLogin(ctx, client, "admin", "secret123", 4*time.Second); err != nil {
		t.Fatalf("auto login: %v", err)
	}
	waitFor(t, 3*time.Second, func() bool {
		server.mu.Lock()
		defer server.mu.Unlock()
		return strings.Contains(string(server.received), "admin\r\nsecret123\r\n")
	}, "login answers missing on the wire")
}
