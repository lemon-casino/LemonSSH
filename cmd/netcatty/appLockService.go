package main

import "sync"

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
	mu    sync.Mutex
	state AppLockRuntimeState
}

func newAppLockService() *AppLockService {
	return &AppLockService{state: AppLockRuntimeState{Initialized: true, Locked: false, Version: 1}}
}

func (s *AppLockService) GetRuntimeState() AppLockRuntimeState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *AppLockService) ReportActivity() {
	s.mu.Lock()
	defer s.mu.Unlock()
}

func (s *AppLockService) SetRuntimeLocked(reason string) AppLockRuntimeState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if reason == "" {
		s.state.Locked = false
		s.state.Reason = nil
		return s.state
	}
	s.state.Locked = true
	s.state.Reason = &reason
	s.state.Version++
	return s.state
}
