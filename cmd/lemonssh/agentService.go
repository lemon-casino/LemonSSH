package main

import (
	"errors"
	"strings"
	"sync"

	"github.com/lemon-casino/lemonssh/internal/agent/runtime"
	"github.com/lemon-casino/lemonssh/internal/agent/tools"
	"github.com/lemon-casino/lemonssh/internal/app/contracts"
)

// AgentService is the Wails-facing facade of the Go turn runtime (W12,
// P7-04, AI-03). It is a thin mapping: the runtime owns all invariants,
// the React client owns no second authoritative state. The five methods
// mirror the W03 wire DTOs one to one.
type AgentService struct {
	host        *AgentHost
	manager     *runtime.TurnManager
	attachments *AttachmentRegistry
	outputStore *tools.OutputStore
	router      *InteractionRouter
	// devDriver reports that the composition root wired the fixture
	// driver (LEMONSSH_AI_DEV_DRIVER=1). The renderer routes LemonSSH
	// path to the Go runtime only when a driver is behind the turn
	// manager, so a release build without a provider keeps exactly one
	// authoritative runtime (the renderer chain).
	devDriver bool
	// liveProvider state (guarded by liveMu). The composition root
	// installs the install/clear closures once the host and dispatcher
	// exist; the renderer pushes the active Settings→AI provider through
	// AgentSetLiveProvider so the Go runtime serves product LemonSSH turns.
	liveMu      sync.Mutex
	liveInstall func(ProviderConfig) error
	liveClear   func()
	liveReady   bool
}

// AgentStatus tells the renderer which LemonSSH path is authoritative.
type AgentStatus struct {
	GoRuntimeReady bool `json:"goRuntimeReady"`
	FixtureDriver  bool `json:"fixtureDriver"`
}

// AgentStatus reports runtime readiness: true when a driver is installed
// behind the turn manager — the dev fixture (LEMONSSH_AI_DEV_DRIVER=1) or
// a live provider (the LEMONSSH_AI_PROVIDER_JSON environment config or the
// provider configured in Settings→AI pushed through AgentSetLiveProvider).
// Without a driver the renderer keeps using its own chain.
func (s *AgentService) AgentStatus() AgentStatus {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	return AgentStatus{GoRuntimeReady: s.devDriver || s.liveReady, FixtureDriver: s.devDriver}
}

// AgentLiveProviderResult reports the outcome of a renderer-pushed live
// provider install.
type AgentLiveProviderResult struct {
	OK     bool   `json:"ok"`
	Active bool   `json:"active"`
	Error  string `json:"error,omitempty"`
}

// setLiveProviderInstaller wires the composition-root closures that turn a
// ProviderConfig into a live driver (netpolicy client, host sessions,
// capability dispatcher live there). Must be called before the service is
// exposed to the renderer.
func (s *AgentService) setLiveProviderInstaller(install func(ProviderConfig) error, clear func()) {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	s.liveInstall = install
	s.liveClear = clear
}

// AgentSetLiveProvider installs (or clears) the renderer-configured live
// provider behind the Go turn runtime. An empty payload clears the current
// driver, so removing the active provider in Settings drops
// AgentStatus.goRuntimeReady back to false instead of leaving a stale
// driver behind. Invalid or rejected configs keep the previous driver
// (the renderer chain stays the fallback for the affected turn).
func (s *AgentService) AgentSetLiveProvider(config ProviderConfig) AgentLiveProviderResult {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	if strings.TrimSpace(config.Endpoint) == "" && strings.TrimSpace(config.Model) == "" {
		s.clearLiveProviderLocked()
		return AgentLiveProviderResult{OK: true}
	}
	if s.liveInstall == nil {
		return AgentLiveProviderResult{Error: "live provider wiring is not available in this shell"}
	}
	if strings.TrimSpace(config.Endpoint) == "" || strings.TrimSpace(config.Model) == "" {
		return AgentLiveProviderResult{Error: "live provider config requires endpoint and model"}
	}
	if err := s.liveInstall(config); err != nil {
		return AgentLiveProviderResult{Error: err.Error()}
	}
	s.liveReady = true
	return AgentLiveProviderResult{OK: true, Active: true}
}

// applyLiveProvider is the composition-root path (environment config). It
// shares the installer and readiness bookkeeping with the renderer path so
// AgentStatus reports both identically.
func (s *AgentService) applyLiveProvider(config ProviderConfig) error {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	if s.liveInstall == nil {
		return errors.New("live provider wiring is not available")
	}
	if err := s.liveInstall(config); err != nil {
		return err
	}
	s.liveReady = true
	return nil
}

func (s *AgentService) clearLiveProviderLocked() {
	if s.liveClear != nil {
		s.liveClear()
	}
	s.liveReady = false
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
