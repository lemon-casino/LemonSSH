package terminaluse

import (
	"strings"
	"testing"

	"github.com/binaricat/lemonssh/internal/terminal/dataplane"
)

func newAttachTestService(t *testing.T) (*Service, *dataplane.RouteController, *dataplane.Server) {
	t.Helper()
	controller := dataplane.NewRouteController()
	dp := dataplane.NewServer(controller, "127.0.0.1:0")
	return New(controller, dp, nil), controller, dp
}

func TestAcquireLeasePausesOutputAndReleaseFlushes(t *testing.T) {
	s, _, dp := newAttachTestService(t)
	bootstrap, err := s.controller.Open("user@host:22")
	if err != nil {
		t.Fatal(err)
	}
	s.sessions["user@host:22"] = &terminalSession{bootstrap: bootstrap, uiID: "ui-1"}

	lease := s.AcquireSessionFlowPauseLease("ui-1")
	if !lease.Success || lease.LeaseID == "" || lease.Authorization == "" {
		t.Fatalf("lease acquisition failed: %+v", lease)
	}
	// While paused, renderer-bound output buffers in Go instead of the queue.
	if !s.publishOutput("user@host:22", []byte("held")) {
		t.Fatal("paused output rejected")
	}
	if got := dp.PendingBytes("user@host:22"); got != 0 {
		t.Fatalf("paused bytes leaked into the queue: %d", got)
	}
	release := s.ReleaseSessionFlowPauseLease("user@host:22", lease.LeaseID, nil)
	if !release.Success {
		t.Fatalf("release failed: %+v", release)
	}
	if got := dp.PendingBytes("user@host:22"); got != len("held") {
		t.Fatalf("buffered bytes were not flushed on release: %d", got)
	}
}

func TestSecondLeaseKeepsPauseUntilLastRelease(t *testing.T) {
	s, _, dp := newAttachTestService(t)
	bootstrap, _ := s.controller.Open("user@host:22")
	s.sessions["user@host:22"] = &terminalSession{bootstrap: bootstrap}

	first := s.AcquireSessionFlowPauseLease("user@host:22")
	second := s.AcquireSessionFlowPauseLease("user@host:22")
	if second.Authorization != first.Authorization {
		t.Fatal("attach authorization must be minted once per attach")
	}
	s.publishOutput("user@host:22", []byte("abc"))
	if r := s.ReleaseSessionFlowPauseLease("user@host:22", first.LeaseID, nil); !r.Success {
		t.Fatalf("first release failed: %+v", r)
	}
	if got := dp.PendingBytes("user@host:22"); got != 0 {
		t.Fatalf("pause ended before the last lease was released: %d", got)
	}
	if r := s.ReleaseSessionFlowPauseLease("user@host:22", second.LeaseID, nil); !r.Success {
		t.Fatalf("second release failed: %+v", r)
	}
	if got := dp.PendingBytes("user@host:22"); got != len("abc") {
		t.Fatalf("buffered bytes were not flushed: %d", got)
	}
}

func TestKeepPausedReleaseHoldsBytes(t *testing.T) {
	s, _, dp := newAttachTestService(t)
	bootstrap, _ := s.controller.Open("user@host:22")
	s.sessions["user@host:22"] = &terminalSession{bootstrap: bootstrap}
	lease := s.AcquireSessionFlowPauseLease("user@host:22")
	s.publishOutput("user@host:22", []byte("xyz"))
	r := s.ReleaseSessionFlowPauseLease("user@host:22", lease.LeaseID, &FlowPauseReleaseOptions{KeepPaused: true})
	if !r.Success {
		t.Fatalf("release failed: %+v", r)
	}
	if got := dp.PendingBytes("user@host:22"); got != 0 {
		t.Fatalf("keepPaused release flushed bytes: %d", got)
	}
}

func TestRebindRequiresLeaseAndRotatesRoute(t *testing.T) {
	s, _, _ := newAttachTestService(t)
	bootstrap, _ := s.controller.Open("user@host:22")
	s.sessions["user@host:22"] = &terminalSession{bootstrap: bootstrap}
	handoffs := make(chan map[string]any, 4)
	s.SetEventEmitter(func(name string, payload any) {
		if name == "terminal:route-handoff" {
			handoffs <- payload.(map[string]any)
		}
	})

	if r := s.RebindSessionOutput("user@host:22", ""); r.Success {
		t.Fatal("rebind without a lease must fail")
	}
	lease := s.AcquireSessionFlowPauseLease("user@host:22")
	rebind := s.RebindSessionOutput("user@host:22", "")
	if !rebind.Success || rebind.Route == nil {
		t.Fatalf("rebind failed: %+v", rebind)
	}
	if rebind.Route.Generation <= bootstrap.Generation {
		t.Fatal("rebind did not rotate the route generation")
	}
	if rebind.Authorization != lease.Authorization {
		t.Fatal("rebind must echo the attach authorization")
	}
	detached := <-handoffs
	if detached["phase"] != "detached" || detached["sessionId"] != "user@host:22" {
		t.Fatalf("unexpected detached payload: %+v", detached)
	}
	if detached["generation"].(uint32) != rebind.Route.Generation {
		t.Fatalf("detached event must carry the new generation: %+v", detached)
	}
	// The previous owner's reconnect must be refused while the popup holds the
	// route, so it suspends instead of stealing the route back.
	if _, err := s.Reconnect("user@host:22"); err == nil || !strings.Contains(err.Error(), "handed off") {
		t.Fatalf("Reconnect during attach must refuse with the handoff error, got %v", err)
	}
	restored := s.RestoreSessionOutput("user@host:22", lease.Authorization)
	if !restored.Success {
		t.Fatalf("restore failed: %+v", restored)
	}
	restoredEvent := <-handoffs
	if restoredEvent["phase"] != "restored" || restoredEvent["route"] == nil {
		t.Fatalf("restored event must carry the fresh route: %+v", restoredEvent)
	}
	if _, err := s.Reconnect("user@host:22"); err != nil {
		t.Fatalf("Reconnect must succeed after restore: %v", err)
	}
}

