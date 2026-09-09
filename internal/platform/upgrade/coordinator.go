// Package upgrade implements the Electron N-1 to Wails upgrade bootstrap
// coordinator (P6-04). It validates version compatibility, directs the
// migration flow (export → import → promote), and tracks the upgrade state
// machine through: detect → backup → migrate → verify → activate.
package upgrade

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

var (
	ErrInvalidVersions   = errors.New("upgrade: invalid version pair")
	ErrNotSupported      = errors.New("upgrade path not supported")
	ErrUpgradeInProgress = errors.New("upgrade already in progress")
)

// Step is the upgrade state machine.
type Step string

const (
	StepDetected  Step = "detected"
	StepBackingUp Step = "backing_up"
	StepBackedUp  Step = "backed_up"
	StepMigrating Step = "migrating"
	StepMigrated  Step = "migrated"
	StepVerified  Step = "verified"
	StepActivated Step = "activated"
	StepFailed    Step = "failed"
)

// Coordinator tracks one upgrade sequence.
type Coordinator struct {
	mu      sync.Mutex
	from    string // Electron version
	to      string // Wails version
	step    Step
	history []Step
}

// NewCoordinator validates the version pair and creates an upgrade coordinator.
func NewCoordinator(fromVersion, toVersion string) (*Coordinator, error) {
	fromVersion = strings.TrimPrefix(fromVersion, "v")
	toVersion = strings.TrimPrefix(toVersion, "v")
	if fromVersion == "" || toVersion == "" {
		return nil, ErrInvalidVersions
	}
	if fromVersion == toVersion {
		return nil, ErrNotSupported
	}
	return &Coordinator{from: fromVersion, to: toVersion, step: StepDetected, history: []Step{StepDetected}}, nil
}

// Transition advances to the next step. Only forward transitions are allowed.
func (c *Coordinator) Transition(next Step) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !allowedTransition(c.step, next) {
		return fmt.Errorf("invalid transition %s -> %s", c.step, next)
	}
	c.step = next
	c.history = append(c.history, next)
	return nil
}

func allowedTransition(from, to Step) bool {
	switch from {
	case StepDetected:
		return to == StepBackingUp
	case StepBackingUp:
		return to == StepBackedUp || to == StepFailed
	case StepBackedUp:
		return to == StepMigrating || to == StepFailed
	case StepMigrating:
		return to == StepMigrated || to == StepFailed
	case StepMigrated:
		return to == StepVerified || to == StepFailed
	case StepVerified:
		return to == StepActivated || to == StepFailed
	default:
		return false
	}
}

// Step reports the current step.
func (c *Coordinator) Step() Step {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.step
}

// History returns the transition history.
func (c *Coordinator) History() []Step {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Step(nil), c.history...)
}
