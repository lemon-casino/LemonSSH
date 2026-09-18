package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/agent/drivers/fixture"
	"github.com/binaricat/netcatty/internal/agent/runtime"
	"github.com/binaricat/netcatty/internal/app/contracts"
)

func newTestAgentService(withDriver bool) *AgentService {
	manager := runtime.NewTurnManager()
	if withDriver {
		manager.SetDriver(fixture.New())
	}
	return newAgentService(manager, withDriver, newAttachmentRegistry(), nil)
}

func TestAgentStatusMirrorsDevFlag(t *testing.T) {
	if status := newTestAgentService(true).AgentStatus(); !status.GoRuntimeReady || !status.FixtureDriver {
		t.Errorf("dev-flagged service must report ready+fixture, got %+v", status)
	}
	if status := newTestAgentService(false).AgentStatus(); status.GoRuntimeReady || status.FixtureDriver {
		t.Errorf("release service must report not-ready, got %+v", status)
	}
}

func prepareVia(t *testing.T, service *AgentService) (contracts.TurnID, contracts.RequestID) {
	t.Helper()
	chat := contracts.NewChatSessionID()
	request := contracts.NewRequestID()
	turn, err := service.AgentPrepare(contracts.PrepareTurnRequest{
		RequestID:      request,
		ChatSessionID:  chat,
		AgentID:        contracts.AgentID("agnt_test"),
		Input:          contracts.TurnInput{Text: "hello"},
		RequestedScope: contracts.TurnScope{TerminalRead: true},
	})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	return turn.TurnID, request
}

// TestAgentServiceMinimalChain exercises the W12 loop through the Wails
// facade only: prepare, start, reconcile events after the live
// notifications are "missed", and read the terminal snapshot.
func TestAgentServiceMinimalChain(t *testing.T) {
	service := newTestAgentService(true)
	turnID, request := prepareVia(t, service)

	if err := service.AgentStart(contracts.TurnCommand{
		RequestID: request,
		Kind:      contracts.TurnCommandStart,
		TurnID:    turnID,
	}); err != nil {
		t.Fatalf("start: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var snapshot contracts.TurnSnapshot
	var page contracts.EventPage
	var err error
	for time.Now().Before(deadline) {
		snapshot, err = service.AgentSnapshot(string(turnID))
		if err == nil && (snapshot.Status == runtime.StatusCompleted) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if err != nil || snapshot.Status != runtime.StatusCompleted {
		t.Fatalf("turn did not complete: %+v (%v)", snapshot, err)
	}

	page, err = service.AgentReadEvents(string(turnID), "0", 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var text, terminal bool
	for _, event := range page.Events {
		switch event.Type {
		case "text_delta":
			text = strings.Contains(string(event.Payload), "fixture")
		case "turn_end":
			terminal = true
		}
	}
	if !text || !terminal {
		t.Errorf("fixture text=%v terminal=%v in %+v", text, terminal, page.Events)
	}
}

func TestAgentServiceStopConverges(t *testing.T) {
	manager := runtime.NewTurnManager()
	manager.SetDriver(&blockingFixture{})
	service := newAgentService(manager, false, newAttachmentRegistry(), nil)
	turnID, request := prepareVia(t, service)

	if err := service.AgentStart(contracts.TurnCommand{
		RequestID: request,
		Kind:      contracts.TurnCommandStart,
		TurnID:    turnID,
	}); err != nil {
		t.Fatalf("start: %v", err)
	}

	snapshot, err := service.AgentStop(string(turnID), "user requested")
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if snapshot.Status != runtime.StatusStopped {
		t.Errorf("status = %q, want stopped", snapshot.Status)
	}
}

type blockingFixture struct{}

func (blockingFixture) Stream(ctx context.Context, session *runtime.DriverSession) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestAgentServiceWithoutDriverUnavailable(t *testing.T) {
	service := newTestAgentService(false)
	turnID, request := prepareVia(t, service)

	err := service.AgentStart(contracts.TurnCommand{
		RequestID: request,
		Kind:      contracts.TurnCommandStart,
		TurnID:    turnID,
	})
	if err == nil {
		t.Fatal("start without driver must fail UNAVAILABLE (release builds never answer with fixture output)")
	}
	var contractErr *contracts.Error
	if e, ok := err.(*contracts.Error); ok {
		contractErr = e
	}
	if contractErr == nil || contractErr.Code != contracts.CodeUnavailable {
		t.Errorf("must fail UNAVAILABLE, got %v", err)
	}
}

func TestAgentServiceUnknownTurnNotFound(t *testing.T) {
	service := newTestAgentService(true)
	if _, err := service.AgentReadEvents("turn_missing", "0", 0); err == nil {
		t.Fatal("unknown turn must fail NOT_FOUND")
	}
}
