package main

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/binaricat/lemonssh/internal/platform/applock"
	"github.com/binaricat/lemonssh/internal/platform/credentials"
	"github.com/binaricat/lemonssh/internal/profile/store"
)

type AppLockRuntimeState struct {
	Initialized    bool    `json:"initialized"`
	Locked         bool    `json:"locked"`
	Reason         *string `json:"reason"`
	Version        int     `json:"version"`
	LastLockedAt   *int64  `json:"lastLockedAt"`
	LastUnlockedAt *int64  `json:"lastUnlockedAt"`
	LastActivityAt *int64  `json:"lastActivityAt"`
}

// Wails event names pushed to every renderer window. The frontend facade in
// infrastructure/runtime/wails/wailsRuntimeClient.ts subscribes to these.
const (
	appLockRuntimeStateChangedEvent = "app-lock:runtime-state-changed"
	appLockSettingsChangedEvent     = "app-lock:settings-changed"
	appLockReopenEvent              = "app-lock:reopen"

	appLockReasonIdle = "idle"

	appLockTimeoutKey        = "app-lock-timeout"
	appLockMaxTimeoutMinutes = 1440
)

type AppLockService struct {
	mu                    sync.Mutex
	state                 AppLockRuntimeState
	core                  *applock.Service
	store                 *store.Store
	verifierPresent       bool
	verifier              applock.Verifier
	authenticateBiometric func() error
	biometricSettings     BiometricSettings
	emit                  func(name string, payload any)
	timeoutMinutes        int
	idleTimer             *time.Timer
	now                   func() time.Time
}

const appLockDomain = "settings"
const appLockKey = "app-lock-verifier"
const appLockDefaultTimeoutMinutes = 15

func newAppLockService() *AppLockService {
	return &AppLockService{
		state:          AppLockRuntimeState{Initialized: true, Locked: false, Version: 1},
		timeoutMinutes: appLockDefaultTimeoutMinutes,
		now:            time.Now,
	}
}

func newAppLockServiceWithDeps(core *applock.Service, profileStore *store.Store) *AppLockService {
	service := &AppLockService{
		state:                 AppLockRuntimeState{Initialized: true, Locked: false, Version: 1},
		core:                  core,
		store:                 profileStore,
		authenticateBiometric: applock.AuthenticateBiometric,
		timeoutMinutes:        appLockDefaultTimeoutMinutes,
		now:                   time.Now,
	}
	service.loadVerifier()
	if profileStore != nil {
		if raw, err := profileStore.GetRaw(appLockDomain, appLockKey); err == nil {
			_ = json.Unmarshal(raw, &service.biometricSettings)
		}
	}
	service.loadTimeout()
	return service
}

func (s *AppLockService) loadVerifier() {
	if s.store == nil {
		return
	}
	raw, err := s.store.GetRaw(appLockDomain, appLockKey)
	// A missing or empty verifier means app lock was never configured; an empty
	// value must not lock the app with an unusable verifier.
	if errors.Is(err, store.ErrNoSuchKey) || (err == nil && len(raw) == 0) {
		return
	}
	s.verifierPresent = true
	s.state.Locked = true
	reason := "password"
	s.state.Reason = &reason
	now := s.now().UnixMilli()
	s.state.LastLockedAt = &now
	if err != nil {
		return
	}
	var verifier applock.Verifier
	if err := json.Unmarshal(raw, &verifier); err == nil {
		s.verifier = verifier
	}
}

// loadTimeout restores the persisted idle-lock timeout. An absent or corrupt
// entry falls back to the default so a bad store value can never disable or
// invert the user's choice silently.
func (s *AppLockService) loadTimeout() {
	if s.store == nil {
		return
	}
	raw, err := s.store.GetRaw(appLockDomain, appLockTimeoutKey)
	if err != nil || len(raw) == 0 {
		return
	}
	var minutes int
	if json.Unmarshal(raw, &minutes) != nil || minutes < 0 || minutes > appLockMaxTimeoutMinutes {
		return
	}
	s.timeoutMinutes = minutes
}

func (s *AppLockService) setEventEmitter(emit func(name string, payload any)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emit = emit
}

func (s *AppLockService) GetRuntimeState() AppLockRuntimeState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *AppLockService) ReportActivity() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UnixMilli()
	s.state.LastActivityAt = &now
	s.restartIdleTimerLocked()
}

