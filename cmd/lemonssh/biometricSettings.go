package main

import (
	"encoding/json"
	"fmt"
	"github.com/binaricat/lemonssh/internal/platform/applock"
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
	if s.state.Locked {
		s.mu.Unlock()
		return s.biometricSettings, fmt.Errorf("locked")
	}
	if s.core == nil || s.verifier.Digest == "" {
		s.mu.Unlock()
		return s.biometricSettings, fmt.Errorf("unavailable")
	}
	if enabled != s.biometricSettings.Enabled {
		if password == "" {
			s.mu.Unlock()
			return s.biometricSettings, fmt.Errorf("empty-current")
		}
		if err := s.core.Verify(s.verifier, password); err != nil {
			s.mu.Unlock()
			return s.biometricSettings, fmt.Errorf("incorrect")
		}
	}
	if enabled {
		if err := applock.BiometricAvailable(); err != nil {
			s.mu.Unlock()
			return s.biometricSettings, fmt.Errorf("unavailable")
		}
	}
	next := BiometricSettings{Enabled: enabled, AutoPromptEnabled: enabled && autoPrompt}
	if s.store != nil {
		raw, err := json.Marshal(next)
		if err != nil {
			s.mu.Unlock()
			return s.biometricSettings, err
		}
		if err = s.store.SetRaw(appLockDomain, "app-lock-biometric", raw); err != nil {
			s.mu.Unlock()
			return s.biometricSettings, err
		}
	}
	changed := next != s.biometricSettings
	s.biometricSettings = next
	s.mu.Unlock()
	if changed {
		// Broadcast after the mutation so other windows resync their cached
		// App Lock settings.
		s.emitSettingsChanged()
	}
	return next, nil
}
