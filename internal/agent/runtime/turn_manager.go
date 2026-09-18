// Package runtime implements the Go agent turn state machine (W11, P7-04,
// AI-03): prepare leases with idempotent retry, one active turn per chat,
// an event ring with cursor reconciliation and unified stop. The contracts
// package supplies the wire DTOs; this package owns the invariants.
package runtime

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/binaricat/netcatty/internal/app/contracts"
)

// LeaseTTL bounds how long a prepared reservation holds the chat slot
// (design candidate: 30 seconds; tunable at construction).
const LeaseTTL = 30 * time.Second

// chatState is the per-chat single-slot state.
type chatState struct {
	mu sync.Mutex
	// reservation is the outstanding prepared turn, nil once consumed by
	// Start or expired.
	reservation *reservation
	// active is the currently running turn, nil when idle.
	active *activeTurn
	// completed turns by request id keep idempotent retries answerable.
	byRequest map[contracts.RequestID]*reservation
}

type reservation struct {
	turn      contracts.PreparedTurn
	requestID contracts.RequestID
	inputJSON string // canonical params snapshot for conflict detection (T02)
	createdAt time.Time
	consumed  bool
}

type activeTurn struct {
	turnID   contracts.TurnID
	chatID   contracts.ChatSessionID
	sequence uint64
	terminal bool
	stopSeen bool
}

// TurnManager arbitrates prepare/start/stop across chats. Slice 2 adds the
// event ring and ReadEvents reconciliation; slice 3 the driver-driven
// lifecycle.
type TurnManager struct {
	now   func() time.Time
	lease time.Duration
	// RingCapacity bounds each turn's event ring; 0 means
	// DefaultRingCapacity. Test-only in practice until W14 byte budgets.
	RingCapacity int

	mu    sync.Mutex
	chats map[contracts.ChatSessionID]*chatState
	turns map[contracts.TurnID]*turnRecord
}

func NewTurnManager() *TurnManager {
	return &TurnManager{
		now:   time.Now,
		lease: LeaseTTL,
		chats: map[contracts.ChatSessionID]*chatState{},
		turns: map[contracts.TurnID]*turnRecord{},
	}
}

// turnRecord is the durable handle for one turn: identity, lifecycle
// status and its event ring. Status progresses prepared -> running ->
// terminal (slice 3); history stays readable after termination.
type turnRecord struct {
	id        contracts.TurnID
	chatID    contracts.ChatSessionID
	requestID contracts.RequestID
	status    string
	revision  uint64
	ring      *EventRing
}

func (r *turnRecord) snapshot() contracts.TurnSnapshot {
	return contracts.TurnSnapshot{
		Status:          r.status,
		Revision:        strconv.FormatUint(r.revision, 10),
		ThroughSequence: r.ring.ThroughSequence(),
	}
}

func (m *TurnManager) chat(id contracts.ChatSessionID) *chatState {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.chats[id]
	if state == nil {
		state = &chatState{byRequest: map[contracts.RequestID]*reservation{}}
		m.chats[id] = state
	}
	return state
}

// Prepare reserves one turn slot for the chat. Idempotency: the same
// request ID returns the same reservation; the same request ID with
// different parameters is a stale-revision conflict; a different request
// ID while the slot is held (active turn or live lease) is busy (T01/T02).
func (m *TurnManager) Prepare(req contracts.PrepareTurnRequest) (contracts.PreparedTurn, error) {
	state := m.chat(req.ChatSessionID)
	state.mu.Lock()
	defer state.mu.Unlock()

	if prior, ok := state.byRequest[req.RequestID]; ok {
		switch {
		case prior.inputJSON != canonicalRequestJSON(req):
			return contracts.PreparedTurn{}, contracts.NewError(contracts.CodeStaleRevision,
				fmt.Sprintf("request %s already prepared with different parameters", req.RequestID))
		case prior.consumed:
			return contracts.PreparedTurn{}, contracts.NewError(contracts.CodeConflict,
				fmt.Sprintf("request %s already started a turn", req.RequestID))
		case m.now().After(m.expiry(prior)):
			// Expired: fall through and re-reserve a fresh turn under the
			// same request ID (T03: the lease release must make Prepare
			// possible again).
		default:
			return prior.turn, nil
		}
	}

	if state.active != nil {
		return contracts.PreparedTurn{}, contracts.NewError(contracts.CodeBusy,
			"chat already has an active turn")
	}
	if state.reservation != nil && !m.now().After(m.expiry(state.reservation)) {
		if state.reservation.requestID == req.RequestID {
			return state.reservation.turn, nil
		}
		return contracts.PreparedTurn{}, contracts.NewError(contracts.CodeBusy,
			"chat already has a prepared turn lease")
	}

	turn := contracts.PreparedTurn{
		TurnID:                  contracts.NewTurnID(),
		LeaseExpiresAtMS:        m.now().Add(m.lease).UnixMilli(),
		Cursor:                  "0",
		SnapshotRevision:        "1",
		EffectiveScope:          req.RequestedScope,
		EffectiveConfigRevision: "1",
		PolicyRevision:          "1",
	}
	res := &reservation{
		turn:      turn,
		requestID: req.RequestID,
		inputJSON: canonicalRequestJSON(req),
		createdAt: m.now(),
	}
	state.reservation = res
	state.byRequest[req.RequestID] = res

	record := &turnRecord{
		id:        turn.TurnID,
		chatID:    req.ChatSessionID,
		requestID: req.RequestID,
		status:    StatusPrepared,
		revision:  1,
		ring:      NewEventRing(m.RingCapacity),
	}
	m.mu.Lock()
	m.turns[turn.TurnID] = record
	m.mu.Unlock()
	return turn, nil
}

