package main

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/terminal/dataplane"
	"github.com/binaricat/netcatty/internal/terminal/pty"
	"github.com/binaricat/netcatty/internal/terminal/ssh"
	"github.com/binaricat/netcatty/internal/terminal/supervised"
)

type recoveryProcess struct {
	done   chan struct{}
	once   sync.Once
	err    error
	waits  atomic.Int32
	writes atomic.Int32
}

func (p *recoveryProcess) Read([]byte) (int, error)    { <-p.done; return 0, io.EOF }
func (p *recoveryProcess) Write(b []byte) (int, error) { p.writes.Add(1); return len(b), nil }
func (p *recoveryProcess) Close() error                { return nil }
func (p *recoveryProcess) Resize(uint16, uint16) error { return nil }
func (p *recoveryProcess) Interrupt() error            { return nil }
func (p *recoveryProcess) Kill() error                 { p.end(errors.New("killed")); return nil }
func (p *recoveryProcess) Wait() error                 { p.waits.Add(1); <-p.done; return p.err }
func (p *recoveryProcess) PID() int                    { return 1 }
func (p *recoveryProcess) end(err error)               { p.once.Do(func() { p.err = err; close(p.done) }) }

type recoveryBackend struct{ p *recoveryProcess }

func (b recoveryBackend) Start(context.Context, pty.Config) (pty.Process, error) { return b.p, nil }

func TestHelperRouteRebindDuringRecoveryAndConcurrentClose(t *testing.T) {
	controller := dataplane.NewRouteController()
	service := NewTerminalService(controller, dataplane.NewServer(controller, ""), nil)
	bootstrap, _ := controller.Open("helper")
	term := &terminalSession{bootstrap: bootstrap}
	processes := make(chan *recoveryProcess, 4)
	running := make(chan struct{}, 4)
	term.helper = supervised.NewTerminal(context.Background(), 80, 24, supervised.RestartPolicy{MaxRestarts: 2, Backoff: time.Millisecond}, func(ctx context.Context, cols, rows uint16) (*pty.Session, func(), error) {
		p := &recoveryProcess{done: make(chan struct{})}
		local := pty.NewSession(pty.Config{Cols: cols, Rows: rows})
		err := local.Start(ctx, recoveryBackend{p})
		processes <- p
		return local, nil, err
	}, func(b []byte) bool { return service.publishOutput("helper", b) }, func(event supervised.TerminalEvent) {
		if event.State == "running" {
			running <- struct{}{}
		}
	})
	service.sessions["helper"] = term
	if err := term.helper.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close("helper") })
	<-running
	first := <-processes
	first.end(pty.ExitError{Code: 13})
	rebound, err := service.Reconnect("helper")
	if err != nil || rebound.Generation != bootstrap.Generation+1 {
		t.Fatal("route not rebound", err)
	}
	select {
	case <-running:
	case <-time.After(3 * time.Second):
		t.Fatal("no recovered client")
	}
	second := <-processes
	if service.sessions["helper"] != term {
		t.Fatal("process replacement replaced route owner")
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				service.Write("helper", []byte("x"))
				service.Resize("helper", 100, 30)
				service.handleUrgent("helper", []byte{3})
			}
		}()
	}
	service.Close("helper")
	wg.Wait()
	if first.waits.Load() != 1 || second.waits.Load() != 1 {
		t.Fatal("process reaped more than once")
	}
	if _, ok := controller.Generation("helper"); ok {
		t.Fatal("route survived close")
	}
	if len(processes) != 0 {
		t.Fatal("user Close caused retry")
	}
}

func TestHelperBootstrapCancellationRemovesMFARequest(t *testing.T) {
	controller := dataplane.NewRouteController()
	service := NewTerminalService(controller, dataplane.NewServer(controller, ""), ssh.NewKnownHosts(filepath.Join(t.TempDir(), "known_hosts")))
	challenge := make(chan ssh.KeyboardChallenge, 1)
	service.setChallengeEmitter(func(event ssh.KeyboardChallenge) { challenge <- event })
	ctx, cancel := context.WithCancel(context.Background())
	config, err := sshBootstrapConfig(ctx, service, MoshStartRequest{SSHConnectRequest: SSHConnectRequest{Hostname: "target", Username: "user", EnableMFA: true, JumpHosts: []SSHConnectRequest{{Hostname: "jump", Username: "hop", EnableMFA: true}}}})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := config.JumpHosts[0].Auth.Challenge("MFA", "", []string{"OTP"}, []bool{false})
		result <- err
	}()
	request := <-challenge
	if request.Hostname != "jump" {
		t.Fatal("MFA mislabeled hop")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("MFA ignored cancel")
	}
	if service.interactive.PendingRequestID() != "" {
		t.Fatal("MFA request leaked")
	}
}
