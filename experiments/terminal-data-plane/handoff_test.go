package main

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeWindowHost struct {
	mu          sync.Mutex
	openedName  string
	openedPath  string
	closedNames []string
	openErr     error
	onClosed    map[string]func()
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Advance(duration time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(duration)
	f.mu.Unlock()
}

func (f *fakeWindowHost) OpenPopup(name, path string, onClosed func()) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.openedName = name
	f.openedPath = path
	if f.openErr != nil {
		return f.openErr
	}
	if f.onClosed == nil {
		f.onClosed = make(map[string]func())
	}
	f.onClosed[name] = onClosed
	return nil
}

func (f *fakeWindowHost) CloseWindow(name string) error {
	f.mu.Lock()
	f.closedNames = append(f.closedNames, name)
	onClosed := f.onClosed[name]
	delete(f.onClosed, name)
	f.mu.Unlock()
	if onClosed != nil {
		onClosed()
	}
	return nil
}

func (f *fakeWindowHost) simulatePopupClose(name string) {
	f.mu.Lock()
	onClosed := f.onClosed[name]
	delete(f.onClosed, name)
	f.mu.Unlock()
	if onClosed != nil {
		onClosed()
	}
}

func (f *fakeWindowHost) lastClosed() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.closedNames) == 0 {
		return ""
	}
	return f.closedNames[len(f.closedNames)-1]
}

func (f *fakeWindowHost) closed(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, closedName := range f.closedNames {
		if closedName == name {
			return true
		}
	}
	return false
}

func (f *fakeWindowHost) closedSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.closedNames...)
}

func prepareDrainedHandoffSession(t *testing.T, service *ProbeService) SessionInfo {
	t.Helper()
	session, route := createRoute(t, service, WorkloadMetadataOnly, WorkloadOptions{MetadataFrames: 1})
	service.mu.Lock()
	active := service.sessions[session.SessionID]
	active.route.drainReady = true
	active.route.drainTarget = 0
	active.applied = 0
	active.generation = route.Generation
	service.mu.Unlock()
	return session
}

func confirmRecoveryForTest(t *testing.T, service *ProbeService, info PopupHandoffInfo, result PopupAbortResult) {
	t.Helper()
	data := mustDialRoute(t, service, result.Bootstrap.DataURL, result.Bootstrap.DataToken)
	_ = mustDialRoute(t, service, result.Bootstrap.UrgentURL, result.Bootstrap.UrgentToken)
	sendInitialCredit(t, data, result.Bootstrap, 0)
	waitFor(t, "recovery route did not bind both sockets and credit", func() bool {
		service.mu.Lock()
		defer service.mu.Unlock()
		session := service.sessions[result.Bootstrap.SessionID]
		return session != nil && session.route != nil && session.route.dataConn != nil &&
			session.route.urgentConn != nil && session.route.creditReady
	})
	if err := service.ConfirmPopupRecovery(
		info.HandoffID, info.CancelToken, result.ConfirmationToken, result.Bootstrap.Generation,
	); err != nil {
		t.Fatal(err)
	}
}

