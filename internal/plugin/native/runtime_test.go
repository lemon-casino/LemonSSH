package native

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestStartStopLifecycle(t *testing.T) {
	var stopped bool
	r := NewRuntime(nil, 4)
	err := r.Start("p1", func() error { return nil }, func() { stopped = true })
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsRunning("p1") {
		t.Fatal("must be running")
	}
	if err := r.Stop("p1"); err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Fatal("stop callback not called")
	}
	if r.IsRunning("p1") {
		t.Fatal("must be stopped")
	}
}

func TestAuthorizeGate(t *testing.T) {
	r := NewRuntime(func(pluginID string) error {
		return errors.New("not authorized for " + pluginID)
	}, 4)
	err := r.Start("p1", func() error { return nil }, func() {})
	if err == nil || !strings.Contains(err.Error(), "not authorized") {
		t.Fatalf("unauthorized must fail: %v", err)
	}
}

func TestMaxConcurrency(t *testing.T) {
	r := NewRuntime(nil, 2)
	var mu sync.Mutex
	count := 0
	for i := 0; i < 3; i++ {
		id := "plugin-" + strings.TrimSpace(string(rune('a'+i)))
		if err := r.Start(id, func() error { return nil }, func() {}); err == nil {
			mu.Lock()
			count++
			mu.Unlock()
		} else {
			break
		}
	}
	if count != 2 {
		t.Fatalf("max concurrency must be 2, got %d", count)
	}
}

func TestStopAll(t *testing.T) {
	r := NewRuntime(nil, 8)
	for i := 0; i < 3; i++ {
		_ = r.Start("p", func() error { return nil }, func() {})
	}
	r.StopAll()
	if len(r.process) != 0 {
		t.Fatal("stopAll must clear registry")
	}
}
