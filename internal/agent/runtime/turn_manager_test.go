package runtime

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/app/contracts"
)

func prepareRequest(chat contracts.ChatSessionID, requestID contracts.RequestID, text string) contracts.PrepareTurnRequest {
	return contracts.PrepareTurnRequest{
		RequestID:      requestID,
		ChatSessionID:  chat,
		AgentID:        contracts.AgentID("agnt_test"),
		ModelID:        "test-model",
		Input:          contracts.TurnInput{Text: text},
		RequestedScope: contracts.TurnScope{TerminalRead: true},
	}
}

func TestPrepareIdempotentRetryReturnsSameTurn(t *testing.T) {
	manager := NewTurnManager()
	chat := contracts.NewChatSessionID()
	request := contracts.NewRequestID()

	first, err := manager.Prepare(prepareRequest(chat, request, "hello"))
	if err != nil {
		t.Fatalf("first prepare: %v", err)
	}
	second, err := manager.Prepare(prepareRequest(chat, request, "hello"))
	if err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
	if first.TurnID != second.TurnID || first.Cursor != second.Cursor {
		t.Errorf("same request must return the same reservation: %v vs %v", first, second)
	}
}

func TestPrepareSameRequestChangedParamsConflicts(t *testing.T) {
	manager := NewTurnManager()
	chat := contracts.NewChatSessionID()
	request := contracts.NewRequestID()

	if _, err := manager.Prepare(prepareRequest(chat, request, "hello")); err != nil {
		t.Fatalf("first prepare: %v", err)
	}
	_, err := manager.Prepare(prepareRequest(chat, request, "changed"))
	var contractErr *contracts.Error
	if err == nil {
		t.Fatal("changed params must conflict")
	}
	if e, ok := err.(*contracts.Error); ok {
		contractErr = e
	}
	if contractErr == nil || contractErr.Code != contracts.CodeStaleRevision {
		t.Errorf("changed params must fail STALE_REVISION, got %v", err)
	}
}

func TestPrepareSecondRequestWhileLeaseLiveIsBusy(t *testing.T) {
	manager := NewTurnManager()
	chat := contracts.NewChatSessionID()

	if _, err := manager.Prepare(prepareRequest(chat, contracts.NewRequestID(), "first")); err != nil {
		t.Fatalf("first prepare: %v", err)
	}
	_, err := manager.Prepare(prepareRequest(chat, contracts.NewRequestID(), "second"))
	if err == nil {
		t.Fatal("second different request while lease live must be busy")
	}
	var contractErr *contracts.Error
	if e, ok := err.(*contracts.Error); ok {
		contractErr = e
	}
	if contractErr == nil || contractErr.Code != contracts.CodeBusy {
		t.Errorf("must fail BUSY, got %v", err)
	}
}

// TestConcurrentPrepareSingleReservation pins T01: two different request
// IDs preparing at the same instant resolve to exactly one reservation.
func TestConcurrentPrepareSingleReservation(t *testing.T) {
	manager := NewTurnManager()
	chat := contracts.NewChatSessionID()

	var wg sync.WaitGroup
	results := make(chan error, 2)
	turns := make(chan contracts.TurnID, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			turn, err := manager.Prepare(prepareRequest(chat, contracts.NewRequestID(), "race"))
			results <- err
			turns <- turn.TurnID
		}()
	}
	wg.Wait()
	close(results)
	close(turns)

	successes := 0
	var winner contracts.TurnID
	for err := range results {
		if err == nil {
			successes++
		} else if !strings.Contains(err.Error(), string(contracts.CodeBusy)) {
			t.Errorf("unexpected concurrent prepare error: %v", err)
		}
	}
	for id := range turns {
		if id != "" {
			winner = id
		}
	}
	if successes != 1 || winner == "" {
		t.Errorf("exactly one reservation must win, got %d successes (turn %q)", successes, winner)
	}
}

// TestLeaseExpiryReleasesSlot pins T03: after the lease lapses the next
// Prepare succeeds and no phantom terminal record exists.
func TestLeaseExpiryReleasesSlot(t *testing.T) {
	manager := NewTurnManager()
	now := time.UnixMilli(1_000_000)
	manager.now = func() time.Time { return now }
	chat := contracts.NewChatSessionID()
	request := contracts.NewRequestID()

	first, err := manager.Prepare(prepareRequest(chat, request, "hello"))
	if err != nil {
		t.Fatalf("first prepare: %v", err)
	}

	// Within the lease: same request still returns the reservation.
	now = now.Add(10 * time.Second)
	if _, err := manager.Prepare(prepareRequest(chat, request, "hello")); err != nil {
		t.Fatalf("retry within lease: %v", err)
	}

	// After expiry: a different request takes the slot; the original
	// request re-prepares as a fresh turn (no phantom turn end anywhere).
	now = now.Add(LeaseTTL + time.Second)
	other, err := manager.Prepare(prepareRequest(chat, contracts.NewRequestID(), "other"))
	if err != nil {
		t.Fatalf("prepare after expiry: %v", err)
	}
	if other.TurnID == first.TurnID {
		t.Errorf("expired lease must yield a fresh turn, got %v again", other.TurnID)
	}
}