// Turn lifecycle statuses carried on TurnSnapshot.Status.
const (
	StatusPrepared    = "prepared"
	StatusRunning     = "running"
	StatusCompleted   = "completed"
	StatusStopped     = "stopped"
	StatusInterrupted = "interrupted"
)

// Append stores one event on the turn's ring. Callers own the monotonic
// sequence for now (the driver sink in slice 3 wraps this); redelivery and
// conflict semantics are the ring's (T06).
func (m *TurnManager) Append(turnID contracts.TurnID, env contracts.AgentEventEnvelope) error {
	m.mu.Lock()
	record := m.turns[turnID]
	m.mu.Unlock()
	if record == nil {
		return contracts.NewError(contracts.CodeNotFound, "unknown turn")
	}
	return record.ring.Append(env)
}

// ReadEvents serves one page of turn events after the cursor. A cursor
// pointing into an evicted ring region comes back with CursorExpired and
// an authoritative snapshot so the consumer can resync (T04/T07).
func (m *TurnManager) ReadEvents(turnID contracts.TurnID, afterSequence string, limit int) (contracts.EventPage, error) {
	m.mu.Lock()
	record := m.turns[turnID]
	m.mu.Unlock()
	if record == nil {
		return contracts.EventPage{}, contracts.NewError(contracts.CodeNotFound, "unknown turn")
	}

	events, expired, err := record.ring.Read(afterSequence, limit)
	if err != nil {
		return contracts.EventPage{}, err
	}

	page := contracts.EventPage{Events: events}
	if len(events) > 0 {
		page.NextCursor = events[len(events)-1].Sequence
		page.HasMore = record.ring.hasMore(events[len(events)-1].Sequence)
	}
	if expired {
		snapshot := record.snapshot()
		page.CursorExpired = true
		page.Snapshot = &snapshot
		page.NextCursor = snapshot.ThroughSequence
		page.Events = nil
	}
	return page, nil
}

// Snapshot returns the authoritative turn projection.
func (m *TurnManager) Snapshot(turnID contracts.TurnID) (contracts.TurnSnapshot, error) {
	m.mu.Lock()
	record := m.turns[turnID]
	m.mu.Unlock()
	if record == nil {
		return contracts.TurnSnapshot{}, contracts.NewError(contracts.CodeNotFound, "unknown turn")
	}
	return record.snapshot(), nil
}

// expiry is derived from the reservation's own turn payload so the check
// uses the same value the client saw.
func (m *TurnManager) expiry(res *reservation) time.Time {
	return time.UnixMilli(res.turn.LeaseExpiresAtMS)
}

func canonicalRequestJSON(req contracts.PrepareTurnRequest) string {
	// Deterministic enough for conflict detection: the wire DTO fields that
	// define the turn's meaning.
	return string(req.RequestID) + "|" + string(req.ChatSessionID) + "|" + string(req.AgentID) +
		"|" + req.ProviderConfigID + "|" + req.ModelID + "|" + req.Input.Text +
		"|" + fmt.Sprint(req.Input.AttachmentIDs) + "|" + req.ExpectedChatRevision +
		"|" + scopeKey(req.RequestedScope)
}

func scopeKey(scope contracts.TurnScope) string {
	return fmt.Sprint(scope.TerminalRead, scope.TerminalWrite, scope.Exec, scope.SFTPRead, scope.SFTPWrite)
}
