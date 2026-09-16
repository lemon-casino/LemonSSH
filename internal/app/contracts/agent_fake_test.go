package contracts

import (
	"fmt"
	"sync"
)

// ErrBusy is the sentinel the fake driver returns for a turn that is already
// running. Production owners map their own state onto CodeBusy.
var ErrBusy = NewError(CodeBusy, "turn is already running")

// FakeTurnDriver is a deterministic, transport-free stand-in for the future
// internal/agent runtime. It lives only in test support (W03): no production
// package may import this file because it compiles solely under go test.
type FakeTurnDriver struct {
	mu     sync.Mutex
	turns  map[TurnID]*fakeTurnState
	nextID int
}

type fakeTurnState struct {
	prepare  PrepareTurnRequest
	running  bool
	stopped  bool
	sequence uint64
	events   []AgentEventEnvelope
}

// NewFakeTurnDriver builds an empty driver.
func NewFakeTurnDriver() *FakeTurnDriver {
	return &FakeTurnDriver{turns: map[TurnID]*fakeTurnState{}}
}

// Prepare reserves one deterministic turn.
func (d *FakeTurnDriver) Prepare(request PrepareTurnRequest) PreparedTurn {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.nextID++
	turn := PreparedTurn{
		TurnID:                  TurnID(fmt.Sprintf("turn_%06d", d.nextID)),
		LeaseExpiresAtMS:        1726473600000,
		Cursor:                  "0",
		SnapshotRevision:        "1",
		EffectiveConfigRevision: "1",
		PolicyRevision:          "1",
	}
	d.turns[turn.TurnID] = &fakeTurnState{prepare: request}
	return turn
}

// Start flips the turn to running and emits turn_start.
func (d *FakeTurnDriver) Start(turnID TurnID) (*EventPage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, ok := d.turns[turnID]
	if !ok {
		return nil, NewError(CodeNotFound, "unknown turn")
	}
	if state.running {
		return nil, ErrBusy
	}
	state.running = true
	state.sequence++
	state.events = append(state.events, AgentEventEnvelope{
		SchemaVersion: 1,
		ChatSessionID: state.prepare.ChatSessionID,
		TurnID:        turnID,
		Sequence:      fmt.Sprint(state.sequence),
		Type:          "turn_start",
		Backend:       "go-catty",
	})
	return d.pageLocked(state), nil
}

// Stop cancels a running turn and emits turn_end.
func (d *FakeTurnDriver) Stop(turnID TurnID, reason string) (*EventPage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, ok := d.turns[turnID]
	if !ok {
		return nil, NewError(CodeNotFound, "unknown turn")
	}
	if state.stopped {
		return nil, NewError(CodeConflict, "turn already stopped")
	}
	state.stopped = true
	state.sequence++
	state.events = append(state.events, AgentEventEnvelope{
		SchemaVersion: 1,
		ChatSessionID: state.prepare.ChatSessionID,
		TurnID:        turnID,
		Sequence:      fmt.Sprint(state.sequence),
		Type:          "turn_end",
		Backend:       "go-catty",
	})
	return d.pageLocked(state), nil
}

// ReadEvents returns the page after the given decimal sequence cursor.
func (d *FakeTurnDriver) ReadEvents(turnID TurnID, afterSequence string, limit int) (*EventPage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, ok := d.turns[turnID]
	if !ok {
		return nil, NewError(CodeNotFound, "unknown turn")
	}
	var after uint64
	fmt.Sscanf(afterSequence, "%d", &after)
	var events []AgentEventEnvelope
	cursor := afterSequence
	for _, event := range state.events {
		var sequence uint64
		fmt.Sscanf(event.Sequence, "%d", &sequence)
		if sequence <= after {
			continue
		}
		if limit > 0 && len(events) >= limit {
			break
		}
		events = append(events, event)
		cursor = event.Sequence
	}
	return &EventPage{Events: events, NextCursor: cursor}, nil
}

func (d *FakeTurnDriver) pageLocked(state *fakeTurnState) *EventPage {
	events := append([]AgentEventEnvelope(nil), state.events...)
	return &EventPage{Events: events, NextCursor: fmt.Sprint(state.sequence)}
}
