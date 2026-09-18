package main

import (
	"github.com/binaricat/netcatty/internal/agent/runtime"
	"github.com/binaricat/netcatty/internal/app/contracts"
)

// AgentService is the Wails-facing facade of the Go turn runtime (W12,
// P7-04, AI-03). It is a thin mapping: the runtime owns all invariants,
// the React client owns no second authoritative state. The five methods
// mirror the W03 wire DTOs one to one.
type AgentService struct {
	manager *runtime.TurnManager
}

func newAgentService(manager *runtime.TurnManager) *AgentService {
	return &AgentService{manager: manager}
}

// AgentPrepare reserves one turn slot. Idempotent per request ID.
func (s *AgentService) AgentPrepare(request contracts.PrepareTurnRequest) (contracts.PreparedTurn, error) {
	return s.manager.Prepare(request)
}

// AgentStart starts a prepared turn. Idempotent retries never re-run the
// driver.
func (s *AgentService) AgentStart(command contracts.TurnCommand) error {
	return s.manager.StartTurn(command)
}

// AgentStop converges one turn and returns its terminal snapshot.
func (s *AgentService) AgentStop(turnID, reason string) (contracts.TurnSnapshot, error) {
	return s.manager.StopTurn(contracts.TurnID(turnID), reason)
}

// AgentReadEvents pages turn events after a decimal-string cursor.
func (s *AgentService) AgentReadEvents(turnID, afterSequence string, limit int) (contracts.EventPage, error) {
	return s.manager.ReadEvents(contracts.TurnID(turnID), afterSequence, limit)
}

// AgentSnapshot returns the authoritative turn projection.
func (s *AgentService) AgentSnapshot(turnID string) (contracts.TurnSnapshot, error) {
	return s.manager.Snapshot(contracts.TurnID(turnID))
}
