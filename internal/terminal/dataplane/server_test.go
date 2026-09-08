package dataplane

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func startTestServer(t *testing.T) (*Server, *RouteController, string) {
	t.Helper()
	controller := NewRouteController()
	server := NewServer(controller, "127.0.0.1:0", WithOrigins("http://localhost:5173"))
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Stop() })
	return server, controller, server.Addr()
}

func dialData(t *testing.T, server *Server, bootstrap RouteBootstrap) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws://" + server.Addr() + "/v1/data/" + bootstrap.SessionID + "?generation=" + itoa(int(bootstrap.Generation))
	connection, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader:   map[string][]string{"Origin": {"http://localhost:5173"}},
		Subprotocols: []string{"netcatty-terminal-v1", "route." + bootstrap.DataToken},
	})
	if err != nil {
		t.Fatal(err)
	}
	connection.SetReadLimit(MaxFrameBytes)
	t.Cleanup(func() { _ = connection.Close(websocket.StatusNormalClosure, "") })
	return connection
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

func readFrame(t *testing.T, connection *websocket.Conn) Frame {
	t.Helper()
	// coder/websocket treats context cancellation as connection-fatal, so
	// tests read with a non-cancelling context.
	_, data, err := connection.Read(context.Background())
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	frame, err := UnmarshalFrame(data)
	if err != nil {
		t.Fatalf("parse frame: %v", err)
	}
	return frame
}

func writeFrame(t *testing.T, connection *websocket.Conn, frame Frame) {
	t.Helper()
	encoded, err := frame.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.Write(context.Background(), websocket.MessageBinary, encoded); err != nil {
		t.Fatal(err)
	}
}

func TestServerAuthRejects(t *testing.T) {
	server, controller, address := startTestServer(t)
	bootstrap, err := controller.Open("s1")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Bad origin.
	_, response, err := websocket.Dial(ctx, "ws://"+address+"/v1/data/s1?generation=1", &websocket.DialOptions{
		HTTPHeader:   map[string][]string{"Origin": {"http://evil.example"}},
		Subprotocols: []string{"netcatty-terminal-v1", "route." + bootstrap.DataToken},
	})
	if err == nil {
		t.Fatal("bad origin accepted")
	}
	if response == nil || response.StatusCode != 403 {
		t.Fatalf("expected 403 for bad origin, got %v", response)
	}

	// Bad token.
	_, response, err = websocket.Dial(ctx, "ws://"+address+"/v1/data/s1?generation=1", &websocket.DialOptions{
		HTTPHeader:   map[string][]string{"Origin": {"http://localhost:5173"}},
		Subprotocols: []string{"netcatty-terminal-v1", "route.0000000000000000000000000000000000000000000000000000000000000000"},
	})
	if err == nil {
		t.Fatal("bad token accepted")
	}
	if response == nil || response.StatusCode != 401 {
		t.Fatalf("expected 401 for bad token, got %v", response)
	}
	_ = server
}

func TestServerCreditFlowAndOutputDelivery(t *testing.T) {
	server, controller, _ := startTestServer(t)
	bootstrap, err := controller.Open("s2")
	if err != nil {
		t.Fatal(err)
	}
	connection := dialData(t, server, bootstrap)

	// Initial credit opens the window.
	writeFrame(t, connection, Frame{Kind: FrameCredit, Generation: bootstrap.Generation, Sequence: 0, CreditCost: uint32(ReceiveWindowBytes)})

	payload := []byte("hello terminal")
	server.Publish("s2", payload)
	frame := readFrame(t, connection)
	if frame.Kind != FrameOutput || !bytes.Equal(frame.Payload, payload) || frame.Sequence != 1 {
		t.Fatalf("unexpected output frame: %+v", frame)
	}

	// Output beyond credit must not be admitted until more credit arrives.
	server.Publish("s2", bytes.Repeat([]byte("x"), int(ReceiveWindowBytes)))
	time.Sleep(200 * time.Millisecond)
	_, _, sentThrough, _, err := controller.Snapshot("s2")
	if err != nil {
		t.Fatal(err)
	}
	// The full 1 MiB publish is 9 chunks (first 14-byte output + 8 x 128 KiB);
	// without new credit the whole payload can never be admitted.
	if sentThrough >= 10 {
		t.Fatalf("output admitted beyond credit window: sentThrough=%d", sentThrough)
	}

	// Grant credit for the applied output; the queued chunk is then sent.
	writeFrame(t, connection, Frame{Kind: FrameCredit, Generation: bootstrap.Generation, Sequence: 1, CreditCost: uint32(ReceiveWindowBytes)})
	received := 0
	for received < int(ReceiveWindowBytes) {
		frame = readFrame(t, connection)
		if frame.Kind != FrameOutput {
			t.Fatalf("expected output frames, got %+v", frame)
		}
		received += len(frame.Payload)
	}
	if received != int(ReceiveWindowBytes) {
		t.Fatalf("chunked output size mismatch: %d", received)
	}
}

func TestServerUrgentAck(t *testing.T) {
	server, controller, _ := startTestServer(t)
	ackReceived := make(chan []byte, 1)
	server.SetUrgentHandler(func(sessionID string, payload []byte) []byte {
		ackReceived <- payload
		return []byte("ack")
	})
	bootstrap, err := controller.Open("s3")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws://" + server.Addr() + "/v1/urgent/s3?generation=" + itoa(int(bootstrap.Generation))
	connection, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader:   map[string][]string{"Origin": {"http://localhost:5173"}},
		Subprotocols: []string{"netcatty-terminal-v1", "route." + bootstrap.UrgentToken},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(websocket.StatusNormalClosure, "")

	writeFrame(t, connection, Frame{Kind: FrameUrgent, Generation: bootstrap.Generation, Correlation: 7, Payload: []byte{3}})
	select {
	case payload := <-ackReceived:
		if !bytes.Equal(payload, []byte{3}) {
			t.Fatalf("handler payload mismatch: %v", payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("urgent handler not invoked")
	}
	frame := readFrame(t, connection)
	if frame.Kind != FrameUrgentACK || frame.Correlation != 7 || string(frame.Payload) != "ack" {
		t.Fatalf("unexpected urgent ack: %+v", frame)
	}
}

func TestServerRebindInvalidatesOldGeneration(t *testing.T) {
	server, controller, _ := startTestServer(t)
	first, err := controller.Open("s4")
	if err != nil {
		t.Fatal(err)
	}
	oldConnection := dialData(t, server, first)
	// Rebind: generation advances, old tokens are stale.
	second, err := controller.Open("s4")
	if err != nil {
		t.Fatal(err)
	}
	if second.Generation != first.Generation+1 {
		t.Fatal("generation must advance")
	}
	// Old connection's credit frame is now stale; the server closes it.
	writeFrame(t, oldConnection, Frame{Kind: FrameCredit, Generation: first.Generation, Sequence: 0, CreditCost: uint32(ReceiveWindowBytes)})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, _, err := oldConnection.Read(ctx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("stale connection was not closed")
			}
			return // closed as expected
		}
	}
}
