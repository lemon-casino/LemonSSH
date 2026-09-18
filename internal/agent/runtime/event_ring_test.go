package runtime

import (
	"errors"
	"strconv"
	"testing"

	"github.com/binaricat/netcatty/internal/app/contracts"
)

func envelope(seq uint64, kind string, payload string) contracts.AgentEventEnvelope {
	return contracts.AgentEventEnvelope{
		SchemaVersion: 1,
		ChatSessionID: "chat_test",
		TurnID:        "turn_test",
		Sequence:      strconv.FormatUint(seq, 10),
		Type:          kind,
		Payload:       []byte(payload),
	}
}

func TestRingAppendAndReadBackfill(t *testing.T) {
	ring := NewEventRing(8)
	for i := uint64(1); i <= 5; i++ {
		if err := ring.Append(envelope(i, "text_delta", "x")); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	events, expired, err := ring.Read("0", 0)
	if err != nil || expired {
		t.Fatalf("read from start: %v %v", err, expired)
	}
	if len(events) != 5 {
		t.Fatalf("backfill delivered %d events, want 5 (T04)", len(events))
	}
	// Idempotent reconnect with the served cursor: no duplicates.
	events, expired, err = ring.Read("5", 0)
	if err != nil || expired || len(events) != 0 {
		t.Errorf("reconnect after cursor must be empty, got %d events (expired=%v)", len(events), expired)
	}
	if ring.ThroughSequence() != "5" {
		t.Errorf("through sequence = %q", ring.ThroughSequence())
	}
}

func TestRingLimitAndHasMore(t *testing.T) {
	ring := NewEventRing(8)
	for i := uint64(1); i <= 4; i++ {
		_ = ring.Append(envelope(i, "text_delta", "x"))
	}
	events, _, err := ring.Read("0", 2)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("limited page = %d events", len(events))
	}
	if !ring.hasMore(events[len(events)-1].Sequence) {
		t.Errorf("hasMore must be true with events beyond the page")
	}
	if ring.hasMore(ring.ThroughSequence()) {
		t.Errorf("hasMore must be false at the ring head")
	}
}

// TestRingRedelivery pins T06: identical redelivery of the newest event is
// ignored; the same sequence with different payload is a conflict.
func TestRingRedelivery(t *testing.T) {
	ring := NewEventRing(8)
	_ = ring.Append(envelope(1, "text_delta", "a"))

	if err := ring.Append(envelope(1, "text_delta", "a")); err != nil {
		t.Errorf("identical redelivery must be idempotent, got %v", err)
	}
	err := ring.Append(envelope(1, "text_delta", "different"))
	if !errors.Is(err, ErrSequenceConflict) {
		t.Errorf("same sequence different payload must conflict, got %v", err)
	}
	err = ring.Append(envelope(0, "text_delta", "older"))
	if !errors.Is(err, ErrSequenceBehind) {
		t.Errorf("older sequence must be rejected, got %v", err)
	}
}

// TestRingCursorExpired pins T07: a cursor pointing into an evicted
// region reports expired instead of silently skipping events.
func TestRingCursorExpired(t *testing.T) {
	ring := NewEventRing(4)
	for i := uint64(1); i <= 10; i++ {
		_ = ring.Append(envelope(i, "text_delta", "x"))
	}
	_, expired, err := ring.Read("2", 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !expired {
		t.Fatal("cursor 2 with oldest retained 7 must be expired")
	}

	events, expired, err := ring.Read("6", 0)
	if err != nil || expired {
		t.Fatalf("read at boundary: %v %v", err, expired)
	}
	if len(events) != 4 || sequenceOf(events[0]) != 7 {
		t.Errorf("boundary read = %d events starting at %v", len(events), sequenceOf(events[0]))
	}
}

// TestManagerMissedNotifications pins T04 end to end: the consumer misses
// the live notifications entirely and reconciles from ReadEvents without
// duplicates or gaps.
func TestManagerMissedNotifications(t *testing.T) {
	manager := NewTurnManager()
	chat := contracts.NewChatSessionID()
	turn, err := manager.Prepare(prepareRequest(chat, contracts.NewRequestID(), "hello"))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	// Driver-side: five events land while the consumer is blind.
	for i := uint64(1); i <= 5; i++ {
		if err := manager.Append(turn.TurnID, envelope(i, "text_delta", "x")); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	page, err := manager.ReadEvents(turn.TurnID, "0", 3)
	if err != nil {
		t.Fatalf("read page 1: %v", err)
	}
	if len(page.Events) != 3 || !page.HasMore {
		t.Fatalf("page 1 = %d events, hasMore %v", len(page.Events), page.HasMore)
	}

	page, err = manager.ReadEvents(turn.TurnID, page.NextCursor, 3)
	if err != nil {
		t.Fatalf("read page 2: %v", err)
	}
	if len(page.Events) != 2 || page.HasMore {
		t.Fatalf("page 2 = %d events, hasMore %v", len(page.Events), page.HasMore)
	}

	// Snapshot matches the ring head.
	snapshot, err := manager.Snapshot(turn.TurnID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.Status != StatusPrepared || snapshot.ThroughSequence != "5" {
		t.Errorf("snapshot = %+v", snapshot)
	}
}

// TestManagerExpiredCursorServesSnapshot pins T07 through the manager:
// an evicted cursor gets CursorExpired plus an authoritative snapshot and
// an empty event list.
func TestManagerExpiredCursorServesSnapshot(t *testing.T) {
	manager := NewTurnManager()
	manager.RingCapacity = 4
	chat := contracts.NewChatSessionID()
	turn, err := manager.Prepare(prepareRequest(chat, contracts.NewRequestID(), "hello"))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	for i := uint64(1); i <= 12; i++ {
		_ = manager.Append(turn.TurnID, envelope(i, "text_delta", "x"))
	}

	page, err := manager.ReadEvents(turn.TurnID, "3", 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !page.CursorExpired || page.Snapshot == nil || len(page.Events) != 0 {
		t.Fatalf("expired cursor must serve snapshot, got %+v", page)
	}
	if page.Snapshot.ThroughSequence != "12" || page.NextCursor != "12" {
		t.Errorf("snapshot page = %+v", page)
	}

	// Resync from the served cursor returns exactly the retained tail.
	resync, err := manager.ReadEvents(turn.TurnID, page.NextCursor, 0)
	if err != nil {
		t.Fatalf("resync: %v", err)
	}
	if len(resync.Events) != 0 || resync.CursorExpired {
		t.Errorf("resync at head must be empty, got %d events", len(resync.Events))
	}
}

func TestManagerUnknownTurn(t *testing.T) {
	manager := NewTurnManager()
	if _, err := manager.ReadEvents("turn_missing", "0", 0); err == nil {
		t.Fatal("unknown turn must fail NOT_FOUND")
	}
	if _, err := manager.Snapshot("turn_missing"); err == nil {
		t.Fatal("unknown turn snapshot must fail NOT_FOUND")
	}
	if err := manager.Append("turn_missing", envelope(1, "text_delta", "x")); err == nil {
		t.Fatal("unknown turn append must fail NOT_FOUND")
	}
}
