package upgrade

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistentBeginsAndResumes(t *testing.T) {
	dir := t.TempDir()

	p1, err := OpenPersistentCoordinator(dir, "3.2.0", "4.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, step, _ := p1.Status(); step != StepDetected {
		t.Fatalf("initial step %s", step)
	}
	if err := p1.Transition(StepBackingUp); err != nil {
		t.Fatal(err)
	}
	if err := p1.Transition(StepBackedUp); err != nil {
		t.Fatal(err)
	}

	p2, err := OpenPersistentCoordinator(dir, "3.2.0", "4.0.0")
	if err != nil {
		t.Fatal(err)
	}
	from, to, step, history := p2.Status()
	if from != "3.2.0" || to != "4.0.0" {
		t.Fatalf("version pair mismatch: %s -> %s", from, to)
	}
	if step != StepBackedUp {
		t.Fatalf("resumed at %s, want backed_up", step)
	}
	if len(history) != 3 || history[0] != StepDetected || history[2] != StepBackedUp {
		t.Fatalf("history mismatch: %v", history)
	}
	// The resumed machine still enforces forward-only transitions.
	if err := p2.Transition(StepDetected); err == nil {
		t.Fatal("backward transition accepted")
	}
	if err := p2.Transition(StepMigrating); err != nil {
		t.Fatal(err)
	}
}

func TestPersistentVersionMismatchAndClear(t *testing.T) {
	dir := t.TempDir()
	if _, err := OpenPersistentCoordinator(dir, "3.2.0", "4.0.0"); err != nil {
		t.Fatal(err)
	}
	_, err := OpenPersistentCoordinator(dir, "3.3.0", "4.0.0")
	if !errors.Is(err, ErrVersionMismatch) {
		t.Fatalf("expected mismatch, got %v", err)
	}
	if err := ClearState(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenPersistentCoordinator(dir, "3.3.0", "4.0.0"); err != nil {
		t.Fatalf("state not cleared: %v", err)
	}
	// Clearing a missing state is a no-op.
	if err := ClearState(dir); err != nil {
		t.Fatal(err)
	}
}

func TestPersistentCorruptStateFailsClosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, StateFileName), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := OpenPersistentCoordinator(dir, "3.2.0", "4.0.0")
	if err == nil {
		t.Fatal("corrupt state accepted")
	}
}

func TestPersistentStateFileIsAtomicAndValid(t *testing.T) {
	dir := t.TempDir()
	p, err := OpenPersistentCoordinator(dir, "3.2.0", "4.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Transition(StepBackingUp); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "upgrade-state.json.tmp")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("temp file leaked after commit")
	}
	if _, err := os.Stat(filepath.Join(dir, StateFileName)); err != nil {
		t.Fatalf("state file missing: %v", err)
	}
}
