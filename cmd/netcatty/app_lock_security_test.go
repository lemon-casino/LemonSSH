package main

import (
	"github.com/binaricat/netcatty/internal/platform/applock"
	"github.com/binaricat/netcatty/internal/profile/store"
	"path/filepath"
	"testing"
)

func TestCorruptVerifierAndFailedDisableStayLocked(t *testing.T) {
	for _, raw := range []string{`{`, `{}`} {
		db, err := store.Open(filepath.Join(t.TempDir(), "profile.db"), nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.SetRaw(appLockDomain, appLockKey, []byte(raw)); err != nil {
			t.Fatal(err)
		}
		s := newAppLockServiceWithDeps(applock.New(&memoryCredentials{}), db)
		if !s.GetRuntimeState().Locked {
			t.Fatalf("corrupt verifier %q initialized unlocked", raw)
		}
		if _, err := s.Enable("replacement"); err == nil {
			t.Fatal("corrupt verifier overwritten")
		}
		db.Close()
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "profile.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := newAppLockServiceWithDeps(applock.New(&memoryCredentials{}), db)
	if _, err := s.Enable("test-password"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := s.Disable("test-password"); err == nil {
		t.Fatal("closed store disable succeeded")
	}
	if !s.GetRuntimeState().Locked {
		t.Fatal("failed persistence unlocked app")
	}
}

func TestConfiguredLockCannotBeReplacedOrClearedWithoutAuthentication(t *testing.T) {
	s := newAppLockServiceForTest(t)
	if _, err := s.Enable("original-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enable("replacement-password"); err == nil {
		t.Fatal("existing verifier overwritten without authentication")
	}
	if state := s.SetRuntimeLocked(""); !state.Locked {
		t.Fatal("empty reason bypassed password authentication")
	}
	if err := s.Unlock("replacement-password"); err == nil {
		t.Fatal("replacement password accepted")
	}
	if err := s.Unlock("original-password"); err != nil {
		t.Fatal(err)
	}
}
