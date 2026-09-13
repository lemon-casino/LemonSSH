package main

import (
	"encoding/json"
	"fmt"
	"github.com/binaricat/netcatty/internal/platform/applock"
	"runtime"
)

type BiometricSettings struct {
	Enabled           bool `json:"systemUnlockEnabled"`
	AutoPromptEnabled bool `json:"systemUnlockAutoPromptEnabled"`
}
type BiometricStatus struct {
	Supported bool    `json:"supported"`
	Available bool    `json:"available"`
	Enabled   bool    `json:"enabled"`
	Platform  string  `json:"platform"`
	Label     *string `json:"label"`
	Reason    *string `json:"reason"`
}

func (s *AppLockService) GetSettings() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	var verifier any
	if s.verifier.Digest != "" {
		verifier = map[string]any{"version": 1, "algorithm": "PBKDF2-SHA256", "iterations": 210000, "salt": s.verifier.Salt, "hash": s.verifier.Digest}
	}
	return map[string]any{"enabled": verifier != nil, "timeoutMinutes": 15, "systemUnlockEnabled": s.biometricSettings.Enabled && verifier != nil, "systemUnlockAutoPromptEnabled": s.biometricSettings.AutoPromptEnabled && s.biometricSettings.Enabled && verifier != nil, "passwordVerifier": verifier}
}

func (s *AppLockService) GetSystemUnlockStatus() BiometricStatus {
	s.mu.Lock()
	enabled := s.biometricSettings.Enabled && s.verifier.Digest != ""
	s.mu.Unlock()
	status := BiometricStatus{Enabled: enabled, Platform: "unsupported"}
	label := ""
	switch runtime.GOOS {
	case "windows":
		status.Platform = "win32"
		label = "Windows Hello"
	case "darwin":
		status.Platform = "darwin"
		label = "Touch ID"
	}
	status.Supported = label != ""
	if status.Supported {
		status.Label = &label
	}
	if err := applock.BiometricAvailable(); err != nil {
		reason := err.Error()
		status.Reason = &reason
	} else {
		status.Available = true
	}
	return status
}
func (s *AppLockService) SetSystemUnlockEnabled(enabled bool, password string, autoPrompt bool) (BiometricSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Locked {
		return s.biometricSettings, fmt.Errorf("locked")
	}
	if s.core == nil || s.verifier.Digest == "" {
		return s.biometricSettings, fmt.Errorf("unavailable")
	}
	if enabled != s.biometricSettings.Enabled {
		if password == "" {
			return s.biometricSettings, fmt.Errorf("empty-current")
		}
		if err := s.core.Verify(s.verifier, password); err != nil {
			return s.biometricSettings, fmt.Errorf("incorrect")
		}
	}
	if enabled {
		if err := applock.BiometricAvailable(); err != nil {
			return s.biometricSettings, fmt.Errorf("unavailable")
		}
	}
	next := BiometricSettings{Enabled: enabled, AutoPromptEnabled: enabled && autoPrompt}
	if s.store != nil {
		raw, err := json.Marshal(next)
		if err != nil {
			return s.biometricSettings, err
		}
		if err = s.store.SetRaw(appLockDomain, "app-lock-biometric", raw); err != nil {
			return s.biometricSettings, err
		}
	}
	s.biometricSettings = next
	return next, nil
}