func (s *AppLockService) SetRuntimeLocked(reason string) AppLockRuntimeState {
	s.mu.Lock()
	now := s.now().UnixMilli()
	changed := false
	if reason == "" {
		if s.verifierPresent {
			state := s.state
			s.mu.Unlock()
			return state
		}
		s.state.Locked = false
		s.state.Reason = nil
		s.state.LastUnlockedAt = &now
		changed = true
	} else {
		// Renderer clocks and background timer throttling make remote idle
		// claims unreliable; only honor an idle lock that this process's own
		// activity clock confirms. Manual/background/startup stay unconditional.
		if reason == appLockReasonIdle && !s.idleDeadlinePassedLocked() {
			state := s.state
			s.mu.Unlock()
			return state
		}
		s.state.Locked = true
		s.state.Reason = &reason
		s.state.Version++
		s.state.LastLockedAt = &now
		s.stopIdleTimerLocked()
		changed = true
	}
	state := s.state
	s.mu.Unlock()
	if changed {
		s.emitRuntimeStateChanged()
	}
	return state
}

func (s *AppLockService) Enable(password string) (AppLockRuntimeState, error) {
	s.mu.Lock()
	if s.verifierPresent {
		state := s.state
		s.mu.Unlock()
		return state, fmt.Errorf("app lock already configured; authenticate before changing password")
	}
	if s.core == nil {
		state := s.state
		s.mu.Unlock()
		return state, applock.ErrNoVerifier
	}
	verifier, err := s.core.Enable(context.Background(), password)
	if err != nil {
		state := s.state
		s.mu.Unlock()
		return state, err
	}
	if s.store != nil {
		raw, marshalErr := json.Marshal(verifier)
		if marshalErr != nil {
			state := s.state
			s.mu.Unlock()
			return state, marshalErr
		}
		if err := s.store.SetRaw(appLockDomain, appLockKey, raw); err != nil {
			state := s.state
			s.mu.Unlock()
			return state, err
		}
	}
	s.verifier = verifier
	s.verifierPresent = true
	now := s.now().UnixMilli()
	s.state.Locked = true
	reason := "password"
	s.state.Reason = &reason
	s.state.Version++
	s.state.LastLockedAt = &now
	s.stopIdleTimerLocked()
	state := s.state
	s.mu.Unlock()
	s.emitRuntimeStateChanged()
	s.emitSettingsChanged()
	return state, nil
}

func (s *AppLockService) Unlock(password string) error {
	s.mu.Lock()
	if s.core == nil {
		s.mu.Unlock()
		return applock.ErrNoVerifier
	}
	if err := s.core.Verify(s.verifier, password); err != nil {
		s.mu.Unlock()
		return err
	}
	now := s.now().UnixMilli()
	s.state.Locked = false
	s.state.Reason = nil
	s.state.LastUnlockedAt = &now
	// Unlocking proves user presence: anchor the idle clock to now so the
	// watchdog does not immediately re-lock a stale idle window.
	s.state.LastActivityAt = &now
	s.restartIdleTimerLocked()
	s.mu.Unlock()
	s.emitRuntimeStateChanged()
	return nil
}

func (s *AppLockService) Disable(password string) error {
	s.mu.Lock()
	if s.core == nil {
		s.mu.Unlock()
		return applock.ErrNoVerifier
	}
	if err := s.core.Verify(s.verifier, password); err != nil {
		s.mu.Unlock()
		return err
	}
	if s.store != nil {
		if _, err := s.store.Write(store.WriteRequest{Mutations: []store.Mutation{
			{Domain: appLockDomain, Key: "app-lock-biometric", Delete: true},
			{Domain: appLockDomain, Key: appLockKey, Delete: true},
		}}); err != nil {
			s.mu.Unlock()
			return err
		}
	}
	now := s.now().UnixMilli()
	s.state.Locked = false
	s.state.Reason = nil
	s.state.LastUnlockedAt = &now
	s.state.LastActivityAt = &now
	s.verifierPresent = false
	s.biometricSettings = BiometricSettings{}
	s.verifier = applock.Verifier{}
	s.stopIdleTimerLocked()
	s.mu.Unlock()
	s.emitRuntimeStateChanged()
	s.emitSettingsChanged()
	return nil
}

