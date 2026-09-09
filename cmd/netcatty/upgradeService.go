package main

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/binaricat/netcatty/internal/platform/upgrade"
)

// UpgradeService drives the Electron N-1 → Wails upgrade bootstrap chain
// (P6-04): detect → backup → migrate → verify → activate, persisted
// crash-safely in the profile data directory. The service itself is
// stateless; every call reopens the persisted coordinator so renderer
// reloads cannot desynchronize the machine.
type UpgradeService struct {
	dir string
	mu  sync.Mutex
}

// UpgradeStatus is the wire view of the persisted sequence.
type UpgradeStatus struct {
	From    string   `json:"from"`
	To      string   `json:"to"`
	Step    string   `json:"step"`
	History []string `json:"history"`
}

func newUpgradeService(dataDir string) *UpgradeService {
	return &UpgradeService{dir: filepath.Join(dataDir, "upgrade")}
}

// Status reports the persisted upgrade sequence; the returned status carries
// empty From/To and a "detected" step when no upgrade is in flight.
func (s *UpgradeService) Status() (UpgradeStatus, error) {
	from, to, step, history, ok, err := upgrade.ReadState(s.dir)
	if err != nil {
		return UpgradeStatus{}, err
	}
	if !ok {
		return UpgradeStatus{Step: string(upgrade.StepDetected), History: []string{string(upgrade.StepDetected)}}, nil
	}
	encoded := make([]string, 0, len(history))
	for _, entry := range history {
		encoded = append(encoded, string(entry))
	}
	return UpgradeStatus{From: from, To: to, Step: string(step), History: encoded}, nil
}

// Begin starts (or resumes) the upgrade sequence for the version pair.
func (s *UpgradeService) Begin(fromVersion, toVersion string) (UpgradeStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	persistent, err := upgrade.OpenPersistentCoordinator(s.dir, fromVersion, toVersion)
	if err != nil {
		return UpgradeStatus{}, fmt.Errorf("begin upgrade: %w", err)
	}
	return statusOf(persistent), nil
}

// Advance moves the machine one step forward.
func (s *UpgradeService) Advance(fromVersion, toVersion, next string) (UpgradeStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	persistent, err := upgrade.OpenPersistentCoordinator(s.dir, fromVersion, toVersion)
	if err != nil {
		return UpgradeStatus{}, fmt.Errorf("advance upgrade: %w", err)
	}
	step := upgrade.Step(next)
	switch step {
	case upgrade.StepBackingUp, upgrade.StepBackedUp, upgrade.StepMigrating,
		upgrade.StepMigrated, upgrade.StepVerified, upgrade.StepActivated, upgrade.StepFailed:
	default:
		return UpgradeStatus{}, fmt.Errorf("unknown upgrade step %q", next)
	}
	if err := persistent.Transition(step); err != nil {
		return UpgradeStatus{}, err
	}
	return statusOf(persistent), nil
}

// Cancel discards the persisted sequence (rollback to the Electron carrier).
func (s *UpgradeService) Cancel() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return upgrade.ClearState(s.dir)
}

func statusOf(persistent *upgrade.PersistentCoordinator) UpgradeStatus {
	from, to, step, history := persistent.Status()
	encoded := make([]string, 0, len(history))
	for _, entry := range history {
		encoded = append(encoded, string(entry))
	}
	return UpgradeStatus{From: from, To: to, Step: string(step), History: encoded}
}
