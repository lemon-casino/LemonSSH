package upgrade

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrVersionMismatch reports persisted state for a different version pair.
var ErrVersionMismatch = errors.New("upgrade: persisted state belongs to a different version pair")

// persistedState is the crash-safe on-disk record of one upgrade sequence.
type persistedState struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	Step      Step      `json:"step"`
	History   []Step    `json:"history"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// PersistentCoordinator wraps the in-memory coordinator with crash-safe state
// persistence so an interrupted upgrade resumes at the stored step instead of
// restarting the migration flow (P6-04 bootstrap chain).
type PersistentCoordinator struct {
	mu   sync.Mutex
	path string
	co   *Coordinator
}

// StateFileName is the file stored inside the upgrade state directory.
const StateFileName = "upgrade-state.json"

// OpenPersistentCoordinator resumes (or begins) the upgrade sequence for the
// version pair, persisting every transition atomically into dir.
func OpenPersistentCoordinator(dir, fromVersion, toVersion string) (*PersistentCoordinator, error) {
	if dir == "" {
		return nil, errors.New("upgrade: state directory is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("upgrade: create state dir: %w", err)
	}
	path := filepath.Join(dir, StateFileName)

	co, err := NewCoordinator(fromVersion, toVersion)
	if err != nil {
		return nil, err
	}
	persistent := &PersistentCoordinator{path: path, co: co}

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := persistent.persist(); err != nil {
			return nil, err
		}
		return persistent, nil
	}
	if err != nil {
		return nil, fmt.Errorf("upgrade: read state: %w", err)
	}
	var stored persistedState
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, fmt.Errorf("upgrade: corrupt state %s: %w", path, err)
	}
	if stored.From != co.from || stored.To != co.to {
		return nil, fmt.Errorf("%w: stored %s -> %s, requested %s -> %s",
			ErrVersionMismatch, stored.From, stored.To, fromVersion, toVersion)
	}
	// Rebuild the in-memory machine at the stored step with its history.
	history := append([]Step(nil), stored.History...)
	if len(history) == 0 {
		history = []Step{StepDetected}
	}
	if history[len(history)-1] != stored.Step {
		history = append(history, stored.Step)
	}
	resumed := &Coordinator{from: stored.From, to: stored.To, step: stored.Step, history: history}
	persistent.co = resumed
	return persistent, nil
}

// ClearState removes the persisted state (fresh start for a new sequence).
func ClearState(dir string) error {
	err := os.Remove(filepath.Join(dir, StateFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ReadState reports the persisted sequence without validating or resuming a
// version pair (ok is false when no upgrade state exists).
func ReadState(dir string) (from, to string, step Step, history []Step, ok bool, err error) {
	raw, err := os.ReadFile(filepath.Join(dir, StateFileName))
	if errors.Is(err, os.ErrNotExist) {
		return "", "", "", nil, false, nil
	}
	if err != nil {
		return "", "", "", nil, false, fmt.Errorf("upgrade: read state: %w", err)
	}
	var stored persistedState
	if err := json.Unmarshal(raw, &stored); err != nil {
		return "", "", "", nil, false, fmt.Errorf("upgrade: corrupt state in %s: %w", dir, err)
	}
	return stored.From, stored.To, stored.Step, append([]Step(nil), stored.History...), true, nil
}

// Transition advances the machine and persists the new step. The state file
// is written atomically (temp file + rename) before the call returns.
func (p *PersistentCoordinator) Transition(next Step) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.co.Transition(next); err != nil {
		return err
	}
	return p.persist()
}

// Status reports the persisted sequence.
func (p *PersistentCoordinator) Status() (from, to string, step Step, history []Step) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.co.from, p.co.to, p.co.step, append([]Step(nil), p.co.history...)
}

// persist must be called with the mutex held.
func (p *PersistentCoordinator) persist() error {
	from, to, step, history := p.co.from, p.co.to, p.co.step, p.co.history
	state := persistedState{
		From:      from,
		To:        to,
		Step:      step,
		History:   append([]Step(nil), history...),
		UpdatedAt: time.Now().UTC(),
	}
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("upgrade: encode state: %w", err)
	}
	temp := p.path + ".tmp"
	if err := os.WriteFile(temp, encoded, 0o600); err != nil {
		return fmt.Errorf("upgrade: write state: %w", err)
	}
	if err := os.Rename(temp, p.path); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("upgrade: commit state: %w", err)
	}
	return nil
}
