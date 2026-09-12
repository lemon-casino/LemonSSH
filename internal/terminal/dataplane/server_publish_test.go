package dataplane

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Output published before the renderer attaches must be buffered, not
// dropped: the first credit grant then admits it.
func TestPublishBeforeAttachBuffers(t *testing.T) {
	server, controller, _ := startTestServer(t)
	bootstrap, err := controller.Open("early")
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("pre-attach output")
	server.Publish("early", payload)

	connection := dialData(t, server, bootstrap)
	writeFrame(t, connection, Frame{Kind: FrameCredit, Generation: bootstrap.Generation, Sequence: 0, CreditCost: uint32(ReceiveWindowBytes)})
	frame := readFrame(t, connection)
	if frame.Kind != FrameOutput || !bytes.Equal(frame.Payload, payload) {
		t.Fatalf("buffered output not delivered: %+v", frame)
	}
}

// After a data WebSocket ends, a reconnecting renderer must get a fresh
// queue; reusing the closed queue would end the new connection instantly.
func TestReconnectGetsFreshQueue(t *testing.T) {
	server, controller, _ := startTestServer(t)
	bootstrap, err := controller.Open("reconn")
	if err != nil {
		t.Fatal(err)
	}
	first := dialData(t, server, bootstrap)
	writeFrame(t, first, Frame{Kind: FrameCredit, Generation: bootstrap.Generation, Sequence: 0, CreditCost: uint32(ReceiveWindowBytes)})
	server.Publish("reconn", []byte("one"))
	frame := readFrame(t, first)
	if string(frame.Payload) != "one" {
		t.Fatalf("first output mismatch: %+v", frame)
	}
	_ = first.Close(websocket.StatusNormalClosure, "")

	// Wait for the server-side write loop to close the old queue.
	deadline := time.Now().Add(5 * time.Second)
	for {
		server.mu.Lock()
		queue := server.outputs["reconn"]
		closed := queue != nil && queue.isClosed()
		server.mu.Unlock()
		if closed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("old queue was not closed after disconnect")
		}
		time.Sleep(10 * time.Millisecond)
	}

	server.Publish("reconn", []byte("two"))
	second := dialData(t, server, bootstrap)
	writeFrame(t, second, Frame{Kind: FrameCredit, Generation: bootstrap.Generation, Sequence: 1, CreditCost: uint32(ReceiveWindowBytes)})
	frame = readFrame(t, second)
	if string(frame.Payload) != "two" {
		t.Fatalf("post-reconnect output mismatch: %+v", frame)
	}
}

// DropOutput removes the queue so teardown does not leak buffers.
func TestDropOutput(t *testing.T) {
	server, _, _ := startTestServer(t)
	server.Publish("gone", []byte("x"))
	server.DropOutput("gone")
	server.mu.Lock()
	_, exists := server.outputs["gone"]
	server.mu.Unlock()
	if exists {
		t.Fatal("queue survived DropOutput")
	}
	// Dropping an unknown session is a no-op.
	server.DropOutput("never-existed")
}

func TestPublishBeforeCreditIsBounded(t *testing.T) {
	server := NewServer(NewRouteController(), "")
	if err := server.Publish("bounded", make([]byte, ReceiveWindowBytes)); err != nil {
		t.Fatal(err)
	}
	if err := server.Publish("bounded", []byte("overflow")); !errors.Is(err, ErrOutputBackpressure) {
		t.Fatalf("Publish must propagate backpressure: %v", err)
	}
	queue := server.queueFor("bounded")
	var size int
	for _, chunk := range queue.chunks {
		size += len(chunk)
	}
	if size > int(ReceiveWindowBytes) {
		t.Fatalf("buffered %d bytes without credit, limit %d", size, ReceiveWindowBytes)
	}
}

func TestPublishOwnsBufferedBytes(t *testing.T) {
	server := NewServer(NewRouteController(), "")
	data := []byte("original")
	server.Publish("copy", data)
	copy(data, "modified")
	chunk, _ := server.queueFor("copy").pop()
	if string(chunk) != "original" {
		t.Fatalf("producer reuse corrupted buffered output: %q", chunk)
	}
}

func TestOutputAdmissionErrorsAndRelease(t *testing.T) {
	queue := newOutputQueue()
	if err := queue.push(make([]byte, ReceiveWindowBytes-1)); err != nil {
		t.Fatal(err)
	}
	if err := queue.push([]byte("ab")); !errors.Is(err, ErrOutputBackpressure) {
		t.Fatalf("overflow must reject atomically: %v", err)
	}
	pending, _ := queue.pop()
	if err := queue.push([]byte("ab")); !errors.Is(err, ErrOutputBackpressure) {
		t.Fatalf("pending writer bytes must still consume capacity: %v", err)
	}
	queue.release(len(pending))
	if err := queue.push([]byte("ab")); err != nil {
		t.Fatalf("sent bytes must release capacity: %v", err)
	}
	queue.close()
	if err := queue.push([]byte("late")); !errors.Is(err, ErrOutputClosed) {
		t.Fatalf("dying queue must explicitly reject publish: %v", err)
	}
	if chunk, ok := queue.pop(); ok || chunk != nil {
		t.Fatal("closed queue retained abandoned output")
	}
}

func TestConcurrentOutputAdmissionAndClose(t *testing.T) {
	queue := newOutputQueue()
	var workers sync.WaitGroup
	for i := 0; i < 32; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 64; j++ {
				err := queue.push(make([]byte, 1024))
				if err != nil && !errors.Is(err, ErrOutputClosed) && !errors.Is(err, ErrOutputBackpressure) {
					t.Errorf("unexpected admission error: %v", err)
				}
			}
		}()
	}
	queue.close()
	workers.Wait()
	if err := queue.push([]byte("late")); !errors.Is(err, ErrOutputClosed) {
		t.Fatalf("closed queue accepted bytes: %v", err)
	}
}

func TestOriginAllowed(t *testing.T) {
	server := NewServer(NewRouteController(), "127.0.0.1:0")
	cases := []struct {
		origin string
		want   bool
	}{
		{"http://wails.localhost:34115", true},
		{"http://wails.localhost", true},
		{"wails://wails.localhost", true},
		{"https://wails.localhost", true},
		{"http://localhost:5173", false},
		{"http://evil.example", false},
		{"http://wails.localhost.evil.com", false},
		{"file://wails.localhost", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := server.originAllowed(tc.origin); got != tc.want {
			t.Errorf("originAllowed(%q) = %v, want %v", tc.origin, got, tc.want)
		}
	}
}
