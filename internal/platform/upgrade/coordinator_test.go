package upgrade

import (
	"testing"
)

func TestUpgradeStateMachine(t *testing.T) {
	coordinator, err := NewCoordinator("1.0.0", "2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	steps := []Step{
		StepBackingUp, StepBackedUp, StepMigrating, StepMigrated, StepVerified, StepActivated,
	}
	for _, next := range steps {
		if err := coordinator.Transition(next); err != nil {
			t.Fatalf("transition to %s: %v", next, err)
		}
	}
	if coordinator.Step() != StepActivated {
		t.Fatalf("expected activated, got %s", coordinator.Step())
	}
}

func TestUpgradeRejectsSkippedSteps(t *testing.T) {
	coordinator, err := NewCoordinator("1.0.0", "2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	// Cannot jump from detected to migrating.
	if err := coordinator.Transition(StepMigrating); err == nil {
		t.Fatal("skipped backup must be rejected")
	}
}

func TestUpgradeSameVersionRejected(t *testing.T) {
	_, err := NewCoordinator("1.0.0", "1.0.0")
	if err == nil {
		t.Fatal("same version must be rejected")
	}
}

func TestUpgradeFailureTerminal(t *testing.T) {
	coordinator, err := NewCoordinator("1.0.0", "2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	_ = coordinator.Transition(StepBackingUp)
	_ = coordinator.Transition(StepBackedUp)
	_ = coordinator.Transition(StepMigrating)
	_ = coordinator.Transition(StepFailed)
	// Failed is terminal; no further transitions allowed.
	if err := coordinator.Transition(StepMigrated); err == nil {
		t.Fatal("failed state must be terminal")
	}
}
