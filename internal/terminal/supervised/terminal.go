package supervised

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/binaricat/netcatty/internal/terminal/pty"
)

// Terminal supervises PTY clients. A factory owns each attempt's bootstrap and
// tunnels; its release runs after all process I/O has stopped, including failures.
// Callbacks run serially and must not wait for Terminal.Close.
type Terminal struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	local      *pty.Session
	cols, rows uint16
	policy     RestartPolicy
	factory    TerminalFactory
	output     func([]byte) bool
	event      func(TerminalEvent)
	started    bool
	closed     bool
}

type RestartPolicy struct {
	MaxRestarts   int
	Backoff       time.Duration
	ReadExitGrace time.Duration
}
type TerminalFactory func(context.Context, uint16, uint16) (*pty.Session, func(), error)
type TerminalEvent struct {
	State    string `json:"state"`
	Attempt  int    `json:"attempt"`
	DelayMs  int64  `json:"delayMs,omitempty"`
	ExitCode *int   `json:"exitCode,omitempty"`
	Error    string `json:"error,omitempty"`
}

func NewTerminal(ctx context.Context, cols, rows uint16, policy RestartPolicy, factory TerminalFactory, output func([]byte) bool, event func(TerminalEvent)) *Terminal {
	ctx, cancel := context.WithCancel(ctx)
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}
	if policy.Backoff <= 0 {
		policy.Backoff = 250 * time.Millisecond
	}
	if policy.ReadExitGrace <= 0 {
		policy.ReadExitGrace = 150 * time.Millisecond
	}
	return &Terminal{ctx: ctx, cancel: cancel, done: make(chan struct{}), cols: cols, rows: rows, policy: policy, factory: factory, output: output, event: event}
}

func (t *Terminal) Start() error {
	t.mu.Lock()
	if t.started {
		t.mu.Unlock()
		return pty.ErrAlreadyStarted
	}
	t.started = true
	t.mu.Unlock()
	local, release, err := t.launch()
	if err != nil {
		t.cancel()
		close(t.done)
		return err
	}
	go t.run(local, release)
	return nil
}

func (t *Terminal) launch() (*pty.Session, func(), error) {
	t.mu.Lock()
	cols, rows := t.cols, t.rows
	t.mu.Unlock()
	if err := t.ctx.Err(); err != nil {
		return nil, nil, err
	}
	local, release, err := t.factory(t.ctx, cols, rows)
	if release == nil {
		release = func() {}
	}
	if err != nil {
		release()
		return nil, nil, err
	}
	t.mu.Lock()
	if t.ctx.Err() != nil || t.closed {
		t.mu.Unlock()
		_ = local.Close()
		release()
		return nil, nil, t.ctx.Err()
	}
	// Preserve a resize arriving during bootstrap or backoff.
	_ = local.Resize(local.Generation(), t.cols, t.rows)
	t.local = local
	t.mu.Unlock()
	return local, release, nil
}

func (t *Terminal) notify(event TerminalEvent) {
	if t.event != nil {
		t.event(event)
	}
}