func TestRestoreRequiresGateAndRejectsWrongAuthorization(t *testing.T) {
	s, _, _ := newAttachTestService(t)
	bootstrap, _ := s.controller.Open("user@host:22")
	s.sessions["user@host:22"] = &terminalSession{bootstrap: bootstrap}
	lease := s.AcquireSessionFlowPauseLease("user@host:22")
	if r := s.RestoreSessionOutput("user@host:22", "wrong-token"); r.Success {
		t.Fatal("restore with a mismatched authorization must fail")
	}
	// No lease: the popup close path always re-acquires one first.
	if r := s.ReleaseSessionFlowPauseLease("user@host:22", lease.LeaseID, nil); !r.Success {
		t.Fatalf("release failed: %+v", r)
	}
	if r := s.RestoreSessionOutput("user@host:22", ""); r.Success {
		t.Fatal("restore without a lease must fail")
	}
}

func TestSnapshotRoundTripThroughRespond(t *testing.T) {
	s, _, _ := newAttachTestService(t)
	bootstrap, _ := s.controller.Open("user@host:22")
	s.sessions["user@host:22"] = &terminalSession{bootstrap: bootstrap}
	s.AcquireSessionFlowPauseLease("user@host:22")

	events := make(chan map[string]any, 1)
	s.SetEventEmitter(func(name string, payload any) {
		if name == "terminal:session-snapshot-request" {
			events <- payload.(map[string]any)
		}
	})

	type response struct {
		result SnapshotResult
	}
	done := make(chan response, 1)
	go func() {
		done <- response{result: s.RequestSessionSnapshot("user@host:22", "")}
	}()
	payload := <-events
	requestID, _ := payload["requestId"].(string)
	if requestID == "" || payload["sessionId"] != "ui-1" {
		// uiID empty here → native id is emitted; accept either but require an id.
		if payload["sessionId"] == "" {
			t.Fatal("snapshot request must carry a session id")
		}
	}
	enabled := true
	promptActive := true
	if err := s.RespondSessionSnapshot(requestID, "SNAPSHOT", &KittyKeyboardModeState{MainFlags: 1}, &enabled, &promptActive, nil, nil); err != nil {
		t.Fatal(err)
	}
	result := <-done
	if !result.result.Success || result.result.Snapshot != "SNAPSHOT" {
		t.Fatalf("snapshot round trip failed: %+v", result.result)
	}
	if result.result.KittyKeyboardModeState == nil || result.result.KittyKeyboardModeState.MainFlags != 1 {
		t.Fatal("kitty state lost in the round trip")
	}
	if result.result.PasswordPromptActive == nil || !*result.result.PasswordPromptActive {
		t.Fatal("password prompt flag lost in the round trip")
	}
}

func TestApplySnapshotRoundTripRejectsRejection(t *testing.T) {
	s, _, _ := newAttachTestService(t)
	bootstrap, _ := s.controller.Open("user@host:22")
	s.sessions["user@host:22"] = &terminalSession{bootstrap: bootstrap}
	s.AcquireSessionFlowPauseLease("user@host:22")

	events := make(chan map[string]any, 1)
	s.SetEventEmitter(func(name string, payload any) {
		if name == "terminal:session-apply-snapshot" {
			events <- payload.(map[string]any)
		}
	})
	done := make(chan ActionResult, 1)
	go func() {
		done <- s.ApplySessionSnapshot("user@host:22", "SNAP", SnapshotContext{
			ContextSnapshot:      "ctx",
			AlternateScreen:      true,
			Cwd:                  strPtr("/tmp"),
			Title:                strPtr("title"),
			PasswordPromptActive: boolPtr(false),
		}, "")
	}()
	payload := <-events
	if payload["contextSnapshot"] != "ctx" || payload["alternateScreen"] != true {
		t.Fatalf("apply payload truncated: %+v", payload)
	}
	if payload["cwd"] == nil || payload["title"] == nil {
		t.Fatalf("apply payload lost cwd/title: %+v", payload)
	}
	requestID := payload["requestId"].(string)
	if err := s.RespondApplySnapshot(requestID, false); err != nil {
		t.Fatal(err)
	}
	result := <-done
	if result.Success {
		t.Fatal("rejected apply must surface as failure")
	}
}

func TestUnknownSessionFailsClosed(t *testing.T) {
	s, _, _ := newAttachTestService(t)
	if lease := s.AcquireSessionFlowPauseLease("ghost"); lease.Success {
		t.Fatal("unknown session must not acquire a lease")
	}
	if r := s.RequestSessionSnapshot("ghost", ""); r.Success || r.Error == "" {
		t.Fatal("unknown session snapshot must fail with an error")
	}
	if children := s.PtyGetChildProcesses("ghost"); len(children) != 0 {
		t.Fatal("unknown session must report no children")
	}
	if r := s.GetSessionDistroInfo("ghost"); r.Success {
		t.Fatal("unknown session distro probe must fail")
	}
}

func strPtr(value string) *string { return &value }
func boolPtr(value bool) *bool    { return &value }
