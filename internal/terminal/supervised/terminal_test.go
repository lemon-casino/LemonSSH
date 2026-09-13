package supervised

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/terminal/pty"
)

type terminalProcess struct {
	reader *io.PipeReader
	writer *io.PipeWriter
	done   chan struct{}
	once   sync.Once
	err    error
	waits  atomic.Int32
	cols   atomic.Uint32
}

func newTerminalProcess() *terminalProcess {
	r, w := io.Pipe()
	return &terminalProcess{reader: r, writer: w, done: make(chan struct{})}
}
func (p *terminalProcess) end(err error) {
	p.once.Do(func() { p.err = err; close(p.done); p.writer.Close() })
}
func (p *terminalProcess) Read(b []byte) (int, error) { return p.reader.Read(b) }
func (p *terminalProcess) Write(b []byte) (int, error) {
	select {
	case <-p.done:
		return 0, io.ErrClosedPipe
	default:
		return len(b), nil
	}
}
func (p *terminalProcess) Close() error                   { p.reader.Close(); return nil }
func (p *terminalProcess) Resize(cols, rows uint16) error { p.cols.Store(uint32(cols)); return nil }
func (p *terminalProcess) Interrupt() error               { return nil }
func (p *terminalProcess) Kill() error                    { p.end(errors.New("killed")); return nil }
func (p *terminalProcess) Wait() error                    { p.waits.Add(1); <-p.done; return p.err }
func (p *terminalProcess) PID() int                       { return 1 }

type terminalBackend struct{ p *terminalProcess }

func (b terminalBackend) Start(context.Context, pty.Config) (pty.Process, error) { return b.p, nil }

