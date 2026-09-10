package main

import (
	"context"
	"encoding/json"
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
	mu       sync.Mutex
	state    AppLockRuntimeState
	core     *applock.Service
	store    *store.Store
	verifier applock.Verifier
}

const appLockDomain = "settings"
const appLockKey = "app-lock-verifier"

func newAppLockService() *AppLockService {
	return &AppLockService{state: AppLockRuntimeState{Initialized: true, Locked: false, Version: 1}}
}

func newAppLockServiceWithDeps(core *applock.Service, profileStore *store.Store) *AppLockService {
	service := &AppLockService{
		state: AppLockRuntimeState{Initialized: true, Locked: false, Version: 1},
		core:  core,
		store: profileStore,
	}
	service.loadVerifier()
	return service
}

func (s *AppLockService) loadVerifier() {
	if s.store == nil {
		return
	}
	raw, err := s.store.GetRaw(appLockDomain, appLockKey)
	if err != nil || len(raw) == 0 {
		return
	}
	var verifier applock.Verifier
	if err := json.Unmarshal(raw, &verifier); err != nil {
		return
	}
	s.verifier = verifier
	s.state.Locked = true
	now := time.Now().UnixMilli()
	s.state.LastLockedAt = &now
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
	now := time.Now().UnixMilli()
	s.state.Locked = false
	s.state.Reason = nil
	s.state.LastUnlockedAt = &now
	if s.store != nil {
		_ = s.store.DeleteRaw(appLockDomain, appLockKey)
	}
	s.verifier = applock.Verifier{}
	return nil
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
