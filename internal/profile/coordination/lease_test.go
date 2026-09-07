package coordination

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// sharedDirManager returns a factory over one shared profile path so that
// multiple managers genuinely contend on the same lock file.
func sharedDirManager(t *testing.T) func(name string) *Manager {
	t.Helper()
	dir := t.TempDir()
	return func(name string) *Manager {
		return NewManager(filepath.Join(dir, name+".db"), name)
	}
}

func TestAcquireConflictAndRelease(t *testing.T) {
	newManager := sharedDirManager(t)
	first := newManager("profile")
	lease, err := first.Acquire(30 * time.Second)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if lease.Epoch != 1 {
		t.Fatalf("first epoch must be 1, got %d", lease.Epoch)
	}

	second := newManager("profile")
	t.Cleanup(func() { _ = second.Release() })
	if _, err := second.Acquire(time.Second); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("second acquire must conflict, got %v", err)
	}

	if err := first.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	taken, err := second.Acquire(30 * time.Second)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	if taken.Epoch != 2 {
		t.Fatalf("epoch must bump to 2, got %d", taken.Epoch)
	}
	if err := second.Release(); err != nil {
		t.Fatalf("second release: %v", err)
	}
}

func TestCrashTakeoverKeepsEpochMonotonic(t *testing.T) {
	newManager := sharedDirManager(t)
	profilePath := newManager("probe").profilePath

	crashed := NewManager(profilePath, "crashed")
	if _, err := crashed.Acquire(time.Minute); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	// Simulate process death: drop the flock handle without a clean Release.
	// The OS releases the advisory lock when the handle dies.
	if err := crashed.flock.Unlock(); err != nil {
		t.Fatalf("simulated death: %v", err)
	}
	crashed.flock = nil

	takeover := NewManager(profilePath, "takeover")
	t.Cleanup(func() { _ = takeover.Release() })
	lease, err := takeover.Acquire(time.Minute)
	if err != nil {
		t.Fatalf("takeover: %v", err)
	}
	current, ok := takeover.Current()
	if !ok || current.HolderID != "takeover" || current.Epoch != 2 {
		t.Fatalf("lease file must record the takeover with epoch 2: %+v ok=%v", current, ok)
	}
	if lease.Epoch != 2 {
		t.Fatalf("takeover lease epoch mismatch: %d", lease.Epoch)
	}
}

func TestRenewExtendsAndExpiredRenewRejected(t *testing.T) {
	newManager := sharedDirManager(t)
	m := newManager("profile")
	t.Cleanup(func() { _ = m.Release() })
	lease, err := m.Acquire(50 * time.Millisecond)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	renewed, err := m.Renew(time.Minute)
	if err != nil {
		t.Fatalf("renew: %v", err)
	}
	if renewed.Epoch != lease.Epoch {
		t.Fatal("renewal must not change the epoch")
	}
	if renewed.ExpiresAtMS <= lease.ExpiresAtMS {
		t.Fatal("renewal must extend the expiry")
	}

	short := newManager("profile-short")
	t.Cleanup(func() { _ = short.Release() })
	if _, err := short.Acquire(20 * time.Millisecond); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	if _, err := short.Renew(time.Minute); !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("expired renewal must be rejected, got %v", err)
	}
}

func TestAcquireAfterExpireButHeldIsConflict(t *testing.T) {
	newManager := sharedDirManager(t)
	holder := newManager("profile")
	t.Cleanup(func() { _ = holder.Release() })
	if _, err := holder.Acquire(20 * time.Millisecond); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	time.Sleep(40 * time.Millisecond) // lease expired, but holder is still live
	other := newManager("profile")
	if _, err := other.Acquire(time.Minute); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("live holder must never be stolen by timeout, got %v", err)
	}
}

func TestDoubleAcquireRejected(t *testing.T) {
	newManager := sharedDirManager(t)
	m := newManager("profile")
	t.Cleanup(func() { _ = m.Release() })
	if _, err := m.Acquire(time.Minute); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if _, err := m.Acquire(time.Minute); err == nil {
		t.Fatal("double acquire within one manager must fail")
	}
}