// Reset verifies the current password and removes App Lock entirely (the
// password verifier plus system-unlock settings), unlocking the app. It backs
// the lock-screen recovery entry, so unlike the settings-driven flow it must
// succeed while locked. Error strings double as the renderer's typed mutation
// codes ("empty-current", "incorrect"), mirroring SetSystemUnlockEnabled.
func (s *AppLockService) Reset(password string) (AppLockRuntimeState, error) {
	if password == "" {
		return s.GetRuntimeState(), fmt.Errorf("empty-current")
	}
	if err := s.Disable(password); err != nil {
		state := s.GetRuntimeState()
		if errors.Is(err, applock.ErrPasswordRejected) {
			return state, fmt.Errorf("incorrect")
		}
		return state, err
	}
	return s.GetRuntimeState(), nil
}

// SetTimeoutMinutes persists the idle auto-lock timeout (0 disables idle
// locking) and reschedules the in-process idle timer.
func (s *AppLockService) SetTimeoutMinutes(minutes int) (map[string]any, error) {
	if minutes < 0 || minutes > appLockMaxTimeoutMinutes {
		return nil, fmt.Errorf("invalid app lock timeout: %d minutes", minutes)
	}
	s.mu.Lock()
	changed := s.timeoutMinutes != minutes
	if changed {
		s.timeoutMinutes = minutes
		if s.store != nil {
			raw, err := json.Marshal(minutes)
			if err != nil {
				s.mu.Unlock()
				return nil, err
			}
			if err := s.store.SetRaw(appLockDomain, appLockTimeoutKey, raw); err != nil {
				s.mu.Unlock()
				return nil, err
			}
		}
		s.restartIdleTimerLocked()
	}
	s.mu.Unlock()
	if changed {
		s.emitSettingsChanged()
	}
	return s.GetSettings(), nil
}

type BiometricUnlockResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func (s *AppLockService) UnlockWithBiometrics() BiometricUnlockResult {
	s.mu.Lock()
	if !s.biometricSettings.Enabled {
		s.mu.Unlock()
		return BiometricUnlockResult{Error: "disabled"}
	}
	if !s.state.Locked {
		s.mu.Unlock()
		return BiometricUnlockResult{Error: "not-locked"}
	}
	if s.core == nil || s.verifier.Digest == "" || s.authenticateBiometric == nil {
		s.mu.Unlock()
		return BiometricUnlockResult{Error: "biometric app lock owner or verifier unavailable"}
	}
	version, verifier, authenticate := s.state.Version, s.verifier, s.authenticateBiometric
	s.mu.Unlock()
	if err := authenticate(); err != nil {
		return BiometricUnlockResult{Error: err.Error()}
	}
	s.mu.Lock()
	if s.state.Version != version || s.verifier != verifier {
		s.mu.Unlock()
		return BiometricUnlockResult{Error: "app lock changed during biometric authentication; retry"}
	}
	now := s.now().UnixMilli()
	s.state.Locked = false
	s.state.Reason = nil
	s.state.LastUnlockedAt = &now
	s.state.LastActivityAt = &now
	s.restartIdleTimerLocked()
	s.mu.Unlock()
	s.emitRuntimeStateChanged()
	return BiometricUnlockResult{Success: true}
}

// NotifyReopen broadcasts a "window shown again" signal (tray, hotkey, second
// instance) so every renderer can resync lock state while locked.
func (s *AppLockService) NotifyReopen() {
	s.mu.Lock()
	emit := s.emit
	s.mu.Unlock()
	if emit != nil {
		emit(appLockReopenEvent, nil)
	}
}

// GetSettings returns the App Lock settings map consumed by the renderer.
func (s *AppLockService) GetSettings() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settingsLocked()
}

func (s *AppLockService) settingsLocked() map[string]any {
	var verifier any
	if s.verifier.Digest != "" {
		verifier = map[string]any{
			"version":    1,
			"algorithm":  "PBKDF2-SHA256",
			"iterations": 210000,
			// The renderer's AppLockSettings contract (domain/appLock.ts)
			// validates salt (16 bytes) and hash (32 bytes) as base64; the Go
			// verifier stores hex, so translate here at the boundary.
			"salt": hexToBase64(s.verifier.Salt),
			"hash": hexToBase64(s.verifier.Digest),
		}
	}
	return map[string]any{
		"enabled":                       verifier != nil,
		"timeoutMinutes":                s.timeoutMinutes,
		"systemUnlockEnabled":           s.biometricSettings.Enabled && verifier != nil,
		"systemUnlockAutoPromptEnabled": s.biometricSettings.AutoPromptEnabled && s.biometricSettings.Enabled && verifier != nil,
		"passwordVerifier":              verifier,
	}
}