func (t *Terminal) run(local *pty.Session, release func()) {
	defer close(t.done)
	defer t.cancel()
	attempt := 0
	for {
		t.notify(TerminalEvent{State: "running", Attempt: attempt})
		err := t.observe(local)
		t.mu.Lock()
		t.local = nil
		t.mu.Unlock()
		_ = local.Close()
		release()
		if t.ctx.Err() != nil {
			return
		}
		if err == nil {
			zero := 0
			t.notify(TerminalEvent{State: "exited", Attempt: attempt, ExitCode: &zero})
			return
		}
		for {
			if attempt >= t.policy.MaxRestarts {
				event := TerminalEvent{State: "failed", Attempt: attempt, Error: fmt.Sprintf("%v: %v", ErrTooManyRestarts, err)}
				var exit interface{ ExitCode() int }
				if errors.As(err, &exit) {
					code := exit.ExitCode()
					event.ExitCode = &code
				}
				t.notify(event)
				return
			}
			delay := min(t.policy.Backoff*time.Duration(1<<min(attempt, 6)), 5*time.Second)
			attempt++
			t.notify(TerminalEvent{State: "recovering", Attempt: attempt, DelayMs: delay.Milliseconds(), Error: err.Error()})
			timer := time.NewTimer(delay)
			select {
			case <-t.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			local, release, err = t.launch()
			if err != nil && t.ctx.Err() != nil {
				return
			}
			if err == nil {
				break
			}
		}
	}
}

func (t *Terminal) observe(local *pty.Session) error {
	exit := make(chan error, 1)
	read := make(chan error, 1)
	go func() { exit <- local.Wait() }()
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := local.ReadOnce(buf)
			if n > 0 && t.output != nil && !t.output(buf[:n]) {
				t.cancel()
				read <- context.Canceled
				return
			}
			if err != nil {
				read <- err
				return
			}
		}
	}()
	select {
	case <-t.ctx.Done():
		local.StopIO()
		<-exit
		<-read
		return t.ctx.Err()
	case err := <-exit:
		// ConPTY may retain pipe handles after process exit. Give the pump a
		// short drain window, then close it without altering the exit status.
		timer := time.NewTimer(t.policy.ReadExitGrace)
		defer timer.Stop()
		select {
		case <-read:
		case <-timer.C:
			local.StopIO()
			<-read
		case <-t.ctx.Done():
			local.StopIO()
			<-read
		}
		return err
	case readErr := <-read:
		timer := time.NewTimer(t.policy.ReadExitGrace)
		defer timer.Stop()
		select {
		case err := <-exit:
			return err
		case <-t.ctx.Done():
			local.StopIO()
			<-exit
			return t.ctx.Err()
		case <-timer.C:
			local.StopIO()
			<-exit
			return fmt.Errorf("terminal read failed while client was running: %w", readErr)
		}
	}
}

func (t *Terminal) Write(data []byte) (int, error) {
	t.mu.Lock()
	local := t.local
	t.mu.Unlock()
	if local == nil || t.ctx.Err() != nil {
		return 0, ErrNotRunning
	}
	return local.Write(local.Generation(), data)
}
func (t *Terminal) Resize(cols, rows uint16) error {
	if cols == 0 || rows == 0 {
		return errors.New("invalid terminal dimensions")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ctx.Err() != nil {
		return ErrNotRunning
	}
	t.cols, t.rows = cols, rows
	if t.local == nil {
		return nil
	}
	return t.local.Resize(t.local.Generation(), cols, rows)
}
func (t *Terminal) Cancel() { t.cancel() }

// Restart launches a fresh supervised run under the same Terminal. It is only
// allowed once the previous run loop finished (e.g. after a "failed" event);
// a live run loop, a never-started Terminal, or a closed Terminal reject the
// call. The attempt counter resets because the run loop starts over.
func (t *Terminal) Restart() error {
	t.mu.Lock()
	select {
	case <-t.done:
	default:
		t.mu.Unlock()
		return pty.ErrAlreadyStarted
	}
	if t.closed {
		t.mu.Unlock()
		return ErrClosed
	}
	// Close() cancels whichever context is current when it runs, so replacing
	// ctx/cancel here keeps a concurrent Close effective against the new run.
	t.ctx, t.cancel = context.WithCancel(context.Background())
	t.done = make(chan struct{})
	t.started = false
	t.mu.Unlock()
	return t.Start()
}
func (t *Terminal) Close() error {
	t.mu.Lock()
	t.closed = true
	cancel, started, done := t.cancel, t.started, t.done
	t.mu.Unlock()
	cancel()
	if started {
		<-done
	}
	return nil
}
func (t *Terminal) Done() <-chan struct{} { return t.done }
