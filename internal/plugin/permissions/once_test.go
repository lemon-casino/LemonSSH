package permissions

import "testing"

func TestOnceGrantConsumedAndInvalidModeDenied(t *testing.T) {
	b := NewBroker(nil)
	_, _ = b.Grant("example", "terminal/session:read", LifetimeOnce, 0)
	if err := b.Check("example", "terminal/session:read", "execute"); err == nil {
		t.Fatal("unknown mode authorized")
	}
	if err := b.Check("example", "terminal/session:read", "read"); err != nil {
		t.Fatal(err)
	}
	if err := b.Check("example", "terminal/session:read", "read"); err == nil {
		t.Fatal("once grant reused")
	}
}