func terminalFixture(t *testing.T, max int) (*Terminal, chan *terminalProcess, chan TerminalEvent, *atomic.Int32) {
	t.Helper()
	processes := make(chan *terminalProcess, 8)
	events := make(chan TerminalEvent, 20)
	released := &atomic.Int32{}
	term := NewTerminal(context.Background(), 80, 24, RestartPolicy{MaxRestarts: max, Backoff: time.Millisecond, ReadExitGrace: 10 * time.Millisecond},
		func(ctx context.Context, cols, rows uint16) (*pty.Session, func(), error) {
			p := newTerminalProcess()
			s := pty.NewSession(pty.Config{Cols: cols, Rows: rows})
			err := s.Start(ctx, terminalBackend{p})
			processes <- p
			return s, func() { released.Add(1) }, err
		}, func([]byte) bool { return true }, func(e TerminalEvent) { events <- e })
	t.Cleanup(func() { term.Close() })
	if err := term.Start(); err != nil {
		t.Fatal(err)
	}
	return term, processes, events, released
}
func nextProcess(t *testing.T, c <-chan *terminalProcess) *terminalProcess {
	t.Helper()
	select {
	case p := <-c:
		return p
	case <-time.After(3 * time.Second):
		t.Fatal("process timeout")
		return nil
	}
}
func nextState(t *testing.T, c <-chan TerminalEvent, state string) TerminalEvent {
	t.Helper()
	for {
		select {
		case e := <-c:
			if e.State == state {
				return e
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("state timeout: %s", state)
			return TerminalEvent{}
		}
	}
}
func awaitDone(t *testing.T, term *Terminal) {
	t.Helper()
	select {
	case <-term.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("supervisor did not stop")
	}
}

func TestTerminalRecoversUnexpectedDeathAndReapsOnce(t *testing.T) {
	term, processes, events, released := terminalFixture(t, 3)
	first := nextProcess(t, processes)
	first.end(pty.ExitError{Code: 17})
	if e := nextState(t, events, "recovering"); e.Attempt != 1 {
		t.Fatal(e)
	}
	second := nextProcess(t, processes)
	if _, err := term.Write([]byte("input")); err != nil {
		t.Fatal(err)
	}
	second.end(nil)
	awaitDone(t, term)
	if first.waits.Load() != 1 || second.waits.Load() != 1 || released.Load() != 2 {
		t.Fatal("each process must wait/release exactly once")
	}
}

func TestTerminalNoRetryNormalExitOrClose(t *testing.T) {
	for _, userClose := range []bool{false, true} {
		t.Run(map[bool]string{true: "close", false: "normal"}[userClose], func(t *testing.T) {
			term, processes, events, released := terminalFixture(t, 3)
			first := nextProcess(t, processes)
			if userClose {
				term.Close()
			} else {
				first.end(nil)
			}
			awaitDone(t, term)
			if len(processes) != 0 || released.Load() != 1 || first.waits.Load() != 1 {
				t.Fatal("unexpected retry or duplicate wait")
			}
			for len(events) > 0 {
				if e := <-events; e.State == "recovering" {
					t.Fatal(e)
				}
			}
		})
	}
}

func TestTerminalRestartLimitAndBackoff(t *testing.T) {
	term, processes, events, _ := terminalFixture(t, 3)
	for i := 0; i < 4; i++ {
		nextProcess(t, processes).end(pty.ExitError{Code: 9})
	}
	awaitDone(t, term)
	var delays []int64
	var failed TerminalEvent
	for len(events) > 0 {
		e := <-events
		if e.State == "recovering" {
			delays = append(delays, e.DelayMs)
		}
		if e.State == "failed" {
			failed = e
		}
	}
	if len(delays) != 3 || delays[0] != 1 || delays[1] != 2 || delays[2] != 4 || failed.Attempt != 3 || !strings.Contains(failed.Error, ErrTooManyRestarts.Error()) {
		t.Fatalf("%v %+v", delays, failed)
	}
}

func TestTerminalReadErrorKillsAndRecovers(t *testing.T) {
	term, processes, events, _ := terminalFixture(t, 1)
	first := nextProcess(t, processes)
	first.writer.CloseWithError(io.ErrUnexpectedEOF)
	e := nextState(t, events, "recovering")
	if !strings.Contains(e.Error, "terminal read failed") {
		t.Fatal(e)
	}
	second := nextProcess(t, processes)
	second.end(nil)
	awaitDone(t, term)
}

func TestTerminalCloseDuringBackoffAndConcurrentIO(t *testing.T) {
	term, processes, events, _ := terminalFixture(t, 3)
	term.policy.Backoff = time.Second // no death observer has read the policy yet
	p := nextProcess(t, processes)
	var workers sync.WaitGroup
	for i := 0; i < 4; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 100; j++ {
				term.Write([]byte("x"))
				term.Resize(100, 30)
			}
		}()
	}
	p.end(errors.New("crash"))
	nextState(t, events, "recovering")
	term.Close()
	workers.Wait()
	awaitDone(t, term)
	if len(processes) != 0 {
		t.Fatal("close launched a replacement")
	}
}

