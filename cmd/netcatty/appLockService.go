package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/binaricat/netcatty/internal/platform/applock"
	"github.com/binaricat/netcatty/internal/platform/credentials"
	"github.com/binaricat/netcatty/internal/profile/store"
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

type AppLockService struct {
	mu                    sync.Mutex
	state                 AppLockRuntimeState
	core                  *applock.Service
	store                 *store.Store
	verifierPresent       bool
	verifier              applock.Verifier
	authenticateBiometric func() error
	biometricSettings     BiometricSettings
}

const appLockDomain = "settings"
const appLockKey = "app-lock-verifier"

func newAppLockService() *AppLockService {
	return &AppLockService{state: AppLockRuntimeState{Initialized: true, Locked: false, Version: 1}}
}

func newAppLockServiceWithDeps(core *applock.Service, profileStore *store.Store) *AppLockService {
	service := &AppLockService{
		state:                 AppLockRuntimeState{Initialized: true, Locked: false, Version: 1},
		core:                  core,
		store:                 profileStore,
		authenticateBiometric: applock.AuthenticateBiometric,
	}
	service.loadVerifier()
	if profileStore != nil {
		if raw, err := profileStore.GetRaw(appLockDomain, "app-lock-biometric"); err == nil {
			_ = json.Unmarshal(raw, &service.biometricSettings)
		}
	}
	return service
}

func (s *AppLockService) loadVerifier() {
	if s.store == nil {
		return
	}
	raw, err := s.store.GetRaw(appLockDomain, appLockKey)
	if errors.Is(err, store.ErrNoSuchKey) {
		return
	}
	s.verifierPresent = true
	s.state.Locked = true
	reason := "password"
	s.state.Reason = &reason
	now := time.Now().UnixMilli()
	s.state.LastLockedAt = &now
	if err != nil {
		return
	}
	var verifier applock.Verifier
	if err := json.Unmarshal(raw, &verifier); err == nil {
		s.verifier = verifier
	}
}

func (s *AppLockService) GetRuntimeState() AppLockRuntimeState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *AppLockService) ReportActivity() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixMilli()
	s.state.LastActivityAt = &now
}

func (s *AppLockService) SetRuntimeLocked(reason string) AppLockRuntimeState {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixMilli()
	if reason == "" {
		if s.verifierPresent {
			return s.state
		}
		s.state.Locked = false
		s.state.Reason = nil
		s.state.LastUnlockedAt = &now
		return s.state
	}
	s.state.Locked = true
	s.state.Reason = &reason
	s.state.Version++
	s.state.LastLockedAt = &now
	return s.state
}

func (s *AppLockService) Enable(password string) (AppLockRuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.verifierPresent {
		return s.state, fmt.Errorf("app lock already configured; authenticate before changing password")
	}
	if s.core == nil {
		return s.state, applock.ErrNoVerifier
	}
	verifier, err := s.core.Enable(context.Background(), password)
	if err != nil {
		return s.state, err
	}
	if s.store != nil {
		raw, marshalErr := json.Marshal(verifier)
		if marshalErr != nil {
			return s.state, marshalErr
		}
		if err := s.store.SetRaw(appLockDomain, appLockKey, raw); err != nil {
			return s.state, err
		}
	}
	s.verifier = verifier
	s.verifierPresent = true
	now := time.Now().UnixMilli()
	s.state.Locked = true
	reason := "password"
	s.state.Reason = &reason
	s.state.Version++
	s.state.LastLockedAt = &now
	return s.state, nil
}

func (s *AppLockService) Unlock(password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.core == nil {
		return applock.ErrNoVerifier
	}
	if err := s.core.Verify(s.verifier, password); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	s.state.Locked = false
	s.state.Reason = nil
	s.state.LastUnlockedAt = &now
	return nil
}

func (s *AppLockService) Disable(password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.core == nil {
		return applock.ErrNoVerifier
	}
	if err := s.core.Verify(s.verifier, password); err != nil {
		return err
	}
	if s.store != nil {
		if _, err := s.store.Write(store.WriteRequest{Mutations: []store.Mutation{
			{Domain: appLockDomain, Key: "app-lock-biometric", Delete: true},
			{Domain: appLockDomain, Key: appLockKey, Delete: true},
		}}); err != nil {
			return err
		}
	}
	now := time.Now().UnixMilli()
	s.state.Locked = false
	s.state.Reason = nil
	s.state.LastUnlockedAt = &now
	s.verifierPresent = false
	s.biometricSettings = BiometricSettings{}
	s.verifier = applock.Verifier{}
	return nil
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
	defer s.mu.Unlock()
	if s.state.Version != version || s.verifier != verifier {
		return BiometricUnlockResult{Error: "app lock changed during biometric authentication; retry"}
	}
	now := time.Now().UnixMilli()
	s.state.Locked = false
	s.state.Reason = nil
	s.state.LastUnlockedAt = &now
	return BiometricUnlockResult{Success: true}
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
