package terminaluse

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// Attach flow (popup observe). The Electron shell implemented these primitives
// in its terminalBridge; under Wails the terminaluse service owns them:
//
//  1. AcquireSessionFlowPauseLease pauses renderer-bound output (bytes buffer
//     in Go) so no live output falls into the gap between the home renderer's
//     snapshot and the popup route handoff.
//  2. RequestSessionSnapshot asks the home renderer (event round-trip) to
//     serialize its scrollback; ApplySessionSnapshot pushes the popup's buffer
//     back home before the route is restored.
//  3. RebindSessionOutput rotates the data-plane route for the popup and kicks
//     the previous owner's socket; RestoreSessionOutput rotates the route back
//     home and kicks the popup's socket.
//
// Leases gate rebind/restore: every caller must hold a flow-pause lease, so a
// rogue window cannot rotate a session route it does not display.

const (
	attachRoundTripTimeout = 8 * time.Second
	flowPauseDrainTimeout  = 3 * time.Second
	flowPauseDrainInterval = 10 * time.Millisecond
)

// ActionResult is the shared attach success/error envelope.
type ActionResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// FlowPauseLease is returned by AcquireSessionFlowPauseLease. Authorization is
// the attach token minted once per attach; later rebind/restore calls present
// it (the renderer may pass it through verbatim).
type FlowPauseLease struct {
	Success       bool   `json:"success"`
	LeaseID       string `json:"leaseId,omitempty"`
	Authorization string `json:"authorization,omitempty"`
	Error         string `json:"error,omitempty"`
}

// FlowPauseReleaseOptions marks releases that must keep output paused (popup
// close failure path: the route is stale, so live bytes stay buffered).
type FlowPauseReleaseOptions struct {
	KeepPaused bool `json:"keepPaused,omitempty"`
}

// SnapshotContext is the popup display state pushed home by ApplySessionSnapshot.
type SnapshotContext struct {
	ContextSnapshot              string                  `json:"contextSnapshot"`
	ContextViewportSnapshot      string                  `json:"contextViewportSnapshot"`
	ContextScrollbackSnapshot    string                  `json:"contextScrollbackSnapshot"`
	AlternateScreen              bool                    `json:"alternateScreen"`
	KittyKeyboardModeState       *KittyKeyboardModeState `json:"kittyKeyboardModeState,omitempty"`
	KittyKeyboardProtocolEnabled *bool                   `json:"kittyKeyboardProtocolEnabled,omitempty"`
	PasswordPromptActive         *bool                   `json:"passwordPromptActive,omitempty"`
	Cwd                          *string                 `json:"cwd,omitempty"`
	Title                        *string                 `json:"title,omitempty"`
}

// KittyKeyboardModeState mirrors the renderer's kitty keyboard protocol state.
type KittyKeyboardModeState struct {
	MainFlags             int   `json:"mainFlags"`
	AlternateFlags        int   `json:"alternateFlags"`
	MainStack             []int `json:"mainStack"`
	AlternateStack        []int `json:"alternateStack"`
	AlternateScreenActive bool  `json:"alternateScreenActive"`
}

// SnapshotResult carries the home renderer's serialized scrollback.
type SnapshotResult struct {
	Success                      bool                    `json:"success"`
	Snapshot                     string                  `json:"snapshot,omitempty"`
	KittyKeyboardModeState       *KittyKeyboardModeState `json:"kittyKeyboardModeState,omitempty"`
	KittyKeyboardProtocolEnabled *bool                   `json:"kittyKeyboardProtocolEnabled,omitempty"`
	PasswordPromptActive         *bool                   `json:"passwordPromptActive,omitempty"`
	Cwd                          *string                 `json:"cwd,omitempty"`
	Title                        *string                 `json:"title,omitempty"`
	Error                        string                  `json:"error,omitempty"`
}

// RebindResult hands the rotated route bootstrap to the new display owner.
type RebindResult struct {
	Success       bool                   `json:"success"`
	Authorization string                 `json:"authorization,omitempty"`
	Route         *dataplaneRoutePayload `json:"route,omitempty"`
	Error         string                 `json:"error,omitempty"`
}

