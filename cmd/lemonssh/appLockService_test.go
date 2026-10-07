package main

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/lemon-casino/lemonssh/internal/platform/applock"
	"github.com/lemon-casino/lemonssh/internal/profile/store"
)

type recordedEvent struct {
	name    string
	payload any
}

func newEventRecorder() (func(name string, payload any), *[]recordedEvent) {
	events := &[]recordedEvent{}
	return func(name string, payload any) {
		*events = append(*events, recordedEvent{name: name, payload: payload})
	}, events
}

func eventsByName(events *[]recordedEvent, name string) []recordedEvent {
	var matched []recordedEvent
	for _, event := range *events {
		if event.name == name {
			matched = append(matched, event)
		}
	}
	return matched
}

// stopAppLockIdleTimer disarms any live AfterFunc so a test never leaks a
// timer that could lock a service instance after the test finished.
func stopAppLockIdleTimer(s *AppLockService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopIdleTimerLocked()
}

func newTestAppLockService(t *testing.T, db *store.Store) *AppLockService {
	t.Helper()
	s := newAppLockServiceWithDeps(applock.New(&memoryCredentials{blobs: map[string][]byte{}}), db)
	t.Cleanup(func() { stopAppLockIdleTimer(s) })
	return s
}

func TestSetTimeoutMinutesPersistsAndReloads(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "profile.db")
	db, err := store.Open(dbPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := newTestAppLockService(t, db)
	if got := s.GetSettings()["timeoutMinutes"]; got != appLockDefaultTimeoutMinutes {
		t.Fatalf("default timeout = %v, want %v", got, appLockDefaultTimeoutMinutes)
	}
	if _, err := s.SetTimeoutMinutes(5); err != nil {
		t.Fatal(err)
	}
	if got := s.GetSettings()["timeoutMinutes"]; got != 5 {
		t.Fatalf("timeout after set = %v, want 5", got)
	}
	db.Close()

	// A fresh process must load the persisted timeout, not the default.
	reopened, err := store.Open(dbPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reloaded := newTestAppLockService(t, reopened)
	if got := reloaded.GetSettings()["timeoutMinutes"]; got != 5 {
		t.Fatalf("persisted timeout = %v, want 5", got)
	}
}

func TestSetTimeoutMinutesRejectsInvalidValues(t *testing.T) {
	s := newTestAppLockService(t, nil)
	for _, invalid := range []int{-1, appLockMaxTimeoutMinutes + 1} {
		if _, err := s.SetTimeoutMinutes(invalid); err == nil {
			t.Fatalf("SetTimeoutMinutes(%d) accepted", invalid)
		}
	}
	if got := s.GetSettings()["timeoutMinutes"]; got != appLockDefaultTimeoutMinutes {
		t.Fatalf("timeout after invalid set = %v, want default", got)
	}
}

func TestSetTimeoutMinutesBroadcastsSettingsChanged(t *testing.T) {
	s := newTestAppLockService(t, nil)
	emit, events := newEventRecorder()
	s.setEventEmitter(emit)
	if _, err := s.SetTimeoutMinutes(1); err != nil {
		t.Fatal(err)
	}
	broadcasts := eventsByName(events, appLockSettingsChangedEvent)
	if len(broadcasts) != 1 {
		t.Fatalf("settings broadcasts = %d, want 1", len(broadcasts))
	}
	payload, ok := broadcasts[0].payload.(map[string]any)
	if !ok || payload["timeoutMinutes"] != 1 {
		t.Fatalf("broadcast payload missing timeoutMinutes=1: %#v", broadcasts[0].payload)
	}
}

func TestSetTimeoutMinutesReschedulesIdleTimer(t *testing.T) {
	s := newTestAppLockService(t, nil)
	fixed := time.UnixMilli(1_000_000)
	s.now = func() time.Time { return fixed }

	s.mu.Lock()
	armed := s.timeoutMinutes > 0 && s.idleTimer != nil
	s.mu.Unlock()
	if armed {
		t.Fatal("idle timer armed without a configured verifier")
	}

	if _, err := s.Enable("test-password"); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	if s.idleTimer != nil {
		s.mu.Unlock()
		t.Fatal("idle timer armed while locked")
	}
	s.mu.Unlock()

	// Disabling the timeout must keep the timer unarmed after unlock.
	if _, err := s.SetTimeoutMinutes(0); err != nil {
		t.Fatal(err)
	}
	if err := s.Unlock("test-password"); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	if s.idleTimer != nil {
		s.mu.Unlock()
		t.Fatal("idle timer armed with timeout disabled")
	}
	s.mu.Unlock()

	// Re-enabling reschedules from the unlock anchor.
	if _, err := s.SetTimeoutMinutes(1); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	armed = s.idleTimer != nil
	s.mu.Unlock()
	if !armed {
		t.Fatal("idle timer not armed after enabling timeout while unlocked")
	}
}

func TestIdleExpiryLocksWithIdleReasonAndBroadcasts(t *testing.T) {
	s := newTestAppLockService(t, nil)
	emit, events := newEventRecorder()
	s.setEventEmitter(emit)
	current := time.UnixMilli(5_000_000)
	s.now = func() time.Time { return current }

	if _, err := s.Enable("test-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetTimeoutMinutes(5); err != nil {
		t.Fatal(err)
	}
	if err := s.Unlock("test-password"); err != nil {
		t.Fatal(err)
	}
	current = current.Add(2 * time.Minute)
	s.ReportActivity()

	// One second before the deadline an idle lock request must be refused.
	current = current.Add(5*time.Minute - time.Second)
	if state := s.SetRuntimeLocked(appLockReasonIdle); state.Locked {
		t.Fatal("idle lock honored before the timeout elapsed")
	}

	// At the deadline the Go timer locks with the idle reason.
	current = current.Add(time.Second)
	s.handleIdleTimerExpired()
	state := s.GetRuntimeState()
	if !state.Locked || state.Reason == nil || *state.Reason != appLockReasonIdle {
		t.Fatalf("idle expiry state = locked=%v reason=%v", state.Locked, state.Reason)
	}
	if state.Version < 2 {
		t.Fatalf("idle lock must bump version, got %d", state.Version)
	}
	pushed := eventsByName(events, appLockRuntimeStateChangedEvent)
	if len(pushed) == 0 {
		t.Fatal("idle lock did not broadcast runtime state change")
	}
	last := pushed[len(pushed)-1].payload.(AppLockRuntimeState)
	if !last.Locked || last.Reason == nil || *last.Reason != appLockReasonIdle {
		t.Fatalf("broadcast state = %#v", last)
	}
}

func TestIdleExpiryWithRacingActivityRearmsInsteadOfLocking(t *testing.T) {
	s := newTestAppLockService(t, nil)
	current := time.UnixMilli(9_000_000)
	s.now = func() time.Time { return current }

	if _, err := s.Enable("test-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetTimeoutMinutes(5); err != nil {
		t.Fatal(err)
	}
	if err := s.Unlock("test-password"); err != nil {
		t.Fatal(err)
	}
	// Activity lands just before the deadline: the fired timer must re-arm
	// (never lock) and leave a live timer for the remaining window.
	current = current.Add(5*time.Minute - time.Second)
	s.ReportActivity()
	current = current.Add(time.Second)
	s.handleIdleTimerExpired()
	if state := s.GetRuntimeState(); state.Locked {
		t.Fatal("idle expiry locked despite racing activity")
	}
	s.mu.Lock()
	armed := s.idleTimer != nil
	s.mu.Unlock()
	if !armed {
		t.Fatal("idle timer not re-armed after racing activity")
	}
}

func TestUnlockAnchorsIdleClockToNow(t *testing.T) {
	s := newTestAppLockService(t, nil)
	current := time.UnixMilli(20_000_000)
	s.now = func() time.Time { return current }

	if _, err := s.Enable("test-password"); err != nil {
		t.Fatal(err)
	}
	current = current.Add(time.Hour)
	if err := s.Unlock("test-password"); err != nil {
		t.Fatal(err)
	}
	state := s.GetRuntimeState()
	if state.LastActivityAt == nil || *state.LastActivityAt != current.UnixMilli() {
		t.Fatalf("unlock LastActivityAt = %v, want %d", state.LastActivityAt, current.UnixMilli())
	}
}

func TestTimeoutSurvivesStoreRoundTripAsJSONNumber(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "profile.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := newTestAppLockService(t, db)
	if _, err := s.SetTimeoutMinutes(30); err != nil {
		t.Fatal(err)
	}
	raw, err := db.GetRaw(appLockDomain, appLockTimeoutKey)
	if err != nil {
		t.Fatal(err)
	}
	var persisted int
	if err := json.Unmarshal(raw, &persisted); err != nil || persisted != 30 {
		t.Fatalf("persisted timeout raw=%s err=%v", raw, err)
	}
}

func TestNotifyReopenBroadcastsEvent(t *testing.T) {
	s := newTestAppLockService(t, nil)
	emit, events := newEventRecorder()
	s.setEventEmitter(emit)
	s.NotifyReopen()
	reopens := eventsByName(events, appLockReopenEvent)
	if len(reopens) != 1 {
		t.Fatalf("reopen events = %d, want 1", len(reopens))
	}
}

// The renderer's domain/appLock.ts normalizes settings by validating the
// verifier salt (16 bytes) and hash (32 bytes) as base64; hex values from the
// Go store would normalize to "disabled" and silently disarm lock behaviors.
func TestGetSettingsVerifierUsesRendererBase64Contract(t *testing.T) {
	s := newTestAppLockService(t, nil)
	if _, err := s.Enable("test-password"); err != nil {
		t.Fatal(err)
	}
	if enabled, _ := s.GetSettings()["enabled"].(bool); !enabled {
		t.Fatal("settings must report enabled with a configured verifier")
	}
	verifier, ok := s.GetSettings()["passwordVerifier"].(map[string]any)
	if !ok {
		t.Fatal("passwordVerifier missing from settings")
	}
	salt, _ := verifier["salt"].(string)
	hash, _ := verifier["hash"].(string)
	saltBytes, err := base64.StdEncoding.DecodeString(salt)
	if err != nil || len(saltBytes) != 16 {
		t.Fatalf("salt %q is not base64 of 16 bytes (err=%v)", salt, err)
	}
	hashBytes, err := base64.StdEncoding.DecodeString(hash)
	if err != nil || len(hashBytes) != 32 {
		t.Fatalf("hash %q is not base64 of 32 bytes (err=%v)", hash, err)
	}
}
