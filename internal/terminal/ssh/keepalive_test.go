package ssh

import (
	"testing"
	"time"
)

type stalledKeepalive struct{ closed chan struct{} }

func (c *stalledKeepalive) SendRequest(string, bool, []byte) (bool, []byte, error) {
	<-c.closed
	return false, nil, nil
}
func (c *stalledKeepalive) Close() error { close(c.closed); return nil }
func TestKeepaliveClosesUnresponsivePeer(t *testing.T) {
	client := &stalledKeepalive{closed: make(chan struct{})}
	stop := make(chan struct{})
	defer close(stop)
	go runKeepalive(client, 5*time.Millisecond, 2, stop)
	select {
	case <-client.closed:
	case <-time.After(time.Second):
		t.Fatal("unresponsive peer remained open")
	}
}
