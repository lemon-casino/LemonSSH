package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/binaricat/lemonssh/internal/agent/drivers/fixture"
	"github.com/binaricat/lemonssh/internal/agent/runtime"
	"github.com/binaricat/lemonssh/internal/agent/tools"
	"github.com/binaricat/lemonssh/internal/app/contracts"
)

func newTestAgentService(withDriver bool) *AgentService {
	manager := runtime.NewTurnManager()
	if withDriver {
		manager.SetDriver(fixture.New())
	}
	return newAgentService(manager, withDriver, newAttachmentRegistry(), tools.NewOutputStore(tools.StoreOptions{}), nil)
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
	service := newAgentService(manager, false, newAttachmentRegistry(), tools.NewOutputStore(tools.StoreOptions{}), nil)
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

// TestAgentSetLiveProviderLifecycle pins the F06 product path: the
// renderer-pushed provider installs through the composition-root closure,
// AgentStatus flips without the fixture flag, and an empty payload clears
// the driver instead of leaving stale readiness behind.
func TestAgentSetLiveProviderLifecycle(t *testing.T) {
	service := newTestAgentService(false)
	installed := 0
	cleared := 0
	service.setLiveProviderInstaller(func(config ProviderConfig) error {
		installed++
		if config.Model == "reject-me" {
			return errors.New("endpoint refused")
		}
		return nil
	}, func() { cleared++ })

	if status := service.AgentStatus(); status.GoRuntimeReady || status.FixtureDriver {
		t.Fatalf("service without drivers must start not-ready, got %+v", status)
	}

	result := service.AgentSetLiveProvider(ProviderConfig{Endpoint: "https://api.example.com/v1/chat/completions", Model: "test-model"})
	if !result.OK || !result.Active || result.Error != "" {
		t.Fatalf("install must succeed, got %+v", result)
	}
	if installed != 1 {
		t.Fatalf("installer must run once, ran %d times", installed)
	}
	status := service.AgentStatus()
	if !status.GoRuntimeReady || status.FixtureDriver {
		t.Fatalf("live provider alone must report ready without the fixture flag, got %+v", status)
	}

	// Rejections keep the previous driver: a bad renderer push must not
	// take a working configuration down.
	result = service.AgentSetLiveProvider(ProviderConfig{Endpoint: "https://api.example.com/v1/chat/completions", Model: "reject-me"})
	if result.OK || result.Active || result.Error == "" {
		t.Fatalf("rejected install must fail typed, got %+v", result)
	}
	if !service.AgentStatus().GoRuntimeReady {
		t.Fatal("a rejected install must not clear the previous driver")
	}

	// Empty payload clears: removing the provider in Settings drops
	// goRuntimeReady back to false.
	result = service.AgentSetLiveProvider(ProviderConfig{})
	if !result.OK || result.Active || result.Error != "" {
		t.Fatalf("clear must succeed, got %+v", result)
	}
	if cleared != 1 {
		t.Fatalf("clear closure must run once, ran %d times", cleared)
	}
	if service.AgentStatus().GoRuntimeReady {
		t.Fatal("cleared service must report not-ready")
	}
}

func TestAgentSetLiveProviderPartialConfigRejected(t *testing.T) {
	service := newTestAgentService(false)
	installed := 0
	service.setLiveProviderInstaller(func(ProviderConfig) error { installed++; return nil }, nil)

	if result := service.AgentSetLiveProvider(ProviderConfig{Endpoint: "https://api.example.com"}); result.OK || result.Error == "" {
		t.Fatalf("endpoint-only config must be rejected, got %+v", result)
	}
	if installed != 0 {
		t.Fatalf("invalid config must not reach the installer, ran %d times", installed)
	}

	// Without a composition-root installer the push fails typed.
	bare := newTestAgentService(false)
	if result := bare.AgentSetLiveProvider(ProviderConfig{Endpoint: "https://api.example.com", Model: "m"}); result.OK || result.Error == "" {
		t.Fatalf("missing installer must fail typed, got %+v", result)
	}
}

// TestApplyLiveProvider mirrors the composition-root environment path: the
// same installer and readiness bookkeeping as the renderer push.
func TestApplyLiveProvider(t *testing.T) {
	service := newTestAgentService(false)
	service.setLiveProviderInstaller(func(ProviderConfig) error { return nil }, nil)
	if err := service.applyLiveProvider(ProviderConfig{Endpoint: "https://api.example.com", Model: "m"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	status := service.AgentStatus()
	if !status.GoRuntimeReady || status.FixtureDriver {
		t.Fatalf("applied env provider must report ready-only, got %+v", status)
	}

	failing := newTestAgentService(false)
	failing.setLiveProviderInstaller(func(ProviderConfig) error { return errors.New("policy refused the endpoint") }, nil)
	if err := failing.applyLiveProvider(ProviderConfig{Endpoint: "http://169.254.169.254", Model: "m"}); err == nil {
		t.Fatal("installer errors must propagate")
	}
	if failing.AgentStatus().GoRuntimeReady {
		t.Fatal("failed apply must not report ready")
	}
}
