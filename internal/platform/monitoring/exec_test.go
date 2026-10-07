package monitoring

import (
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type testChannel struct {
	once   sync.Once
	out    io.Writer
	closed chan struct{}
	text   string
	block  bool
}

func (c *testChannel) SetOutput(out, stderr io.Writer) { c.out = out }
func (c *testChannel) Run(string) error {
	if c.block {
		<-c.closed
		return errors.New("closed")
	}
	_, e := io.WriteString(c.out, c.text)
	return e
}
func (c *testChannel) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}
func TestExecuteIdentityAndCancellation(t *testing.T) {
	a := &testChannel{text: "host-a", closed: make(chan struct{})}
	b := &testChannel{text: "host-b", closed: make(chan struct{})}
	for _, c := range []*testChannel{a, b} {
		got, e := Execute(context.Background(), func() (Channel, error) { return c, nil }, "probe")
		if e != nil || got != c.text {
			t.Fatal(got, e)
		}
	}
	blocked := &testChannel{block: true, closed: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, e := Execute(ctx, func() (Channel, error) { return blocked, nil }, "probe"); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	select {
	case <-blocked.closed:
	case <-time.After(time.Second):
		t.Fatal("channel not closed")
	}
}
func TestLocalMonitoring(t *testing.T) {
	out, e := ExecuteLocal(context.Background(), ProbeCommand)
	if runtime.GOOS != "linux" {
		if e == nil || !strings.Contains(e.Error(), "unsupported") {
			t.Fatal(out, e)
		}
		return
	}
	if e != nil || !strings.HasPrefix(out, "Linux\n") {
		t.Fatal(out, e)
	}
}
func TestPendingOpensAreBounded(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	entered := make(chan struct{}, 16)
	open := func() (Channel, error) { entered <- struct{}{}; <-release; return nil, errors.New("released") }
	for i := 0; i < 12; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		Execute(ctx, open, "probe")
		cancel()
	}
	if len(entered) > 8 {
		t.Fatalf("unbounded pending opens: %d", len(entered))
	}
}
func TestExecuteErrorsAndBounds(t *testing.T) {
	if _, e := Execute(context.Background(), func() (Channel, error) { return nil, errors.New("utility absent") }, "probe"); e == nil {
		t.Fatal("swallowed error")
	}
	c := &testChannel{text: string(make([]byte, MaxOutput+1)), closed: make(chan struct{})}
	if _, e := Execute(context.Background(), func() (Channel, error) { return c, nil }, "probe"); e == nil {
		t.Fatal("accepted oversized output")
	}
}
