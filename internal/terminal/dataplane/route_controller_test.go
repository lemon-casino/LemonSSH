package dataplane

import (
	"errors"
	"testing"
)

func TestRouteControllerAuthGenerationAndCredit(t *testing.T) {
	controller := NewRouteController()
	bootstrap, err := controller.Open("session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(bootstrap.DataToken) != 64 || len(bootstrap.UrgentToken) != 64 {
		t.Fatal("tokens must be 32-byte hex")
	}
	if err := controller.Authenticate("session-1", bootstrap.Generation, bootstrap.DataToken, false); err != nil {
		t.Fatal(err)
	}
	if err := controller.Authenticate("session-1", bootstrap.Generation, bootstrap.DataToken+"x", false); err == nil {
		t.Fatal("invalid token accepted")
	}
	if err := controller.ApplyCredit("session-1", bootstrap.Generation, 0, uint32(ReceiveWindowBytes)); err != nil {
		t.Fatal(err)
	}
	sequence, err := controller.AdmitOutput("session-1", bootstrap.Generation, 512)
	if err != nil || sequence != 1 {
		t.Fatalf("admit output: seq=%d err=%v", sequence, err)
	}
	if err := controller.ApplyCredit("session-1", bootstrap.Generation, 1, 512); err != nil {
		t.Fatal(err)
	}
	_, available, _, applied, err := controller.Snapshot("session-1")
	if err != nil || applied != 1 || available != ReceiveWindowBytes {
		t.Fatalf("snapshot mismatch: %d %d %v", available, applied, err)
	}

	replacement, err := controller.Open("session-1")
	if err != nil || replacement.Generation != bootstrap.Generation+1 {
		t.Fatal("generation did not advance")
	}
	if err := controller.Authenticate("session-1", bootstrap.Generation, bootstrap.DataToken, false); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("stale route accepted: %v", err)
	}
}

func TestRouteControllerCreditAdmissionIsBounded(t *testing.T) {
	controller := NewRouteController()
	bootstrap, err := controller.Open("session-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.ApplyCredit("session-1", bootstrap.Generation, 0, uint32(ReceiveWindowBytes)); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.AdmitOutput("session-1", bootstrap.Generation, uint32(ReceiveWindowBytes)+1); !errors.Is(err, ErrInsufficientCredit) {
		t.Fatalf("oversized output accepted: %v", err)
	}
	if err := controller.ApplyCredit("session-1", bootstrap.Generation, 99, 1); !errors.Is(err, ErrCreditSequence) {
		t.Fatalf("invalid sequence accepted: %v", err)
	}
}

func TestRouteControllerConcurrentAdmission(t *testing.T) {
	controller := NewRouteController()
	bootstrap, err := controller.Open("session-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.ApplyCredit("session-1", bootstrap.Generation, 0, uint32(ReceiveWindowBytes)); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 64)
	for i := 0; i < 64; i++ {
		go func() { _, err := controller.AdmitOutput("session-1", bootstrap.Generation, 1024); results <- err }()
	}
	for i := 0; i < 64; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}
