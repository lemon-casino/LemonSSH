package main

import (
	"errors"

	"github.com/binaricat/netcatty/internal/agent/runtime"
	"github.com/binaricat/netcatty/internal/agent/tools"
	"github.com/binaricat/netcatty/internal/app/contracts"
)

// AgentService is the Wails-facing facade of the Go turn runtime (W12,
// P7-04, AI-03). It is a thin mapping: the runtime owns all invariants,
// the React client owns no second authoritative state. The five methods
// mirror the W03 wire DTOs one to one.
type AgentService struct {
	manager     *runtime.TurnManager
	attachments *AttachmentRegistry
	outputStore *tools.OutputStore
	router      *InteractionRouter
	// devDriver reports that the composition root wired the fixture
	// driver (NETCATTY_AI_DEV_DRIVER=1). The renderer routes the Catty
	// path to the Go runtime only when this is true, so a release build
	// keeps exactly one authoritative runtime.
	devDriver bool
}

// AgentStatus tells the renderer which Catty path is authoritative.
type AgentStatus struct {
	GoRuntimeReady bool `json:"goRuntimeReady"`
	FixtureDriver  bool `json:"fixtureDriver"`
}

// AgentStatus reports runtime readiness. Without the dev flag the Go
// runtime has no provider behind it, so the renderer must keep using its
// own chain until the live provider wiring lands (W13+).
func (s *AgentService) AgentStatus() AgentStatus {
	return AgentStatus{GoRuntimeReady: s.devDriver, FixtureDriver: s.devDriver}
}

func newAgentService(manager *runtime.TurnManager, devDriver bool, attachments *AttachmentRegistry, outputStore *tools.OutputStore, router *InteractionRouter) *AgentService {
	return &AgentService{manager: manager, devDriver: devDriver, attachments: attachments, outputStore: outputStore, router: router}
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

// AgentRegisterChatAttachments lets the renderer push the current chat's
// attachments into the host registry so agents can list/read them.
func (s *AgentService) AgentRegisterChatAttachments(chatSessionID string, attachments []Attachment) error {
	if chatSessionID == "" {
		return errors.New("chatSessionId is required")
	}
	s.attachments.Register(chatSessionID, attachments)
	return nil
}

// AgentPendingInteractions lists open approval prompts for the settings UI.
func (s *AgentService) AgentPendingInteractions() []map[string]any {
	if s.router == nil {
		return []map[string]any{}
	}
	return s.router.Pending()
}

// AgentRespondInteraction resolves one pending approval. The decision is
// consumed exactly once; unknown or stale IDs fail typed.
func (s *AgentService) AgentRespondInteraction(interactionID string, approved bool) error {
	if s.router == nil {
		return errors.New("approval routing is not available")
	}
	return s.router.Respond(interactionID, approved)
}

// AgentSnapshot returns the authoritative turn projection.
func (s *AgentService) AgentSnapshot(turnID string) (contracts.TurnSnapshot, error) {
	return s.manager.Snapshot(contracts.TurnID(turnID))
}