func hexToBase64(value string) string {
	raw, err := hex.DecodeString(value)
	if err != nil {
		return value
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// lastActivityAnchorLocked is the timestamp the idle clock counts from: the
// last reported activity, falling back to the last unlock/lock so a freshly
// started process without any ReportActivity call still has a sane anchor.
func (s *AppLockService) lastActivityAnchorLocked() int64 {
	if s.state.LastActivityAt != nil {
		return *s.state.LastActivityAt
	}
	if s.state.LastUnlockedAt != nil {
		return *s.state.LastUnlockedAt
	}
	if s.state.LastLockedAt != nil {
		return *s.state.LastLockedAt
	}
	return s.now().UnixMilli()
}

func (s *AppLockService) idleDeadlinePassedLocked() bool {
	if !s.verifierPresent || s.state.Locked || s.timeoutMinutes <= 0 {
		return false
	}
	elapsed := s.now().Sub(time.UnixMilli(s.lastActivityAnchorLocked()))
	return elapsed >= time.Duration(s.timeoutMinutes)*time.Minute
}

func (s *AppLockService) stopIdleTimerLocked() {
	if s.idleTimer != nil {
		s.idleTimer.Stop()
		s.idleTimer = nil
	}
}

// restartIdleTimerLocked (re)arms the single idle timer from the current
// anchor. It is a no-op without a verifier, while already locked, or when the
// timeout is disabled (0).
func (s *AppLockService) restartIdleTimerLocked() {
	s.stopIdleTimerLocked()
	if !s.verifierPresent || s.state.Locked || s.timeoutMinutes <= 0 {
		return
	}
	timeout := time.Duration(s.timeoutMinutes) * time.Minute
	remaining := timeout - s.now().Sub(time.UnixMilli(s.lastActivityAnchorLocked()))
	if remaining < 0 {
		remaining = 0
	}
	s.idleTimer = time.AfterFunc(remaining, s.handleIdleTimerExpired)
}

// handleIdleTimerExpired locks the app with the "idle" reason once the
// configured timeout has really elapsed; if activity raced the expiry it
// re-arms for the remainder instead of locking an app still in use.
func (s *AppLockService) handleIdleTimerExpired() {
	s.mu.Lock()
	if !s.idleDeadlinePassedLocked() {
		s.restartIdleTimerLocked()
		s.mu.Unlock()
		return
	}
	s.idleTimer = nil
	reason := appLockReasonIdle
	now := s.now().UnixMilli()
	s.state.Locked = true
	s.state.Reason = &reason
	s.state.Version++
	s.state.LastLockedAt = &now
	s.mu.Unlock()
	s.emitRuntimeStateChanged()
}

func (s *AppLockService) emitRuntimeStateChanged() {
	s.mu.Lock()
	emit := s.emit
	state := s.state
	s.mu.Unlock()
	if emit != nil {
		emit(appLockRuntimeStateChangedEvent, state)
	}
}

func (s *AppLockService) emitSettingsChanged() {
	s.mu.Lock()
	emit := s.emit
	var settings map[string]any
	if emit != nil {
		settings = s.settingsLocked()
	}
	s.mu.Unlock()
	if emit != nil {
		emit(appLockSettingsChangedEvent, settings)
	}
}

func newAppLockServiceForTest(t interface{ Fatal(...any) }) *AppLockService {
	return newAppLockServiceWithDeps(applock.New(&memoryCredentials{blobs: map[string][]byte{}}), nil)
}

type memoryCredentials struct {
	mu    sync.Mutex
	blobs map[string][]byte
}

func (m *memoryCredentials) Seal(plaintext []byte, purpose string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.blobs == nil {
		m.blobs = map[string][]byte{}
	}
	copied := append([]byte(nil), plaintext...)
	m.blobs[purpose] = copied
	return append([]byte("sealed|"+purpose+"|"), copied...), nil
}

func (m *memoryCredentials) Open(envelope []byte, purpose string) ([]byte, error) {
	prefix := []byte("sealed|" + purpose + "|")
	if len(envelope) <= len(prefix) {
		return nil, credentials.ErrMalformedEnvelope
	}
	return envelope[len(prefix):], nil
}
