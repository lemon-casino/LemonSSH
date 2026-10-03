package main

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type fakeMainWindow struct {
	mu    sync.Mutex
	hides int
}

func (f *fakeMainWindow) Hide() application.Window {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hides++
	return nil
}

func (f *fakeMainWindow) Focus() {}

func (f *fakeMainWindow) HideCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hides
}

func newTestWindowLifecycleService(closeToTray bool, timeout time.Duration) (*WindowLifecycleService, *fakeMainWindow, *[]string, *bool) {
	window := &fakeMainWindow{}
	var events []string
	quit := false
	service := &WindowLifecycleService{
		mainWindow:        window,
		closeToTray:       closeToTray,
		dirtyCheckTimeout: timeout,
	}
	service.emit = func(name string) {
		events = append(events, name)
	}
	service.quit = func() {
		quit = true
	}
	return service, window, &events, &quit
}

func TestCloseToTrayHidesInsteadOfQuitting(t *testing.T) {
	service, window, events, quit := newTestWindowLifecycleService(true, 50*time.Millisecond)

	service.handleCloseRequest()

	if window.HideCount() != 1 {
		t.Fatalf("close-to-tray must hide the main window once, got %d hides", window.HideCount())
	}
	if *quit {
		t.Fatal("close-to-tray must not quit the app")
	}
	if len(*events) != 0 {
		t.Fatalf("close-to-tray must not run the dirty-editor round trip, got events %v", *events)
	}
}

func TestCleanCloseQuitsAfterRendererConfirms(t *testing.T) {
	service, window, events, quit := newTestWindowLifecycleService(false, 2*time.Second)

	done := make(chan struct{})
	go func() {
		service.handleCloseRequest()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		hasQuery := func() bool {
			service.mu.Lock()
			defer service.mu.Unlock()
			return service.pendingDirtyQuery != nil
		}()
		if hasQuery || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	service.ReportDirtyEditorsResult(false)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("clean close did not finish")
	}
	if !*quit {
		t.Fatal("a confirmed clean close must quit the app")
	}
	if window.HideCount() != 0 {
		t.Fatal("quit path must not hide the window")
	}
	if len(*events) != 1 || (*events)[0] != windowCheckDirtyEditorsEvent {
		t.Fatalf("clean close must query the renderer exactly once, got %v", *events)
	}
}

func TestDirtyEditorReportKeepsWindowOpen(t *testing.T) {
	service, window, _, quit := newTestWindowLifecycleService(false, 2*time.Second)

	done := make(chan struct{})
	go func() {
		service.handleCloseRequest()
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		hasQuery := func() bool {
			service.mu.Lock()
			defer service.mu.Unlock()
			return service.pendingDirtyQuery != nil
		}()
		if hasQuery || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	service.ReportDirtyEditorsResult(true)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("dirty close did not finish")
	}
	if *quit {
		t.Fatal("a dirty report must cancel the quit")
	}
	if window.HideCount() != 0 {
		t.Fatal("a dirty report must keep the window visible")
	}
}

func TestRendererTimeoutFailsOpen(t *testing.T) {
	service, _, events, quit := newTestWindowLifecycleService(false, 5*time.Millisecond)

	service.handleCloseRequest()

	if !*quit {
		t.Fatal("a wedged renderer must fail open and quit after the timeout")
	}
	if len(*events) != 1 || (*events)[0] != windowCheckDirtyEditorsEvent {
		t.Fatalf("timeout path must still query the renderer once, got %v", *events)
	}
}

func TestConcurrentCloseRequestsShareOneRendererQuery(t *testing.T) {
	service, _, events, quit := newTestWindowLifecycleService(false, 2*time.Second)

	done := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		go func() {
			service.handleCloseRequest()
			done <- struct{}{}
		}()
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		hasQuery := func() bool {
			service.mu.Lock()
			defer service.mu.Unlock()
			return service.pendingDirtyQuery != nil
		}()
		if hasQuery || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	service.ReportDirtyEditorsResult(false)

	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("concurrent close requests did not settle")
		}
	}
	if *quit != true {
		t.Fatal("shared clean query must still quit")
	}
	if len(*events) != 1 {
		t.Fatalf("concurrent closes must share a single renderer query, got %v", *events)
	}
}

func TestReportWithoutPendingQueryIsIgnored(t *testing.T) {
	service, _, _, quit := newTestWindowLifecycleService(true, 50*time.Millisecond)

	service.ReportDirtyEditorsResult(false)

	if *quit {
		t.Fatal("a report without a pending query must not quit the app")
	}
}

