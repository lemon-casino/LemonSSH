package runtime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/app/contracts"
)

// echoDriver streams a fixed number of text events then finishes; it
// honors ctx cancellation as an interruption.
type echoDriver struct {
	events int
	runs   atomic.Int32
	// block makes Stream wait on ctx before finishing (stop tests).
	block bool
}

func (d *echoDriver) Stream(ctx context.Context, session *DriverSession) error {
	d.runs.Add(1)
	if d.block {
		<-ctx.Done()
		return ctx.Err()
	}
	for i := 0; i < d.events; i++ {
		if err := session.Emit("text_delta", []byte{byte('a' + i)}); err != nil {
			return err
		}
	}
	return nil
}

func startRequest(chat contracts.ChatSessionID, requestID contracts.RequestID, turn contracts.TurnID) contracts.TurnCommand {
	return contracts.TurnCommand{
		RequestID: requestID,
		Kind:      contracts.TurnCommandStart,
		TurnID:    turn,
	}
}

func waitForTerminal(t *testing.T, manager *TurnManager, turnID contracts.TurnID) contracts.TurnSnapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := manager.Snapshot(turnID)
		if err == nil {
			switch snapshot.Status {
			case StatusCompleted, StatusStopped, StatusInterrupted:
				return snapshot
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("turn never reached a terminal status")
	return contracts.TurnSnapshot{}
}

func TestStartRunsDriverAndRecordsTerminal(t *testing.T) {
	manager := NewTurnManager()
	manager.SetDriver(&echoDriver{events: 3})
	chat := contracts.NewChatSessionID()
	request := contracts.NewRequestID()

	turn, err := manager.Prepare(prepareRequest(chat, request, "hello"))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := manager.StartTurn(startRequest(chat, request, turn.TurnID)); err != nil {
		t.Fatalf("start: %v", err)
	}

	snapshot := waitForTerminal(t, manager, turn.TurnID)
	if snapshot.Status != StatusCompleted {
		t.Errorf("status = %q, want completed", snapshot.Status)
	}
	page, err := manager.ReadEvents(turn.TurnID, "0", 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// 3 text deltas + 1 terminal record; sequences strictly increasing.
	if len(page.Events) != 4 {
		t.Fatalf("events = %d, want 4", len(page.Events))
	}
	last := page.Events[len(page.Events)-1]
	if last.Type != "turn_end" {
		t.Errorf("terminal event type = %q", last.Type)
	}
	for i := 1; i < len(page.Events); i++ {
		if page.Events[i].Sequence <= page.Events[i-1].Sequence {
			t.Errorf("sequences not increasing at %d", i)
		}
	}
}

// TestStartIdempotentRetrySingleRun pins T02's Start face: retrying Start
// with the same request never runs the driver twice.
func TestStartIdempotentRetrySingleRun(t *testing.T) {
	manager := NewTurnManager()
	driver := &echoDriver{events: 1}
	manager.SetDriver(driver)
	chat := contracts.NewChatSessionID()
	request := contracts.NewRequestID()

	turn, _ := manager.Prepare(prepareRequest(chat, request, "hello"))
	for i := 0; i < 3; i++ {
		if err := manager.StartTurn(startRequest(chat, request, turn.TurnID)); err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
	}
	waitForTerminal(t, manager, turn.TurnID)
	if got := driver.runs.Load(); got != 1 {
		t.Errorf("driver ran %d times, want exactly 1", got)
	}
}

func TestStartUnknownRequestFails(t *testing.T) {
	manager := NewTurnManager()
	manager.SetDriver(&echoDriver{events: 1})
	if err := manager.StartTurn(startRequest(contracts.NewChatSessionID(), contracts.NewRequestID(), "turn_x")); err == nil {
		t.Fatal("unknown request must fail NOT_FOUND")
	}
}

func TestStartWithoutDriverUnavailable(t *testing.T) {
	manager := NewTurnManager()
	chat := contracts.NewChatSessionID()
	request := contracts.NewRequestID()
	turn, _ := manager.Prepare(prepareRequest(chat, request, "hello"))
	err := manager.StartTurn(startRequest(chat, request, turn.TurnID))
	if err == nil {
		t.Fatal("start without driver must fail unavailable")
	}
}

// TestStopDuringStreamConverges pins T11: Stop at any point yields exactly
// one terminal record with status stopped, and the driver observes the
// cancellation.
func TestStopDuringStreamConverges(t *testing.T) {
	manager := NewTurnManager()
	manager.SetDriver(&echoDriver{block: true})
	chat := contracts.NewChatSessionID()
	request := contracts.NewRequestID()

	turn, _ := manager.Prepare(prepareRequest(chat, request, "hello"))
	if err := manager.StartTurn(startRequest(chat, request, turn.TurnID)); err != nil {
		t.Fatalf("start: %v", err)
	}

	snapshot, err := manager.StopTurn(turn.TurnID, "user requested")
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if snapshot.Status != StatusStopped {
		t.Errorf("status = %q, want stopped", snapshot.Status)
	}

	// Repeated Stops converge on the same terminal state without new
	// records.
	for i := 0; i < 3; i++ {
		again, err := manager.StopTurn(turn.TurnID, "again")
		if err != nil || again.Status != StatusStopped {
			t.Errorf("repeat stop %d: %v %v", i, err, again.Status)
		}
	}
	page, _ := manager.ReadEvents(turn.TurnID, "0", 0)
	terminal := 0
	for _, ev := range page.Events {
		if ev.Type == "turn_end" {
			terminal++
		}
	}
	if terminal != 1 {
		t.Errorf("terminal records = %d, want exactly 1", terminal)
	}
}

// mixedDriver blocks the chat "a" turn until cancelled and completes every
// other turn immediately, making the isolation test deterministic.
type mixedDriver struct{}

func (mixedDriver) Stream(ctx context.Context, session *DriverSession) error {
	if session.Input.Text == "a" {
		<-ctx.Done()
		return ctx.Err()
	}
	_ = session.Emit("text_delta", []byte("done"))
	return nil
}

// TestStopIsolatedAcrossChats pins the isolation half of T11: stopping one
// chat's turn never touches another chat's running turn.
func TestStopIsolatedAcrossChats(t *testing.T) {
	manager := NewTurnManager()
	manager.SetDriver(mixedDriver{})

	chatA := contracts.NewChatSessionID()
	chatB := contracts.NewChatSessionID()
	reqA, reqB := contracts.NewRequestID(), contracts.NewRequestID()
	turnA, _ := manager.Prepare(prepareRequest(chatA, reqA, "a"))
	turnB, _ := manager.Prepare(prepareRequest(chatB, reqB, "b"))
	if err := manager.StartTurn(startRequest(chatA, reqA, turnA.TurnID)); err != nil {
		t.Fatalf("start a: %v", err)
	}
	if err := manager.StartTurn(startRequest(chatB, reqB, turnB.TurnID)); err != nil {
		t.Fatalf("start b: %v", err)
	}

	if _, err := manager.StopTurn(turnA.TurnID, "only a"); err != nil {
		t.Fatalf("stop a: %v", err)
	}

	if got := waitForTerminal(t, manager, turnA.TurnID).Status; got != StatusStopped {
		t.Errorf("chat a status = %q", got)
	}
	if got := waitForTerminal(t, manager, turnB.TurnID).Status; got != StatusCompleted {
		t.Errorf("chat b status = %q, want completed (isolation)", got)
	}
}

// TestDriverErrorInterrupted pins the crash path: a driver failure lands
// as interrupted with the failure visible, not as fake success.
func TestDriverErrorInterrupted(t *testing.T) {
	manager := NewTurnManager()
	manager.SetDriver(failDriver{})
	chat := contracts.NewChatSessionID()
	request := contracts.NewRequestID()
	turn, _ := manager.Prepare(prepareRequest(chat, request, "hello"))
	if err := manager.StartTurn(startRequest(chat, request, turn.TurnID)); err != nil {
		t.Fatalf("start: %v", err)
	}
	if got := waitForTerminal(t, manager, turn.TurnID).Status; got != StatusInterrupted {
		t.Errorf("status = %q, want interrupted", got)
	}
}

type failDriver struct{}

func (failDriver) Stream(ctx context.Context, session *DriverSession) error {
	return errors.New("provider connection reset")
}

// TestStopChatWithBlockingDriver exercises the chat-slot stop path.
func TestStopChatWithBlockingDriver(t *testing.T) {
	manager := NewTurnManager()
	manager.SetDriver(&echoDriver{block: true})
	chat := contracts.NewChatSessionID()
	request := contracts.NewRequestID()
	turn, _ := manager.Prepare(prepareRequest(chat, request, "hello"))
	if err := manager.StartTurn(startRequest(chat, request, turn.TurnID)); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := manager.StopChat(chat, "chat deleted"); err != nil {
		t.Fatalf("stop chat: %v", err)
	}
	if got := waitForTerminal(t, manager, turn.TurnID).Status; got != StatusStopped {
		t.Errorf("status = %q, want stopped", got)
	}
	// Idle chat stop is a no-op.
	if err := manager.StopChat(chat, "again"); err != nil {
		t.Errorf("idle chat stop must be a no-op, got %v", err)
	}
}

// TestConcurrentStopAndComplete pins the finalize-once race: the driver
// finishing naturally while Stop lands still yields exactly one terminal
// record.
func TestConcurrentStopAndComplete(t *testing.T) {
	gate := make(chan struct{})
	manager := NewTurnManager()
	manager.SetDriver(gatedDriver{gate: gate})
	chat := contracts.NewChatSessionID()
	request := contracts.NewRequestID()
	turn, _ := manager.Prepare(prepareRequest(chat, request, "hello"))
	if err := manager.StartTurn(startRequest(chat, request, turn.TurnID)); err != nil {
		t.Fatalf("start: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = manager.StopTurn(turn.TurnID, "raced stop")
	}()
	go func() {
		defer wg.Done()
		close(gate) // driver completes concurrently with the stop
	}()
	wg.Wait()

	snapshot := waitForTerminal(t, manager, turn.TurnID)
	page, _ := manager.ReadEvents(turn.TurnID, "0", 0)
	terminal := 0
	for _, ev := range page.Events {
		if ev.Type == "turn_end" {
			terminal++
		}
	}
	if terminal != 1 {
		t.Errorf("terminal records = %d, want exactly 1 (status %q)", terminal, snapshot.Status)
	}
}

// gatedDriver finishes as soon as the gate closes.
type gatedDriver struct{ gate chan struct{} }

func (d gatedDriver) Stream(ctx context.Context, session *DriverSession) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-d.gate:
		return nil
	}
}