func TestPopupHandoffIsOpaqueOneUseAndCompletionClosesSource(t *testing.T) {
	service := newTestService(t)
	host := &fakeWindowHost{}
	service.mu.Lock()
	service.windowHost = host
	service.mu.Unlock()
	session := prepareDrainedHandoffSession(t, service)

	info, err := service.CreatePopupHandoff(session.SessionID, 0, `{"bounded":"state"}`)
	if err != nil {
		t.Fatal(err)
	}
	if metrics := metricsFor(t, service, session.SessionID); metrics.RouteInterruptionCount != 0 {
		t.Fatalf("popup creation duplicated backend interruption count: %#v", metrics)
	}
	if info.HandoffID == "" || host.openedName != info.PopupWindowName || !strings.Contains(host.openedPath, "handoff=") {
		t.Fatalf("popup was not opened with opaque handoff identity: %#v / %#v", info, host)
	}
	if strings.Contains(host.openedPath, session.SessionID) || strings.Contains(host.openedPath, "route.") {
		t.Fatalf("popup URL leaked session or route credentials: %s", host.openedPath)
	}
	claimed, err := service.ClaimPopupHandoff(info.HandoffID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.SessionID != session.SessionID || claimed.LastApplied != 0 || claimed.ClientState != `{"bounded":"state"}` {
		t.Fatalf("claimed handoff mismatch: %#v", claimed)
	}
	if _, err := service.ClaimPopupHandoff(info.HandoffID); err == nil {
		t.Fatal("popup handoff was claimed twice")
	}
	if err := service.CompletePopupHandoff(info.HandoffID, "wrong"); err == nil {
		t.Fatal("wrong completion token closed the source window")
	}
	if host.lastClosed() != "" {
		t.Fatalf("source closed before valid completion: %s", host.lastClosed())
	}
	if _, err := service.ResumePopupHandoff(info.HandoffID, claimed.CompletionToken, 0); err != nil {
		t.Fatal(err)
	}
	if err := service.CompletePopupHandoff(info.HandoffID, claimed.CompletionToken); err != nil {
		t.Fatal(err)
	}
	if host.lastClosed() != mainWindowName {
		t.Fatalf("closed window %q, want %q", host.lastClosed(), mainWindowName)
	}
	if metrics := metricsFor(t, service, session.SessionID); metrics.RouteInterruptionCount != 1 {
		t.Fatalf("popup route replacement count = %d, want 1", metrics.RouteInterruptionCount)
	}
}

func TestPopupHandoffExpiryBoundsAndOpenFailure(t *testing.T) {
	service := newTestService(t)
	host := &fakeWindowHost{}
	clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	service.mu.Lock()
	service.windowHost = host
	service.now = clock.Now
	service.mu.Unlock()
	session := prepareDrainedHandoffSession(t, service)

	if _, err := service.CreatePopupHandoff(session.SessionID, 0, strings.Repeat("x", maxHandoffStateBytes+1)); err == nil {
		t.Fatal("oversized client state was accepted")
	}
	info, err := service.CreatePopupHandoff(session.SessionID, 0, "state")
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(popupHandoffTTL)
	service.expirePopupHandoffs()
	if _, err := service.ClaimPopupHandoff(info.HandoffID); err == nil {
		t.Fatal("expired popup handoff was claimed")
	}
	if !host.closed(info.PopupWindowName) || host.closed(mainWindowName) {
		t.Fatalf("unclaimed expiry cleanup closed wrong windows: %#v", host.closedSnapshot())
	}

	claimedInfo, err := service.CreatePopupHandoff(session.SessionID, 0, "state")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ClaimPopupHandoff(claimedInfo.HandoffID); err != nil {
		t.Fatal(err)
	}
	clock.Advance(popupHandoffTTL)
	service.expirePopupHandoffs()
	if !host.closed(claimedInfo.PopupWindowName) || host.closed(mainWindowName) {
		t.Fatalf("claimed expiry cleanup closed wrong windows: %#v", host.closedSnapshot())
	}

	host.openErr = errors.New("window creation failed")
	if _, err := service.CreatePopupHandoff(session.SessionID, 0, "state"); err == nil {
		t.Fatal("window creation failure was hidden")
	}
	host.openErr = nil
	for index := 0; index < maxPopupHandoffs; index++ {
		if _, err := service.CreatePopupHandoff(session.SessionID, 0, "state"); err != nil {
			t.Fatalf("create bounded handoff %d: %v", index, err)
		}
	}
	if _, err := service.CreatePopupHandoff(session.SessionID, 0, "state"); err == nil {
		t.Fatal("popup handoff map exceeded its bound")
	}
	service.mu.Lock()
	handoffCount := len(service.handoffs)
	service.mu.Unlock()
	if handoffCount != maxPopupHandoffs {
		t.Fatalf("handoff map retained %d records, want %d", handoffCount, maxPopupHandoffs)
	}
	clock.Advance(popupHandoffTTL)
	service.expirePopupHandoffs()
	service.mu.Lock()
	handoffCount = len(service.handoffs)
	service.mu.Unlock()
	if handoffCount != 0 {
		t.Fatalf("expired handoffs retained %d capacity slots", handoffCount)
	}
}

func TestPopupAbortRecoversUnclaimedClaimedAndReplacedRoutes(t *testing.T) {
	states := []struct {
		name    string
		claim   bool
		replace bool
	}{
		{name: "unclaimed"},
		{name: "claimed-before-route", claim: true},
		{name: "claimed-route-replaced", claim: true, replace: true},
	}
	for _, state := range states {
		t.Run(state.name, func(t *testing.T) {
			service := newTestService(t)
			host := &fakeWindowHost{}
			service.mu.Lock()
			service.windowHost = host
			service.mu.Unlock()
			session := prepareDrainedHandoffSession(t, service)
			info, err := service.CreatePopupHandoff(session.SessionID, 0, "state")
			if err != nil {
				t.Fatal(err)
			}
			var claimed ClaimedPopupHandoff
			if state.claim {
				claimed, err = service.ClaimPopupHandoff(info.HandoffID)
				if err != nil {
					t.Fatal(err)
				}
			}
			if state.replace {
				if _, err := service.ResumePopupHandoff(info.HandoffID, claimed.CompletionToken, 0); err != nil {
					t.Fatal(err)
				}
				service.mu.Lock()
				pending := service.sessions[session.SessionID].route.handoffPending
				service.mu.Unlock()
				if !pending {
					t.Fatal("popup replacement route was not frozen before host completion")
				}
			}
			result, err := service.AbortPopupHandoff(info.HandoffID, info.CancelToken)
			if err != nil {
				t.Fatal(err)
			}
			if result.Bootstrap.Generation < 2 || host.closed(mainWindowName) || !host.closed(info.PopupWindowName) {
				t.Fatalf("abort recovery mismatch: %#v closed=%#v", result, host.closedSnapshot())
			}
			confirmRecoveryForTest(t, service, info, result)
			service.mu.Lock()
			recovered := service.sessions[session.SessionID]
			if recovered.route == nil || recovered.route.handoffPending || recovered.generation != result.Bootstrap.Generation {
				service.mu.Unlock()
				t.Fatal("source did not receive a fresh unfrozen generation")
			}
			service.mu.Unlock()
		})
	}
}

func TestPopupEarlyCloseAndExpiryLeaveBoundedRecoveryGrant(t *testing.T) {
	service := newTestService(t)
	host := &fakeWindowHost{}
	clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	service.mu.Lock()
	service.windowHost = host
	service.now = clock.Now
	service.mu.Unlock()
	session := prepareDrainedHandoffSession(t, service)

	info, err := service.CreatePopupHandoff(session.SessionID, 0, "state")
	if err != nil {
		t.Fatal(err)
	}
	host.simulatePopupClose(info.PopupWindowName)
	status, err := service.PopupHandoffStatus(info.HandoffID, info.CancelToken)
	if err != nil || status.State != "aborted" {
		t.Fatalf("early close status = %#v, %v", status, err)
	}
	recovered, err := service.AbortPopupHandoff(info.HandoffID, info.CancelToken)
	if err != nil {
		t.Fatal(err)
	}
	confirmRecoveryForTest(t, service, info, recovered)
	service.mu.Lock()
	service.sessions[session.SessionID].route.drainReady = true
	service.sessions[session.SessionID].route.drainTarget = 0
	service.mu.Unlock()

	expiring, err := service.CreatePopupHandoff(session.SessionID, 0, "state")
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(popupHandoffTTL)
	service.expirePopupHandoffs()
	status, err = service.PopupHandoffStatus(expiring.HandoffID, expiring.CancelToken)
	if err != nil || status.State != "aborted" {
		t.Fatalf("expiry recovery status = %#v, %v", status, err)
	}
	service.mu.Lock()
	recoveryCount := len(service.popupRecoveries)
	service.mu.Unlock()
	if recoveryCount > maxPopupHandoffs || host.closed(mainWindowName) {
		t.Fatalf("recovery grants unbounded or source closed: count=%d closed=%#v", recoveryCount, host.closedSnapshot())
	}
}

func TestRejectedClaimedPopupBecomesSourceRecovery(t *testing.T) {
	service := newTestService(t)
	host := &fakeWindowHost{}
	service.mu.Lock()
	service.windowHost = host
	service.mu.Unlock()
	session := prepareDrainedHandoffSession(t, service)
	info, err := service.CreatePopupHandoff(session.SessionID, 0, "state")
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := service.ClaimPopupHandoff(info.HandoffID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ResumePopupHandoff(info.HandoffID, claimed.CompletionToken, 0); err != nil {
		t.Fatal(err)
	}
	if err := service.RejectPopupHandoff(info.HandoffID, claimed.CompletionToken); err != nil {
		t.Fatal(err)
	}
	status, err := service.PopupHandoffStatus(info.HandoffID, info.CancelToken)
	if err != nil || status.State != "aborted" || host.closed(mainWindowName) {
		t.Fatalf("rejected popup recovery state = %#v, %v closed=%#v", status, err, host.closedSnapshot())
	}
	recovered, err := service.AbortPopupHandoff(info.HandoffID, info.CancelToken)
	if err != nil {
		t.Fatal(err)
	}
	confirmRecoveryForTest(t, service, info, recovered)
}

func TestShutdownStopsOwnedHandoffCleanupLoop(t *testing.T) {
	service := newTestService(t)
	host := &fakeWindowHost{}
	service.mu.Lock()
	service.windowHost = host
	service.handoffTTL = 10 * time.Millisecond
	service.mu.Unlock()
	session := prepareDrainedHandoffSession(t, service)
	info, err := service.CreatePopupHandoff(session.SessionID, 0, "state")
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "owned cleanup loop did not expire popup", func() bool {
		return host.closed(info.PopupWindowName)
	})
	if err := service.shutdown(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-service.handoffLoopDone:
	default:
		t.Fatal("handoff cleanup loop remained active after shutdown")
	}
}

func TestPopupResumeAndAbortSerializeToOneRecoverableGeneration(t *testing.T) {
	service := newTestService(t)
	host := &fakeWindowHost{}
	service.mu.Lock()
	service.windowHost = host
	service.mu.Unlock()
	session := prepareDrainedHandoffSession(t, service)
	info, err := service.CreatePopupHandoff(session.SessionID, 0, "state")
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := service.ClaimPopupHandoff(info.HandoffID)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	var popupBootstrap RouteBootstrap
	var popupErr error
	var abortResult PopupAbortResult
	var abortErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		popupBootstrap, popupErr = service.ResumePopupHandoff(info.HandoffID, claimed.CompletionToken, 0)
	}()
	go func() {
		defer wg.Done()
		<-start
		abortResult, abortErr = service.AbortPopupHandoff(info.HandoffID, info.CancelToken)
	}()
	close(start)
	wg.Wait()
	if abortErr != nil {
		t.Fatalf("source abort lost race: %v (popup=%#v, %v)", abortErr, popupBootstrap, popupErr)
	}
	confirmRecoveryForTest(t, service, info, abortResult)
	service.mu.Lock()
	current := service.sessions[session.SessionID]
	currentGeneration := current.generation
	pending := current.route != nil && current.route.handoffPending
	service.mu.Unlock()
	if currentGeneration != abortResult.Bootstrap.Generation || pending {
		t.Fatalf("current generation=%d pending=%t abort=%#v popup=%#v/%v", currentGeneration, pending, abortResult, popupBootstrap, popupErr)
	}
}

