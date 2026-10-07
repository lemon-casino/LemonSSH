package permissions

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestGrantCheckDefaultDeny(t *testing.T) {
	broker := NewBroker(nil)
	// No grants at all: deny.
	if err := broker.Check("p1", "terminal/session:read", "read"); !errors.Is(err, ErrNotGranted) {
		t.Fatalf("default must deny: %v", err)
	}
	if _, err := broker.Grant("p1", "terminal/session:read", LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}
	if err := broker.Check("p1", "terminal/session:read", "read"); err != nil {
		t.Fatalf("granted read must pass: %v", err)
	}
	// Write against a read-only resource resource suffix is denied.
	if err := broker.Check("p1", "terminal/session:write", "write"); !errors.Is(err, ErrNotGranted) {
		t.Fatalf("write must be denied by read grant: %v", err)
	}
	// Unknown principal denied.
	if err := broker.Check("ghost", "terminal/session:read", "read"); !errors.Is(err, ErrNotGranted) {
		t.Fatal("unknown principal must be denied")
	}
}

func TestGrantExpiryAndRevoke(t *testing.T) {
	now := time.Now()
	broker := NewBroker(func() time.Time { return now })
	if _, err := broker.Grant("p1", "clipboard:write", LifetimeApplication, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := broker.Check("p1", "clipboard:write", "write"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if err := broker.Check("p1", "clipboard:write", "write"); !errors.Is(err, ErrGrantExpired) {
		t.Fatalf("expired grant must fail: %v", err)
	}
	broker.Revoke("p1", "clipboard:write")
	if err := broker.Check("p1", "clipboard:write", "write"); !errors.Is(err, ErrNotGranted) {
		t.Fatal("revoked grant must deny")
	}
}

func TestRevokeAllQuarantinePath(t *testing.T) {
	broker := NewBroker(nil)
	_, _ = broker.Grant("p1", "a:read", LifetimeAlways, 0)
	_, _ = broker.Grant("p1", "b:read", LifetimeAlways, 0)
	broker.RevokeAll("p1")
	if err := broker.Check("p1", "a:read", "read"); !errors.Is(err, ErrNotGranted) {
		t.Fatal("revoke-all must clear every grant")
	}
}

func TestResourceValidation(t *testing.T) {
	broker := NewBroker(nil)
	if _, err := broker.Grant("p1", "  ", LifetimeAlways, 0); !errors.Is(err, ErrResourceInvalid) {
		t.Fatalf("blank resource must fail: %v", err)
	}
	if _, err := broker.Grant("", "a:read", LifetimeAlways, 0); !errors.Is(err, ErrPrincipalUnknown) {
		t.Fatal("blank principal must fail")
	}
}

func TestResourceStringNormalization(t *testing.T) {
	// Resource strings are the canonical authorization key; no trimming
	// means "fs:/data " and "fs:/data" are distinct and neither silently
	// broadens the other.
	broker := NewBroker(nil)
	_, _ = broker.Grant("p1", "fs:/data ", LifetimeAlways, 0)
	if err := broker.Check("p1", "fs:/data", "read"); !errors.Is(err, ErrNotGranted) {
		t.Fatal("trailing-space resource must not alias the clean resource")
	}
	_ = strings.TrimSpace
}
