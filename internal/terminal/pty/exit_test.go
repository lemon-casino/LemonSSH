package pty

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

func TestPTYExitHelper(t *testing.T) {
	if os.Getenv("NETCATTY_PTY_EXIT_TEST") == "1" {
		os.Exit(23)
	}
}

func TestPlatformWaitReportsProcessExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	s := NewSession(Config{Shell: path, Args: []string{"-test.run=^TestPTYExitHelper$"}, Env: append(os.Environ(), "NETCATTY_PTY_EXIT_TEST=1")})
	if err = s.Start(ctx, NewPlatformBackend()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	done := make(chan error, 1)
	go func() { done <- s.Wait() }()
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := s.ReadOnce(buf); err != nil {
				return
			}
		}
	}()
	select {
	case err = <-done:
		var exit interface{ ExitCode() int }
		if !errors.As(err, &exit) || exit.ExitCode() != 23 {
			t.Fatalf("exit status lost: %v", err)
		}
	case <-ctx.Done():
		s.StopIO()
		t.Fatal("wait did not observe process death")
	}
	var closeGroup sync.WaitGroup
	for i := 0; i < 5; i++ {
		closeGroup.Add(1)
		go func() { defer closeGroup.Done(); s.Close() }()
	}
	closeGroup.Wait()
	if _, err = s.Write(s.Generation(), []byte("closed")); !errors.Is(err, ErrSessionNotFound) {
		t.Fatal(err)
	}
}