// dataplaneRoutePayload mirrors dataplane.RouteBootstrap without pulling the
// transport type into every binding signature.
type dataplaneRoutePayload struct {
	SessionID   string `json:"sessionID"`
	Generation  uint32 `json:"generation"`
	DataToken   string `json:"dataToken"`
	UrgentToken string `json:"urgentToken"`
	WindowBytes uint32 `json:"windowBytes"`
}

// pendingAttachReply is one renderer round-trip in flight.
type pendingAttachReply[T any] struct {
	respond chan T
}

func newPendingAttachReply[T any]() *pendingAttachReply[T] {
	return &pendingAttachReply[T]{respond: make(chan T, 1)}
}

func randomAttachToken() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// attachTerm resolves a native-or-alias session id and copies the native id.
func (s *Service) attachTerm(idOrAlias string) (string, *terminalSession, error) {
	nativeID, term, ok := s.resolveSessionID(idOrAlias)
	if !ok {
		return "", nil, fmt.Errorf("terminal session %q not found", idOrAlias)
	}
	return nativeID, term, nil
}

// authorizationMatches fails closed: a non-empty caller token must match the
// token minted for the attach. An empty token is tolerated for the tray-attach
// path whose payload predates attach authorizations.
func authorizationMatches(stored, provided string) error {
	if stored == "" || provided == "" {
		return nil
	}
	if stored != provided {
		return fmt.Errorf("attach authorization rejected")
	}
	return nil
}

