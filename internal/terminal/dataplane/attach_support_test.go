package dataplane

import "testing"

func TestPendingBytesReportsQueuedOutput(t *testing.T) {
	controller := NewRouteController()
	server := NewServer(controller, "127.0.0.1:0")
	if got := server.PendingBytes("ghost"); got != 0 {
		t.Fatalf("unknown session must report 0 pending bytes, got %d", got)
	}
	const payload = "queued-bytes"
	if err := server.Publish("s1", []byte(payload)); err != nil {
		t.Fatal(err)
	}
	if got := server.PendingBytes("s1"); got != len(payload) {
		t.Fatalf("pending bytes mismatch: got %d want %d", got, len(payload))
	}
	server.DropOutput("s1")
	if got := server.PendingBytes("s1"); got != 0 {
		t.Fatalf("dropped queue must report 0 pending bytes, got %d", got)
	}
}

func TestKickWithoutConnectionsIsSafe(t *testing.T) {
	controller := NewRouteController()
	server := NewServer(controller, "127.0.0.1:0")
	server.Kick("nobody") // must not panic
}