func TestTerminalCloseCancelsBootstrap(t *testing.T) {
	entered := make(chan struct{})
	term := NewTerminal(context.Background(), 80, 24, RestartPolicy{MaxRestarts: 3}, func(ctx context.Context, _, _ uint16) (*pty.Session, func(), error) {
		close(entered)
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}, nil, nil)
	result := make(chan error, 1)
	go func() { result <- term.Start() }()
	<-entered
	term.Close()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestTerminalRestartRefusesWhileRunLoopLives(t *testing.T) {
	term, processes, _, _ := terminalFixture(t, 3)
	if err := term.Restart(); !errors.Is(err, pty.ErrAlreadyStarted) {
		t.Fatalf("restart before the first run finished: %v", err)
	}
	p := nextProcess(t, processes)
	if err := term.Restart(); !errors.Is(err, pty.ErrAlreadyStarted) {
		t.Fatalf("restart while running: %v", err)
	}
	p.end(nil)
	awaitDone(t, term)
}

func TestTerminalRestartAfterFailureStartsFreshRun(t *testing.T) {
	term, processes, events, released := terminalFixture(t, 1)
	first := nextProcess(t, processes)
	first.end(pty.ExitError{Code: 9})
	second := nextProcess(t, processes)
	second.end(pty.ExitError{Code: 9})
	nextState(t, events, "failed")
	awaitDone(t, term)
	if err := term.Restart(); err != nil {
		t.Fatal(err)
	}
	third := nextProcess(t, processes)
	if e := nextState(t, events, "running"); e.Attempt != 0 {
		t.Fatalf("attempt counter did not reset: %+v", e)
	}
	third.end(nil)
	awaitDone(t, term)
	if first.waits.Load() != 1 || second.waits.Load() != 1 || third.waits.Load() != 1 || released.Load() != 3 {
		t.Fatal("restart must reap old clients exactly once and launch one replacement")
	}
}

func TestTerminalCloseDuringRestartAbortsRelaunch(t *testing.T) {
	events := make(chan TerminalEvent, 10)
	processes := make(chan *terminalProcess, 4)
	entered := make(chan struct{}, 1)
	resume := make(chan struct{})
	var attempts atomic.Int32
	term := NewTerminal(context.Background(), 80, 24, RestartPolicy{MaxRestarts: 0, Backoff: time.Millisecond},
		func(ctx context.Context, cols, rows uint16) (*pty.Session, func(), error) {
			if attempts.Add(1) > 1 {
				entered <- struct{}{}
				select {
				case <-resume:
				case <-ctx.Done():
					return nil, nil, ctx.Err()
				}
			}
			p := newTerminalProcess()
			local := pty.NewSession(pty.Config{Cols: cols, Rows: rows})
			err := local.Start(ctx, terminalBackend{p})
			processes <- p
			return local, func() {}, err
		}, nil, func(e TerminalEvent) { events <- e })
	t.Cleanup(func() { term.Close() })
	if err := term.Start(); err != nil {
		t.Fatal(err)
	}
	nextProcess(t, processes).end(pty.ExitError{Code: 5})
	nextState(t, events, "failed")
	awaitDone(t, term)

	result := make(chan error, 1)
	go func() { result <- term.Restart() }()
	<-entered // relaunch bootstrap is in flight
	if err := term.Close(); err != nil {
		t.Fatal(err)
	}
	close(resume)
	if err := <-result; err == nil {
		t.Fatal("restart must fail once Close won the race")
	}
	awaitDone(t, term)
	if len(processes) != 0 {
		t.Fatal("closed terminal spawned a client during restart")
	}
	for len(events) > 0 {
		if e := <-events; e.State == "running" {
			t.Fatal("closed terminal emitted running after restart raced Close")
		}
	}
}

func TestTerminalSpawnFailuresConsumeRestartBudget(t *testing.T) {
	p := newTerminalProcess()
	var attempts, releases atomic.Int32
	events := make(chan TerminalEvent, 10)
	term := NewTerminal(context.Background(), 80, 24, RestartPolicy{MaxRestarts: 3, Backoff: time.Millisecond},
		func(ctx context.Context, cols, rows uint16) (*pty.Session, func(), error) {
			release := func() { releases.Add(1) }
			if attempts.Add(1) > 1 {
				return nil, release, errors.New("spawn failed")
			}
			local := pty.NewSession(pty.Config{Cols: cols, Rows: rows})
			err := local.Start(ctx, terminalBackend{p})
			return local, release, err
		}, nil, func(event TerminalEvent) { events <- event })
	t.Cleanup(func() { term.Close() })
	if err := term.Start(); err != nil {
		t.Fatal(err)
	}
	p.end(errors.New("crash"))
	awaitDone(t, term)
	if attempts.Load() != 4 || releases.Load() != 4 || nextState(t, events, "failed").Attempt != 3 {
		t.Fatal("spawn errors exceeded budget or leaked attempt resources")
	}
}