// AcquireSessionFlowPauseLease pauses the session's renderer-bound output and
// returns a lease. The pause holds until the last lease is released.
func (s *Service) AcquireSessionFlowPauseLease(sessionID string) FlowPauseLease {
	_, term, err := s.attachTerm(sessionID)
	if err != nil {
		return FlowPauseLease{Error: err.Error()}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if term.closing {
		return FlowPauseLease{Error: "terminal session is closing"}
	}
	if term.flowLeases == nil {
		term.flowLeases = make(map[string]bool)
	}
	leaseID, err := randomAttachToken()
	if err != nil {
		return FlowPauseLease{Error: err.Error()}
	}
	term.flowLeases[leaseID] = true
	term.flowPaused = true
	if term.attachAuthorization == "" {
		token, tokenErr := randomAttachToken()
		if tokenErr != nil {
			delete(term.flowLeases, leaseID)
			term.flowPaused = len(term.flowLeases) > 0
			return FlowPauseLease{Error: tokenErr.Error()}
		}
		term.attachAuthorization = token
	}
	return FlowPauseLease{Success: true, LeaseID: leaseID, Authorization: term.attachAuthorization}
}

// WaitSessionFlowPauseLease waits until output queued before the pause reached
// the previous owner's socket, so the snapshot cannot miss pending bytes.
func (s *Service) WaitSessionFlowPauseLease(sessionID, leaseID string) ActionResult {
	nativeID, term, err := s.attachTerm(sessionID)
	if err != nil {
		return ActionResult{Error: err.Error()}
	}
	s.mu.Lock()
	_, leased := term.flowLeases[leaseID]
	s.mu.Unlock()
	if !leased {
		return ActionResult{Error: "flow pause lease is not active"}
	}
	deadline := time.Now().Add(flowPauseDrainTimeout)
	for time.Now().Before(deadline) {
		if s.dp.PendingBytes(nativeID) == 0 {
			return ActionResult{Success: true}
		}
		time.Sleep(flowPauseDrainInterval)
	}
	// Electron treated "did not settle" as a soft failure with a fallback
	// delay; surface it as unavailable so the renderer takes that path.
	return ActionResult{Error: "Output drain unavailable"}
}

// ReleaseSessionFlowPauseLease drops one lease; when the last lease goes and
// KeepPaused is false, buffered output flushes to the current route owner.
func (s *Service) ReleaseSessionFlowPauseLease(sessionID, leaseID string, options *FlowPauseReleaseOptions) ActionResult {
	_, term, err := s.attachTerm(sessionID)
	if err != nil {
		return ActionResult{Error: err.Error()}
	}
	keepPaused := options != nil && options.KeepPaused
	s.mu.Lock()
	if term.flowLeases == nil || !term.flowLeases[leaseID] {
		s.mu.Unlock()
		return ActionResult{Error: "flow pause lease is not active"}
	}
	delete(term.flowLeases, leaseID)
	remaining := len(term.flowLeases)
	if remaining == 0 && !keepPaused {
		term.flowPaused = false
		buffered := term.flowBuffer
		term.flowBuffer = nil
		s.mu.Unlock()
		s.flushFlowBuffer(sessionID, buffered)
		return ActionResult{Success: true}
	}
	s.mu.Unlock()
	return ActionResult{Success: true}
}

// forceReleaseFlowPause drops every lease and flushes (overflow guard).
func (s *Service) forceReleaseFlowPause(sessionID string) {
	s.mu.Lock()
	term, ok := s.sessions[sessionID]
	if !ok || !term.flowPaused {
		s.mu.Unlock()
		return
	}
	term.flowLeases = nil
	term.flowPaused = false
	buffered := term.flowBuffer
	term.flowBuffer = nil
	s.mu.Unlock()
	s.flushFlowBuffer(sessionID, buffered)
}

func (s *Service) flushFlowBuffer(sessionID string, buffered []byte) {
	if len(buffered) == 0 {
		return
	}
	_ = s.dp.Publish(sessionID, buffered)
}

// SetSessionFlowPaused pauses/resumes renderer-bound output without a lease.
// Used by the popup hibernate close path, which resumes explicitly.
func (s *Service) SetSessionFlowPaused(sessionID string, paused bool) error {
	_, term, err := s.attachTerm(sessionID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	term.flowPaused = paused
	var buffered []byte
	if !paused {
		buffered = term.flowBuffer
		term.flowBuffer = nil
	}
	s.mu.Unlock()
	if !paused {
		s.flushFlowBuffer(sessionID, buffered)
	}
	return nil
}

// SetSessionFlowPausedAndWait pauses and waits for the pre-pause queue to drain.
func (s *Service) SetSessionFlowPausedAndWait(sessionID string, paused bool) ActionResult {
	nativeID, _, err := s.attachTerm(sessionID)
	if err != nil {
		return ActionResult{Error: err.Error()}
	}
	if err := s.SetSessionFlowPaused(sessionID, paused); err != nil {
		return ActionResult{Error: err.Error()}
	}
	if !paused {
		return ActionResult{Success: true}
	}
	deadline := time.Now().Add(flowPauseDrainTimeout)
	for time.Now().Before(deadline) {
		if s.dp.PendingBytes(nativeID) == 0 {
			return ActionResult{Success: true}
		}
		time.Sleep(flowPauseDrainInterval)
	}
	return ActionResult{Error: "Output drain unavailable"}
}

// RequestSessionSnapshot asks the home renderer to serialize its scrollback
// and waits for the response.
func (s *Service) RequestSessionSnapshot(sessionID, authorization string) SnapshotResult {
	_, term, err := s.attachTerm(sessionID)
	if err != nil {
		return SnapshotResult{Error: err.Error()}
	}
	requestID, err := randomAttachToken()
	if err != nil {
		return SnapshotResult{Error: err.Error()}
	}
	reply := newPendingAttachReply[SnapshotResult]()
	s.mu.Lock()
	authErr := authorizationMatches(term.attachAuthorization, authorization)
	closing := term.closing
	if authErr == nil && !closing {
		if s.pendingSnapshots == nil {
			s.pendingSnapshots = make(map[string]*pendingAttachReply[SnapshotResult])
		}
		s.pendingSnapshots[requestID] = reply
	}
	s.mu.Unlock()
	if authErr != nil {
		return SnapshotResult{Error: authErr.Error()}
	}
	if closing {
		return SnapshotResult{Error: "terminal session is closing"}
	}
	defer s.dropPendingSnapshot(requestID)
	s.mu.Lock()
	uiID := attachEventSessionID(term, sessionID)
	s.mu.Unlock()
	s.emit("terminal:session-snapshot-request", map[string]any{
		"sessionId": uiID,
		"requestId": requestID,
	})
	select {
	case result := <-reply.respond:
		return result
	case <-time.After(attachRoundTripTimeout):
		return SnapshotResult{Error: "timed out waiting for the terminal snapshot"}
	}
}

func (s *Service) dropPendingSnapshot(requestID string) {
	s.mu.Lock()
	delete(s.pendingSnapshots, requestID)
	s.mu.Unlock()
}

// RespondSessionSnapshot completes a pending snapshot request (home renderer).
func (s *Service) RespondSessionSnapshot(
	requestID string,
	snapshot string,
	kittyState *KittyKeyboardModeState,
	kittyEnabled *bool,
	passwordPromptActive *bool,
	cwd *string,
	title *string,
) error {
	s.mu.Lock()
	reply, ok := s.pendingSnapshots[requestID]
	if ok {
		delete(s.pendingSnapshots, requestID)
	}
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown snapshot request %q", requestID)
	}
	reply.respond <- SnapshotResult{
		Success:                      true,
		Snapshot:                     snapshot,
		KittyKeyboardModeState:       kittyState,
		KittyKeyboardProtocolEnabled: kittyEnabled,
		PasswordPromptActive:         passwordPromptActive,
		Cwd:                          cwd,
		Title:                        title,
	}
	return nil
}

// ApplySessionSnapshot pushes the popup display state to the home renderer and
// waits for the apply result.
func (s *Service) ApplySessionSnapshot(sessionID, snapshot string, context SnapshotContext, authorization string) ActionResult {
	_, term, err := s.attachTerm(sessionID)
	if err != nil {
		return ActionResult{Error: err.Error()}
	}
	requestID, err := randomAttachToken()
	if err != nil {
		return ActionResult{Error: err.Error()}
	}
	reply := newPendingAttachReply[bool]()
	var eventPayload map[string]any
	var emitEvent bool
	s.mu.Lock()
	authErr := authorizationMatches(term.attachAuthorization, authorization)
	closing := term.closing
	if authErr == nil && !closing {
		if s.pendingApplies == nil {
			s.pendingApplies = make(map[string]*pendingAttachReply[bool])
		}
		s.pendingApplies[requestID] = reply
		uiID := attachEventSessionID(term, sessionID)
		eventPayload = map[string]any{
			"sessionId":                 uiID,
			"requestId":                 requestID,
			"snapshot":                  snapshot,
			"contextSnapshot":           context.ContextSnapshot,
			"contextViewportSnapshot":   context.ContextViewportSnapshot,
			"contextScrollbackSnapshot": context.ContextScrollbackSnapshot,
			"alternateScreen":           context.AlternateScreen,
			"kittyKeyboardModeState":    context.KittyKeyboardModeState,
			"passwordPromptActive":      context.PasswordPromptActive != nil && *context.PasswordPromptActive,
			"cwd":                       context.Cwd,
			"title":                     context.Title,
		}
		if context.KittyKeyboardProtocolEnabled != nil {
			eventPayload["kittyKeyboardProtocolEnabled"] = *context.KittyKeyboardProtocolEnabled
		}
		emitEvent = true
	}
	s.mu.Unlock()
	if !emitEvent {
		if authErr != nil {
			return ActionResult{Error: authErr.Error()}
		}
		return ActionResult{Error: "terminal session is closing"}
	}
	s.emit("terminal:session-apply-snapshot", eventPayload)
	select {
	case ok := <-reply.respond:
		if !ok {
			return ActionResult{Error: "home renderer rejected the terminal snapshot"}
		}
		return ActionResult{Success: true}
	case <-time.After(attachRoundTripTimeout):
		return ActionResult{Error: "timed out applying the terminal snapshot"}
	}
}

// RespondApplySnapshot completes a pending apply-snapshot push (home renderer).
func (s *Service) RespondApplySnapshot(requestID string, accepted bool) error {
	s.mu.Lock()
	reply, ok := s.pendingApplies[requestID]
	if ok {
		delete(s.pendingApplies, requestID)
	}
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown apply-snapshot request %q", requestID)
	}
	reply.respond <- accepted
	return nil
}