func TestPopupResumeAndExpiryRemainSourceRecoverable(t *testing.T) {
	service := newTestService(t)
	host := &fakeWindowHost{}
	clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	service.mu.Lock()
	service.windowHost = host
	service.now = clock.Now
	service.mu.Unlock()
	session := prepareDrainedHandoffSession(t, service)
	info, err := service.CreatePopupHandoff(session.SessionID, 0, "state")
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := service.ClaimPopupHandoff(info.HandoffID)
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(popupHandoffTTL)

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, _ = service.ResumePopupHandoff(info.HandoffID, claimed.CompletionToken, 0)
	}()
	go func() {
		defer wg.Done()
		<-start
		service.expirePopupHandoffs()
	}()
	close(start)
	wg.Wait()
	abortResult, err := service.AbortPopupHandoff(info.HandoffID, info.CancelToken)
	if err != nil {
		t.Fatal(err)
	}
	confirmRecoveryForTest(t, service, info, abortResult)
	service.mu.Lock()
	current := service.sessions[session.SessionID]
	currentGeneration := current.generation
	pending := current.route != nil && current.route.handoffPending
	service.mu.Unlock()
	if currentGeneration != abortResult.Bootstrap.Generation || pending || host.closed(mainWindowName) {
		t.Fatalf("expiry recovery generation=%d pending=%t result=%#v closed=%#v", currentGeneration, pending, abortResult, host.closedSnapshot())
	}
}