func TestSetCloseToTrayUpdatesGuardDecision(t *testing.T) {
	service, window, _, quit := newTestWindowLifecycleService(false, 50*time.Millisecond)

	result, err := service.SetCloseToTray(true)
	if err != nil {
		t.Fatalf("SetCloseToTray failed: %v", err)
	}
	if !result.Success || !result.Enabled {
		t.Fatalf("SetCloseToTray result mismatch: %+v", result)
	}
	status, err := service.IsCloseToTray()
	if err != nil {
		t.Fatalf("IsCloseToTray failed: %v", err)
	}
	if !status.Enabled {
		t.Fatal("IsCloseToTray must reflect the enabled flag")
	}

	service.handleCloseRequest()

	if window.HideCount() != 1 {
		t.Fatal("enabling close-to-tray must switch the close decision to hide")
	}
	if *quit {
		t.Fatal("enabling close-to-tray must switch the close decision away from quit")
	}
}

func TestRequestCloseMainRunsQuitGuard(t *testing.T) {
	for _, name := range []string{"main", "", "  "} {
		service, window, events, quit := newTestWindowLifecycleService(true, 50*time.Millisecond)

		result, err := service.RequestClose(name)
		if err != nil {
			t.Fatalf("RequestClose(%q) failed: %v", name, err)
		}
		if !result.Success || !result.Guarded {
			t.Fatalf("RequestClose(%q) must route through the guard, got %+v", name, result)
		}
		if window.HideCount() != 1 {
			t.Fatalf("RequestClose(%q) with close-to-tray must hide the main window, got %d hides", name, window.HideCount())
		}
		if *quit {
			t.Fatalf("RequestClose(%q) must not quit directly", name)
		}
		if len(*events) != 0 {
			t.Fatalf("close-to-tray must skip the renderer query, got %v", *events)
		}
	}
}

func TestRequestCloseNamedWindowClosesItself(t *testing.T) {
	service, window, events, quit := newTestWindowLifecycleService(true, 50*time.Millisecond)
	var closed []string
	service.closeWindow = func(name string) bool {
		closed = append(closed, name)
		return true
	}

	result, err := service.RequestClose("settings")
	if err != nil {
		t.Fatalf("RequestClose(settings) failed: %v", err)
	}
	if !result.Success || result.Guarded {
		t.Fatalf("named close must not run the quit guard, got %+v", result)
	}
	if len(closed) != 1 || closed[0] != "settings" {
		t.Fatalf("named close must close the requested window, got %v", closed)
	}
	if window.HideCount() != 0 {
		t.Fatal("named close must not touch the main window")
	}
	if *quit || len(*events) != 0 {
		t.Fatal("named close must not run the quit guard")
	}
}

func TestRequestCloseUnknownWindowFailsClosed(t *testing.T) {
	service, window, _, quit := newTestWindowLifecycleService(true, 50*time.Millisecond)
	service.closeWindow = func(string) bool { return false }

	result, err := service.RequestClose("ghost")
	if err == nil {
		t.Fatal("an unknown window name must fail, not fall through to the guard")
	}
	if result.Success {
		t.Fatalf("an unknown window name must not report success: %+v", result)
	}
	if window.HideCount() != 0 || *quit {
		t.Fatal("a failed named close must not touch the main window")
	}
}

func TestWindowOpacityPlan(t *testing.T) {
	cases := []struct {
		opacity     float64
		alpha       uint32
		keepLayered bool
		ok          bool
	}{
		{opacity: 1, alpha: 255, keepLayered: false, ok: true},
		{opacity: 0.5, alpha: 128, keepLayered: true, ok: true},
		{opacity: 0.75, alpha: 191, keepLayered: true, ok: true},
		{opacity: 0, alpha: 0, keepLayered: true, ok: true},
		{opacity: -0.1, ok: false},
		{opacity: 1.5, ok: false},
		{opacity: math.NaN(), ok: false},
	}
	for _, testCase := range cases {
		alpha, keepLayered, ok := windowOpacityPlan(testCase.opacity)
		if ok != testCase.ok {
			t.Fatalf("windowOpacityPlan(%v) ok = %v, want %v", testCase.opacity, ok, testCase.ok)
		}
		if !ok {
			continue
		}
		if alpha != testCase.alpha || keepLayered != testCase.keepLayered {
			t.Fatalf("windowOpacityPlan(%v) = (%v,%v), want (%v,%v)",
				testCase.opacity, alpha, keepLayered, testCase.alpha, testCase.keepLayered)
		}
	}
}
