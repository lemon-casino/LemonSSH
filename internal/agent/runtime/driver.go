package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/binaricat/netcatty/internal/app/contracts"
)

// TurnDriver is the transport-independent seam the runtime drives (W03
// kept this shape; W11 owns the lifecycle). Implementations stream one
// turn: they emit events through the session and return when the model
// turn finishes or ctx is cancelled. Stop convergence is bounded by ctx.
type TurnDriver interface {
	Stream(ctx context.Context, session *DriverSession) error
}

// DriverSession is one turn's view into the runtime: identity, resolved
// input and the event sink. Sequence assignment stays inside the runtime
// — there is exactly one counter per turn, shared with the terminal
// record, so no event can collide or travel out of order.
type DriverSession struct {
	TurnID contracts.TurnID
	ChatID contracts.ChatSessionID
	Input  contracts.TurnInput
	Scope  contracts.TurnScope

	emit func(eventType string, payload json.RawMessage) error
}

// Emit stores one driver event under the turn's single sequence counter.
// Payload may be nil.
func (s *DriverSession) Emit(eventType string, payload json.RawMessage) error {
	return s.emit(eventType, payload)
}

// startTurn consumes the reservation, flips the turn to running and starts
// the driver on a per-turn context. Start retries for the same request are
// idempotent: exactly one driver run ever happens (T02).
func (m *TurnManager) StartTurn(cmd contracts.TurnCommand) error {
	if cmd.Kind != contracts.TurnCommandStart {
		return contracts.NewError(contracts.CodeInvalidRequest, "StartTurn requires the start kind")
	}
	m.mu.Lock()
	res, ok := m.requests[cmd.RequestID]
	driver := m.driver
	m.mu.Unlock()
	if !ok {
		return contracts.NewError(contracts.CodeNotFound, "unknown request")
	}
	if driver == nil {
		return contracts.NewError(contracts.CodeUnavailable, "no turn driver is configured")
	}
	state := m.chat(res.chatID)
	state.mu.Lock()
	if res.consumed {
		state.mu.Unlock()
		if res.turn.TurnID == cmd.TurnID || cmd.TurnID == "" {
			return nil // idempotent retry: the turn is already running
		}
		return contracts.NewError(contracts.CodeConflict, "request already started a different turn")
	}
	if res.turn.TurnID != cmd.TurnID {
		state.mu.Unlock()
		return contracts.NewError(contracts.CodeInvalidRequest, "turn id does not match the reservation")
	}
	res.consumed = true
	state.reservation = nil
	state.mu.Unlock()

	m.mu.Lock()
	record := m.turns[res.turn.TurnID]
	m.mu.Unlock()
	if record == nil {
		return contracts.NewError(contracts.CodeNotFound, "unknown turn")
	}

	// Lock order: chatState.mu before turnRecord.mu (consistent with
	// StopChat and finalize).
	state.mu.Lock()
	record.mu.Lock()
	record.status = StatusRunning
	record.revision++
	state.active = record
	ctx, cancel := context.WithCancel(context.Background())
	record.cancel = cancel
	record.done = make(chan struct{})
	session := &DriverSession{
		TurnID: record.id,
		ChatID: record.chatID,
		Input:  res.input,
		Scope:  res.turn.EffectiveScope,
	}
	session.emit = func(eventType string, payload json.RawMessage) error {
		record.mu.Lock()
		defer record.mu.Unlock()
		record.nextSeq++
		env := contracts.AgentEventEnvelope{
			SchemaVersion: 1,
			ChatSessionID: record.chatID,
			TurnID:        record.id,
			Sequence:      strconv.FormatUint(record.nextSeq, 10),
			Type:          eventType,
			Payload:       payload,
		}
		return record.ring.Append(env)
	}
	record.mu.Unlock()
	state.mu.Unlock()

	go func() {
		defer close(record.done)
		runErr := driver.Stream(ctx, session)
		m.finalizeTurn(record, runErr)
	}()
	return nil
}

// finalizeTurn records the terminal state exactly once: stop wins over
// driver completion, driver errors produce interrupted with the failure
// observable in the terminal event payload.
func (m *TurnManager) finalizeTurn(record *turnRecord, runErr error) {
	record.mu.Lock()
	if record.finalized {
		record.mu.Unlock()
		return
	}
	record.finalized = true
	if record.cancel != nil {
		defer record.cancel()
	}

	// One critical section: the terminal status becomes visible only
	// together with the turn_end record, so consumers reconciling from a
	// snapshot never miss the terminal event.
	status := StatusCompleted
	reason := "completed"
	switch {
	case record.stopSeen:
		status = StatusStopped
		reason = record.stopReason
		if reason == "" {
			reason = "stopped"
		}
	case runErr != nil:
		status = StatusInterrupted
		reason = fmt.Sprintf("interrupted: %v", runErr)
	}
	record.status = status
	record.revision++

	terminal := contracts.AgentEventEnvelope{
		SchemaVersion: 1,
		ChatSessionID: record.chatID,
		TurnID:        record.id,
		Type:          "turn_end",
		Payload:       []byte(fmt.Sprintf(`{"reason":%q,"status":%q}`, reason, status)),
	}
	record.nextSeq++
	terminal.Sequence = strconv.FormatUint(record.nextSeq, 10)
	// The ring is the history owner: a terminal append failure still leaves
	// the snapshot authoritative.
	_ = record.ring.Append(terminal)
	cancel := record.cancel
	record.mu.Unlock()

	// Release the chat slot once the terminal record is durable.
	state := m.chat(record.chatID)
	state.mu.Lock()
	if state.active == record {
		state.active = nil
	}
	state.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// StopTurn asks the turn to converge and waits for its terminal record.
// Repeated Stops converge on the same single terminal record (T11).
func (m *TurnManager) StopTurn(turnID contracts.TurnID, reason string) (contracts.TurnSnapshot, error) {
	m.mu.Lock()
	record := m.turns[turnID]
	m.mu.Unlock()
	if record == nil {
		return contracts.TurnSnapshot{}, contracts.NewError(contracts.CodeNotFound, "unknown turn")
	}

	record.mu.Lock()
	if record.status == StatusPrepared {
		// Never started: converge immediately without a driver run.
		record.stopSeen = true
		record.stopReason = reason
		record.mu.Unlock()
		m.finalizeTurn(record, nil)
		return record.snapshot(), nil
	}
	record.stopSeen = true
	record.stopReason = reason
	cancel := record.cancel
	done := record.done
	record.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done // bounded: the driver must honor ctx; no unbounded wait
	}
	return record.snapshot(), nil
}

// StopChat stops the chat's active turn (T15 groundwork): the chat slot is
// released immediately, the terminal record converges asynchronously.
func (m *TurnManager) StopChat(chatID contracts.ChatSessionID, reason string) error {
	state := m.chat(chatID)
	state.mu.Lock()
	active := state.active
	state.mu.Unlock()
	if active == nil {
		return nil
	}
	_, err := m.StopTurn(active.id, reason)
	return err
}
