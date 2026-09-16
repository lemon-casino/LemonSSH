package terminaluse

import (
	"errors"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/terminal/dataplane"
	"github.com/binaricat/netcatty/internal/terminal/pty"
)

func TestTerminalExitStatusRequiresActualWaitResult(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		reason string
		code   *int
	}{
		{"clean", nil, "exited", intPtr(0)},
		{"nonzero", pty.ExitError{Code: 7}, "exited", intPtr(7)},
		{"transport", errors.New("connection reset"), "error", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := terminalWaitExit("s", tc.err)
			if got.Reason != tc.reason || (got.ExitCode == nil) != (tc.code == nil) || got.ExitCode != nil && *got.ExitCode != *tc.code {
				t.Fatalf("unexpected status: %+v", got)
			}
		})
	}
}
func TestTerminalClosePreservesIntentAndPublishesStatusBeforeComplete(t *testing.T) {
	controller := dataplane.NewRouteController()
	service := New(controller, dataplane.NewServer(controller, "127.0.0.1:0"), nil)
	bootstrap, err := controller.Open("test")
	if err != nil {
		t.Fatal(err)
	}
	id := bootstrap.SessionID
	service.sessions[id] = &terminalSession{bootstrap: bootstrap}
	count := 0
	service.SetEventEmitter(func(name string, payload any) {
		if name != "terminal:exit" {
			return
		}
		count++
		status := service.GetExitStatus(id)
		if status == nil || status.Reason != "closed" || status.ExitCode != nil {
			t.Fatalf("status not available at event: %+v", status)
		}
	})
	if err := service.Close(id); err != nil {
		t.Fatal(err)
	}
	_ = service.closeWithStatus(id, terminalWaitExit(id, nil))
	if count != 1 || service.GetExitStatus(id).Reason != "closed" {
		t.Fatal("late wait overwrote intentional close")
	}
}

func TestTerminalActualLocalExit(t *testing.T) {
	for _, code := range []int{0, 7} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			controller := dataplane.NewRouteController()
			service := New(controller, dataplane.NewServer(controller, "127.0.0.1:0"), nil)
			request := LocalStartRequest{Shell: "/bin/sh", ShellArgs: []string{"-c", "exit " + strconv.Itoa(code)}, Cols: 80, Rows: 24}
			if runtime.GOOS == "windows" {
				request.Shell = "cmd.exe"
				request.ShellArgs = []string{"/c", "exit", strconv.Itoa(code)}
			}
			id, err := service.StartLocalWithOptions(request)
			if err != nil {
				t.Fatal(err)
			}
			defer service.Close(id)
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if status := service.GetExitStatus(id); status != nil {
					if status.Reason != "exited" || status.ExitCode == nil || *status.ExitCode != code {
						t.Fatalf("actual exit mismatch: %+v", status)
					}
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatal("native process exit was never observed")
		})
	}
}

func intPtr(n int) *int { return &n }