func TestPopupRecoveryBootstrapCanRetryUntilConfirmed(t *testing.T) {
	service := newTestService(t)
	host := &fakeWindowHost{}
	service.mu.Lock()
	service.windowHost = host
	service.mu.Unlock()
	session := prepareDrainedHandoffSession(t, service)
	info, err := service.CreatePopupHandoff(session.SessionID, 0, "state")
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.AbortPopupHandoff(info.HandoffID, info.CancelToken)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.AbortPopupHandoff(info.HandoffID, info.CancelToken)
	if err != nil {
		t.Fatal(err)
	}
	if second.Bootstrap.Generation <= first.Bootstrap.Generation || second.ConfirmationToken == first.ConfirmationToken {
		t.Fatalf("retry did not rotate recovery generation/token: first=%#v second=%#v", first, second)
	}
	if err := service.ConfirmPopupRecovery(
		info.HandoffID, info.CancelToken, first.ConfirmationToken, first.Bootstrap.Generation,
	); err == nil {
		t.Fatal("stale recovery bootstrap was confirmed")
	}
	confirmRecoveryForTest(t, service, info, second)
	service.mu.Lock()
	_, retained := service.popupRecoveries[info.HandoffID]
	pending := service.sessions[session.SessionID].route.handoffPending
	service.mu.Unlock()
	if retained || pending {
		t.Fatal("confirmed recovery retained grant or output freeze")
	}
}