// MarkAttachPopupClosePrepared records that the popup began its close so the
// restore is idempotent and stays possible after the last lease is gone.
func (s *Service) MarkAttachPopupClosePrepared(sessionID, authorization string) ActionResult {
	_, term, err := s.attachTerm(sessionID)
	if err != nil {
		return ActionResult{Error: err.Error()}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if authErr := authorizationMatches(term.attachAuthorization, authorization); authErr != nil {
		return ActionResult{Error: authErr.Error()}
	}
	term.attachClosePrepared = true
	return ActionResult{Success: true}
}

// attachGate validates that the caller may rotate the display route: a held
// flow-pause lease, an explicit close preparation, or a simple pause.
func attachGate(term *terminalSession) error {
	if len(term.flowLeases) > 0 || term.flowPaused || term.attachClosePrepared {
		return nil
	}
	return fmt.Errorf("terminal attach requires an active flow-pause lease")
}

// RebindSessionOutput rotates the data-plane route for the popup: the previous
// owner's socket is kicked so it cannot keep consuming the shared queue.
func (s *Service) RebindSessionOutput(sessionID, authorization string) RebindResult {
	nativeID, term, err := s.attachTerm(sessionID)
	if err != nil {
		return RebindResult{Error: err.Error()}
	}
	s.mu.Lock()
	authErr := authorizationMatches(term.attachAuthorization, authorization)
	gateErr := attachGate(term)
	closing := term.closing
	s.mu.Unlock()
	if authErr != nil {
		return RebindResult{Error: authErr.Error()}
	}
	if gateErr != nil {
		return RebindResult{Error: gateErr.Error()}
	}
	if closing {
		return RebindResult{Error: "terminal session is closing"}
	}
	bootstrap, err := s.controller.Open(nativeID)
	if err != nil {
		return RebindResult{Error: err.Error()}
	}
	s.mu.Lock()
	term.bootstrap = bootstrap
	term.attachRebound = true
	uiID := attachEventSessionID(term, nativeID)
	auth := term.attachAuthorization
	s.mu.Unlock()
	s.emit("terminal:route-handoff", map[string]any{
		"sessionId":  uiID,
		"phase":      "detached",
		"generation": bootstrap.Generation,
	})
	s.dp.Kick(nativeID)
	return RebindResult{
		Success:       true,
		Authorization: auth,
		Route: &dataplaneRoutePayload{
			SessionID:   bootstrap.SessionID,
			Generation:  bootstrap.Generation,
			DataToken:   bootstrap.DataToken,
			UrgentToken: bootstrap.UrgentToken,
			WindowBytes: bootstrap.WindowBytes,
		},
	}
}

// RestoreSessionOutput rotates the route back to the home renderer, kicks the
// popup's socket, clears the attach state, and hands the new bootstrap to the
// waiting home window through the route-handoff event.
func (s *Service) RestoreSessionOutput(sessionID, authorization string) ActionResult {
	nativeID, term, err := s.attachTerm(sessionID)
	if err != nil {
		return ActionResult{Error: err.Error()}
	}
	s.mu.Lock()
	authErr := authorizationMatches(term.attachAuthorization, authorization)
	gateErr := attachGate(term)
	closing := term.closing
	s.mu.Unlock()
	if authErr != nil {
		return ActionResult{Error: authErr.Error()}
	}
	if gateErr != nil {
		return ActionResult{Error: gateErr.Error()}
	}
	if closing {
		return ActionResult{Error: "terminal session is closing"}
	}
	bootstrap, err := s.controller.Open(nativeID)
	if err != nil {
		return ActionResult{Error: err.Error()}
	}
	s.mu.Lock()
	term.bootstrap = bootstrap
	term.attachRebound = false
	term.attachClosePrepared = false
	uiID := attachEventSessionID(term, nativeID)
	paused := term.flowPaused
	var buffered []byte
	if !paused {
		buffered = term.flowBuffer
		term.flowBuffer = nil
	}
	s.mu.Unlock()
	s.emit("terminal:route-handoff", map[string]any{
		"sessionId": uiID,
		"phase":     "restored",
		"route": &dataplaneRoutePayload{
			SessionID:   bootstrap.SessionID,
			Generation:  bootstrap.Generation,
			DataToken:   bootstrap.DataToken,
			UrgentToken: bootstrap.UrgentToken,
			WindowBytes: bootstrap.WindowBytes,
		},
	})
	s.dp.Kick(nativeID)
	if len(buffered) > 0 {
		s.flushFlowBuffer(nativeID, buffered)
	}
	return ActionResult{Success: true}
}
