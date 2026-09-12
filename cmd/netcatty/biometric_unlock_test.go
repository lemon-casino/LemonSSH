package main

import (
	"testing"
)

func TestBiometricPreferencesRequireUnlockedPasswordAndClearOnDisable(t *testing.T) {
	s := newAppLockServiceForTest(t)
	if _, err := s.Enable("test-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetSystemUnlockEnabled(true, "test-password", false); err == nil {
		t.Fatal("locked settings mutation succeeded")
	}
	if err := s.Unlock("test-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetSystemUnlockEnabled(true, "wrong", false); err == nil {
		t.Fatal("wrong password enabled system authentication")
	}
	s.biometricSettings = BiometricSettings{Enabled: true, AutoPromptEnabled: true}
	if err := s.Disable("test-password"); err != nil {
		t.Fatal(err)
	}
	if s.biometricSettings.Enabled || s.biometricSettings.AutoPromptEnabled {
		t.Fatal("disable retained biometric opt-in")
	}
	if result := s.UnlockWithBiometrics(); result.Success {
		t.Fatal("disabled system auth unlocked")
	}
}

// A successful OS prompt must not unlock a newer lock imposed while it was open.
func TestBiometricUnlockRejectsStaleAuthentication(t *testing.T) {
	s := newAppLockServiceForTest(t)
	if _, err := s.Enable("test-password"); err != nil {
		t.Fatal(err)
	}
	s.biometricSettings.Enabled = true
	s.authenticateBiometric = func() error { s.SetRuntimeLocked("new-lock"); return nil }
	result := s.UnlockWithBiometrics()
	if result.Success || !s.GetRuntimeState().Locked {
		t.Fatal("stale biometric prompt unlocked a newer lock")
	}
}
func TestBiometricUnlockUpdatesStateOnlyAfterAuthentication(t *testing.T) {
	s := newAppLockServiceForTest(t)
	if _, err := s.Enable("test-password"); err != nil {
		t.Fatal(err)
	}
	s.biometricSettings.Enabled = true
	s.authenticateBiometric = func() error { return nil }
	if result := s.UnlockWithBiometrics(); !result.Success {
		t.Fatal(result.Error)
	}
	state := s.GetRuntimeState()
	if state.Locked || state.LastUnlockedAt == nil {
		t.Fatal("native authentication did not unlock")
	}
}
