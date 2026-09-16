package terminaluse

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/terminal/dataplane"
	"github.com/binaricat/netcatty/internal/terminal/pty"
	"github.com/binaricat/netcatty/internal/terminal/supervised"
)

// newHelperServiceFixture builds a TerminalService with a supervised mosh
// helper wired through the production lifecycle callback so the tests below
// exercise the real "failed" handling instead of a look-alike.
func newHelperServiceFixture(t *testing.T, policy supervised.RestartPolicy) (*Service, *terminalSession, chan *recoveryProcess, chan HelperSessionState, *atomic.Int32) {
	t.Helper()
	controller := dataplane.NewRouteController()
	service := New(controller, dataplane.NewServer(controller, ""), nil)
	running := make(chan HelperSessionState, 8)
	var eventsMu sync.Mutex
	var lifecycle []HelperSessionState
	var exits []TerminalExitStatus
	service.SetEventEmitter(func(name string, payload any) {
		switch name {
		case "mosh:lifecycle":
			if state, ok := payload.(HelperSessionState); ok {
				eventsMu.Lock()
				lifecycle = append(lifecycle, state)
				eventsMu.Unlock()
				if state.State == "running" {
					running <- state
				}
			}
		case "terminal:exit":
			if status, ok := payload.(TerminalExitStatus); ok {
				eventsMu.Lock()
				exits = append(exits, status)
				eventsMu.Unlock()
			}
		}
	})
	bootstrap, err := controller.Open("helper")
	if err != nil {
		t.Fatal(err)
	}
	term := &terminalSession{bootstrap: bootstrap, helperState: HelperSessionState{SessionID: "helper", BootEpoch: 7, Kind: "mosh", RecoveryMode: "new-shell", Readiness: "client-started"}}
	processes := make(chan *recoveryProcess, 8)
	var attempts atomic.Int32
	factory := func(ctx context.Context, cols, rows uint16) (*pty.Session, func(), error) {
		p := &recoveryProcess{done: make(chan struct{})}
		attempts.Add(1)
		local := pty.NewSession(pty.Config{Cols: cols, Rows: rows})
		err := local.Start(ctx, recoveryBackend{p})
		processes <- p
		return local, nil, err
	}
	term.helperFactory = factory
	term.helper = supervised.NewTerminal(context.Background(), 80, 24, policy, factory,
		func(b []byte) bool { return service.publishOutput("helper", b) },
		service.handleHelperLifecycle("helper", "mosh", term))
	service.sessions["helper"] = term
	if err := term.helper.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close("helper") })
	return service, term, processes, running, &attempts
}

func awaitHelperState(t *testing.T, service *Service, want string) HelperSessionState {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		state, err := service.GetHelperSessionState("helper")
		if err == nil && state.State == want {
			return state
		}
		if time.Now().After(deadline) {
			t.Fatalf("helper never reached %q: %+v %v", want, state, err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestHelperFailureKeepsSessionAliveForManualRestart(t *testing.T) {
	service, term, processes, running, attempts := newHelperServiceFixture(t, supervised.RestartPolicy{MaxRestarts: 1, Backoff: time.Millisecond})
	<-running
	first := <-processes
	first.end(pty.ExitError{Code: 9})
	<-running
	second := <-processes
	second.end(pty.ExitError{Code: 9})
	// The failed event arrives asynchronously once the supervisor observes the
	// second crash and exhausts MaxRestarts.
	awaitHelperState(t, service, "failed")

	state, err := service.GetHelperSessionState("helper")
	if err != nil || state.State != "failed" || state.SessionID != "helper" {
		t.Fatalf("failed helper state lost: %+v %v", state, err)
	}
	// The session stays registered: write/resize reach the (idle) helper
	// instead of being rejected with "session not found".
	if _, err = service.Write("helper", []byte("x")); !errors.Is(err, supervised.ErrNotRunning) {
		t.Fatalf("write after failure must reach the session, not reject it: %v", err)
	}
	if err = service.Resize("helper", 100, 30); !errors.Is(err, supervised.ErrNotRunning) {
		t.Fatalf("resize after failure must reach the session, not reject it: %v", err)
	}

	if _, err = service.RestartHelper("helper"); err != nil {
		t.Fatal(err)
	}
	third := <-processes
	if state := <-running; state.State != "running" || state.Attempt != 0 {
		t.Fatalf("restart did not run a fresh attempt: %+v", state)
	}
	if state, err := service.GetHelperSessionState("helper"); err != nil || state.State != "running" {
		t.Fatalf("helper state did not return to running: %+v %v", state, err)
	}
	if attempts.Load() != 3 {
		t.Fatalf("restart must launch exactly one replacement client, got %d", attempts.Load())
	}
	if service.sessions["helper"] != term {
		t.Fatal("restart must keep the same terminal session")
	}

	// A clean user exit still closes the whole session (today's behavior).
	third.end(nil)
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err = service.GetHelperSessionState("helper"); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("clean exit did not close the session")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err = service.RestartHelper("helper"); err == nil {
		t.Fatal("RestartHelper must reject a closed session")
	}
}

func TestRestartHelperRejectsRunningAndClosedHelpers(t *testing.T) {
	service, _, processes, running, _ := newHelperServiceFixture(t, supervised.RestartPolicy{MaxRestarts: 1, Backoff: time.Millisecond})
	<-running
	<-processes
	if _, err := service.RestartHelper("helper"); !errors.Is(err, pty.ErrAlreadyStarted) {
		t.Fatalf("RestartHelper must reject a live run loop: %v", err)
	}
	if err := service.Close("helper"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RestartHelper("helper"); err == nil {
		t.Fatal("RestartHelper must reject a session closed via Close")
	}
}

func TestRestartHelperRacingCloseEndsClosed(t *testing.T) {
	service, _, processes, running, _ := newHelperServiceFixture(t, supervised.RestartPolicy{MaxRestarts: 1, Backoff: time.Millisecond})
	<-running
	<-processes
	restart := make(chan error, 1)
	go func() {
		_, err := service.RestartHelper("helper")
		restart <- err
	}()
	if err := service.Close("helper"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-restart:
	case <-time.After(3 * time.Second):
		t.Fatal("RestartHelper deadlocked against Close")
	}
	if _, err := service.GetHelperSessionState("helper"); err == nil {
		t.Fatal("session must stay closed after the restart/close race")
	}
}
