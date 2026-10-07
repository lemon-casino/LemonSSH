package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lemon-casino/lemonssh/internal/platform/applock"
	"github.com/lemon-casino/lemonssh/internal/profile/store"
)

func TestResetRejectsEmptyAndIncorrectPasswords(t *testing.T) {
	s := newAppLockServiceForTest(t)
	if _, err := s.Enable("original-password"); err != nil {
		t.Fatal(err)
	}
	locked := s.SetRuntimeLocked("startup")
	if !locked.Locked {
		t.Fatal("service must be lockable before reset")
	}

	if _, err := s.Reset(""); err == nil || err.Error() != "empty-current" {
		t.Fatalf("empty reset password: want typed empty-current, got %v", err)
	}
	if _, err := s.Reset("wrong-password"); err == nil || err.Error() != "incorrect" {
		t.Fatalf("wrong reset password: want typed incorrect, got %v", err)
	}
	if !s.GetRuntimeState().Locked {
		t.Fatal("failed reset attempts must keep the app locked")
	}
	if err := s.Unlock("original-password"); err != nil {
		t.Fatal("verifier must survive failed reset attempts")
	}
}

func TestResetRemovesVerifierAndSystemUnlockWhileLocked(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "profile.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := newAppLockServiceWithDeps(applock.New(&memoryCredentials{blobs: map[string][]byte{}}), db)
	if _, err := s.Enable("original-password"); err != nil {
		t.Fatal(err)
	}
	// Seed the system-unlock setting like biometric_unlock_test.go: directly
	// on the service and in the store, so the test never invokes the platform
	// biometric availability probe.
	if err := db.SetRaw(appLockDomain, "app-lock-biometric", []byte(`{"systemUnlockEnabled":true,"systemUnlockAutoPromptEnabled":true}`)); err != nil {
		t.Fatal(err)
	}
	s.biometricSettings = BiometricSettings{Enabled: true, AutoPromptEnabled: true}
	if locked := s.SetRuntimeLocked("startup"); !locked.Locked {
		t.Fatal("service must be locked before the reset")
	}

	// The lock-screen recovery entry runs while locked, with no unlock first.
	state, err := s.Reset("original-password")
	if err != nil {
		t.Fatal(err)
	}
	if state.Locked {
		t.Fatal("reset must unlock the app")
	}
	if state.Reason != nil {
		t.Fatalf("reset must clear the lock reason, got %q", *state.Reason)
	}

	settings := s.GetSettings()
	if enabled, ok := settings["enabled"].(bool); !ok || enabled {
		t.Fatalf("reset must disable app lock, got %v", settings["enabled"])
	}
	if verifier := settings["passwordVerifier"]; verifier != nil {
		t.Fatalf("reset must clear the stored verifier, got %v", verifier)
	}
	if status := s.GetSystemUnlockStatus(); status.Enabled {
		t.Fatal("reset must clear the system unlock setting")
	}
	// Persistence: a fresh service must start unlocked with nothing configured.
	reloaded := newAppLockServiceWithDeps(applock.New(&memoryCredentials{blobs: map[string][]byte{}}), db)
	if reloaded.GetRuntimeState().Locked {
		t.Fatal("reset must remove the persisted verifier")
	}
	if raw, err := db.GetRaw(appLockDomain, appLockKey); err == nil && len(raw) != 0 {
		t.Fatalf("reset must delete the verifier key, got %q", string(raw))
	}
	if raw, err := db.GetRaw(appLockDomain, "app-lock-biometric"); err == nil && len(raw) != 0 {
		t.Fatalf("reset must delete the biometric key, got %q", string(raw))
	}
}

func TestResetWithoutConfiguredVerifierFailsClosed(t *testing.T) {
	s := newAppLockServiceForTest(t)
	_, err := s.Reset("whatever")
	if err == nil {
		t.Fatal("reset without a configured verifier must fail")
	}
	if !strings.Contains(err.Error(), applock.ErrNoVerifier.Error()) {
		t.Fatalf("want ErrNoVerifier, got %v", err)
	}
}
