package sftp

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// Rejecting sudo must terminate negotiation and surface the remote diagnostic.
func TestElevatedNegotiationFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := negotiateElevated(ctx, strings.NewReader(""), nopWriteCloser{io.Discard}, func() error { return nil }, func() string { return "sudo: a password is required" })
	if err == nil || !strings.Contains(err.Error(), "password is required") {
		t.Fatalf("got %v", err)
	}
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func TestElevatedNegotiationTimeoutClosesChannel(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	closed := false
	_, err := negotiateElevated(ctx, reader, nopWriteCloser{io.Discard}, func() error { closed = true; return reader.Close() }, func() string { return "" })
	if err == nil || !closed {
		t.Fatalf("timeout must close channel, err=%v closed=%v", err, closed)
	}
}
