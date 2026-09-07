package main

import (
	"errors"
	"fmt"
	"net/url"
	"time"
)

const popupHandoffTTL = 30 * time.Second
const popupRecoveryTTL = 30 * time.Second

type popupHandoffState string

const (
	popupStateCreated    popupHandoffState = "created"
	popupStateClaimed    popupHandoffState = "claimed"
	popupStateReplacing  popupHandoffState = "replacing"
	popupStateReplaced   popupHandoffState = "replaced"
	popupStateCompleting popupHandoffState = "completing"
	popupStateAborting   popupHandoffState = "aborting"
	popupStateExpired    popupHandoffState = "expired"
)

type popupHandoff struct {
	id               string
	completionToken  string
	cancelToken      string
	sessionID        string
	lastApplied      uint64
	clientState      string
	workload         string
	popupWindowName  string
	sourceWindowName string
	expiresAt        time.Time
	state            popupHandoffState
	abortRequested   bool
}

type popupRecovery struct {
	cancelToken       string
	sessionID         string
	lastApplied       uint64
	routeReplaced     bool
	expiresAt         time.Time
	issuedGeneration  uint32
	confirmationToken string
}

type PopupHandoffInfo struct {
	HandoffID           string `json:"handoffId"`
	PopupWindowName     string `json:"popupWindowName"`
	ExpiresAtUnixMillis int64  `json:"expiresAtUnixMillis"`
	CancelToken         string `json:"cancelToken"`
}

type PopupHandoffStatus struct {
	State string `json:"state"`
}

type PopupAbortResult struct {
	Bootstrap         RouteBootstrap `json:"bootstrap"`
	ConfirmationToken string         `json:"confirmationToken"`
}

type ClaimedPopupHandoff struct {
	HandoffID       string `json:"handoffId"`
	CompletionToken string `json:"completionToken"`
	SessionID       string `json:"sessionId"`
	LastApplied     uint64 `json:"lastApplied"`
	ClientState     string `json:"clientState"`
	Workload        string `json:"workload"`
	PopupWindowName string `json:"popupWindowName"`
}

func (p *ProbeService) CreatePopupHandoff(sessionID string, lastApplied uint64, clientState string) (PopupHandoffInfo, error) {
	if len(clientState) > maxHandoffStateBytes {
		return PopupHandoffInfo{}, fmt.Errorf("client handoff state exceeds %d bytes", maxHandoffStateBytes)
	}
	p.expirePopupHandoffs()
	p.mu.Lock()
	if len(p.handoffs) >= maxPopupHandoffs {
		p.mu.Unlock()
		return PopupHandoffInfo{}, errors.New("popup handoff limit reached")
	}
	session := p.sessions[sessionID]
	if session == nil || session.stopped || session.route == nil {
		p.mu.Unlock()
		return PopupHandoffInfo{}, errors.New("session is not active")
	}
	if !session.route.drainReady || session.route.drainTarget != lastApplied || session.applied != lastApplied {
		p.mu.Unlock()
		return PopupHandoffInfo{}, errors.New("session route must be drained through the acknowledged prefix")
	}
	host := p.windowHost
	if host == nil {
		p.mu.Unlock()
		return PopupHandoffInfo{}, errors.New("Wails window host is not attached")
	}
	handoffID, err := randomHex(16)
	if err != nil {
		p.mu.Unlock()
		return PopupHandoffInfo{}, err
	}
	completionToken, err := randomHex(24)
	if err != nil {
		p.mu.Unlock()
		return PopupHandoffInfo{}, err
	}
	cancelToken, err := randomHex(24)
	if err != nil {
		p.mu.Unlock()
		return PopupHandoffInfo{}, err
	}
	now := p.now()
	entry := &popupHandoff{
		id: handoffID, completionToken: completionToken, cancelToken: cancelToken, sessionID: sessionID,
		lastApplied: lastApplied, clientState: clientState,
		workload:         session.workloadID,
		popupWindowName:  "terminal-data-plane-popup-" + handoffID[:12],
		sourceWindowName: mainWindowName, expiresAt: now.Add(p.handoffTTL), state: popupStateCreated,
	}
	p.handoffs[handoffID] = entry
	p.mu.Unlock()

	path := "/?handoff=" + url.QueryEscape(handoffID)
	if err := host.OpenPopup(entry.popupWindowName, path, func() { p.handlePopupClosed(handoffID) }); err != nil {
		p.mu.Lock()
		delete(p.handoffs, handoffID)
		p.mu.Unlock()
		return PopupHandoffInfo{}, err
	}
	p.wakeHandoffCleanupLoop()
	return PopupHandoffInfo{
		HandoffID: handoffID, PopupWindowName: entry.popupWindowName,
		ExpiresAtUnixMillis: entry.expiresAt.UnixMilli(), CancelToken: cancelToken,
	}, nil
}

func (p *ProbeService) ClaimPopupHandoff(handoffID string) (ClaimedPopupHandoff, error) {
	p.expirePopupHandoffs()
	p.mu.Lock()
	defer p.mu.Unlock()
	entry := p.handoffs[handoffID]
	if entry == nil || entry.state != popupStateCreated {
		return ClaimedPopupHandoff{}, errors.New("popup handoff is invalid or already consumed")
	}
	entry.state = popupStateClaimed
	return ClaimedPopupHandoff{
		HandoffID: entry.id, CompletionToken: entry.completionToken,
		SessionID: entry.sessionID, LastApplied: entry.lastApplied,
		ClientState: entry.clientState, Workload: entry.workload, PopupWindowName: entry.popupWindowName,
	}, nil
}

func (p *ProbeService) CompletePopupHandoff(handoffID, completionToken string) error {
	p.expirePopupHandoffs()
	p.mu.Lock()
	entry := p.handoffs[handoffID]
	if entry == nil || entry.state != popupStateReplaced || entry.completionToken != completionToken {
		p.mu.Unlock()
		return errors.New("popup handoff completion is invalid")
	}
	entry.state = popupStateCompleting
	host := p.windowHost
	sourceWindowName := entry.sourceWindowName
	p.mu.Unlock()
	if err := host.CloseWindow(sourceWindowName); err != nil {
		p.mu.Lock()
		if current := p.handoffs[handoffID]; current != nil && current.state == popupStateCompleting {
			current.state = popupStateReplaced
		}
		p.mu.Unlock()
		return err
	}
	p.mu.Lock()
	entry = p.handoffs[handoffID]
	if entry != nil && entry.state == popupStateCompleting && entry.completionToken == completionToken && !entry.abortRequested {
		if session := p.sessions[entry.sessionID]; session != nil && session.route != nil {
			session.route.handoffPending = false
			p.wakeLocked(session)
		}
		delete(p.handoffs, handoffID)
	} else if entry != nil && entry.state == popupStateCompleting {
		entry.state = popupStateExpired
		delete(p.handoffs, handoffID)
		p.addPopupRecoveryLocked(handoffID, entry, true)
		p.mu.Unlock()
		p.wakeHandoffCleanupLoop()
		return errors.New("popup completion was interrupted")
	}
	p.mu.Unlock()
	p.wakeHandoffCleanupLoop()
	return nil
}

func (p *ProbeService) ResumePopupHandoff(handoffID, completionToken string, lastApplied uint64) (RouteBootstrap, error) {
	p.expirePopupHandoffs()
	p.mu.Lock()
	entry := p.handoffs[handoffID]
	if entry == nil || entry.state != popupStateClaimed || entry.completionToken != completionToken || entry.lastApplied != lastApplied {
		p.mu.Unlock()
		return RouteBootstrap{}, errors.New("popup handoff route claim is invalid")
	}
	entry.state = popupStateReplacing
	replacement, err := p.replaceRouteLocked(entry.sessionID, lastApplied, false)
	if err != nil {
		entry.state = popupStateClaimed
		p.mu.Unlock()
		return RouteBootstrap{}, err
	}
	entry.state = popupStateReplaced
	if session := p.sessions[entry.sessionID]; session != nil && session.route != nil {
		session.route.handoffPending = true
	}
	p.mu.Unlock()
	closeConnections(replacement.dataConn, replacement.urgentConn)
	return replacement.bootstrap, nil
}

func (p *ProbeService) PopupHandoffStatus(handoffID, cancelToken string) (PopupHandoffStatus, error) {
	p.expirePopupHandoffs()
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry := p.handoffs[handoffID]; entry != nil && entry.cancelToken == cancelToken {
		return PopupHandoffStatus{State: string(entry.state)}, nil
	}
	if recovery := p.popupRecoveries[handoffID]; recovery != nil && recovery.cancelToken == cancelToken {
		return PopupHandoffStatus{State: "aborted"}, nil
	}
	return PopupHandoffStatus{}, errors.New("popup handoff status is unavailable")
}

func (p *ProbeService) RejectPopupHandoff(handoffID, completionToken string) error {
	p.mu.Lock()
	entry := p.handoffs[handoffID]
	if entry == nil || (entry.state != popupStateClaimed && entry.state != popupStateReplaced) || entry.completionToken != completionToken {
		p.mu.Unlock()
		return errors.New("popup rejection is invalid")
	}
	routeReplaced := entry.state == popupStateReplaced
	entry.state = popupStateAborting
	delete(p.handoffs, handoffID)
	p.addPopupRecoveryLocked(handoffID, entry, routeReplaced)
	host := p.windowHost
	popupName := entry.popupWindowName
	p.mu.Unlock()
	if host != nil {
		_ = host.CloseWindow(popupName)
	}
	p.wakeHandoffCleanupLoop()
	return nil
}

func (p *ProbeService) AbortPopupHandoff(handoffID, cancelToken string) (PopupAbortResult, error) {
	p.expirePopupHandoffs()
	p.mu.Lock()
	var recovery *popupRecovery
	var popupName string
	var previousState popupHandoffState
	if entry := p.handoffs[handoffID]; entry != nil && entry.cancelToken == cancelToken {
		if entry.state != popupStateCreated && entry.state != popupStateClaimed && entry.state != popupStateReplaced {
			p.mu.Unlock()
			return PopupAbortResult{}, errors.New("popup handoff transition is busy")
		}
		previousState = entry.state
		entry.state = popupStateAborting
		popupName = entry.popupWindowName
		recovery = &popupRecovery{
			cancelToken: cancelToken, sessionID: entry.sessionID, lastApplied: entry.lastApplied,
			routeReplaced: previousState == popupStateReplaced, expiresAt: p.now().Add(p.popupRecoveryTTL),
		}
	} else if stored := p.popupRecoveries[handoffID]; stored != nil && stored.cancelToken == cancelToken {
		recovery = stored
	} else {
		p.mu.Unlock()
		return PopupAbortResult{}, errors.New("popup abort grant is invalid")
	}
	host := p.windowHost
	if recovery.routeReplaced {
		session := p.sessions[recovery.sessionID]
		if session == nil || session.route == nil || !session.route.handoffPending || session.applied != recovery.lastApplied {
			if entry := p.handoffs[handoffID]; entry != nil && entry.state == popupStateAborting {
				entry.state = popupStateReplaced
			}
			p.mu.Unlock()
			return PopupAbortResult{}, errors.New("popup route cannot be safely recovered")
		}
	}
	confirmationToken, err := randomHex(24)
	if err != nil {
		if entry := p.handoffs[handoffID]; entry != nil && entry.state == popupStateAborting {
			entry.state = previousState
		}
		p.mu.Unlock()
		return PopupAbortResult{}, err
	}
	replacement, err := p.replaceRouteLocked(recovery.sessionID, recovery.lastApplied, recovery.routeReplaced)
	if err != nil {
		if entry := p.handoffs[handoffID]; entry != nil && entry.state == popupStateAborting {
			entry.state = previousState
		}
		p.mu.Unlock()
		return PopupAbortResult{}, err
	}
	if session := p.sessions[recovery.sessionID]; session != nil && session.route != nil {
		session.route.handoffPending = true
	}
	recovery.routeReplaced = true
	recovery.issuedGeneration = replacement.bootstrap.Generation
	recovery.confirmationToken = confirmationToken
	recovery.expiresAt = p.now().Add(p.popupRecoveryTTL)
	p.popupRecoveries[handoffID] = recovery
	delete(p.handoffs, handoffID)
	p.mu.Unlock()
	closeConnections(replacement.dataConn, replacement.urgentConn)
	if popupName != "" && host != nil {
		_ = host.CloseWindow(popupName)
	}
	p.wakeHandoffCleanupLoop()
	return PopupAbortResult{Bootstrap: replacement.bootstrap, ConfirmationToken: confirmationToken}, nil
}

func (p *ProbeService) ConfirmPopupRecovery(handoffID, cancelToken, confirmationToken string, generation uint32) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	recovery := p.popupRecoveries[handoffID]
	if recovery == nil || recovery.cancelToken != cancelToken || recovery.confirmationToken != confirmationToken ||
		recovery.issuedGeneration != generation {
		return errors.New("popup recovery confirmation is invalid")
	}
	session := p.sessions[recovery.sessionID]
	if session == nil || session.route == nil || session.generation != generation ||
		session.route.generation != generation || !session.route.handoffPending ||
		session.route.dataConn == nil || session.route.urgentConn == nil || !session.route.creditReady {
		return errors.New("popup recovery route is not fully bound")
	}
	session.route.handoffPending = false
	delete(p.popupRecoveries, handoffID)
	p.wakeLocked(session)
	p.wakeHandoffCleanupLoop()
	return nil
}

func (p *ProbeService) expirePopupHandoffs() {
	type expiredPopup struct {
		host probeWindowHost
		name string
	}
	var expired []expiredPopup
	p.mu.Lock()
	now := p.now()
	for id, entry := range p.handoffs {
		if !now.Before(entry.expiresAt) {
			if entry.state == popupStateReplacing || entry.state == popupStateCompleting || entry.state == popupStateAborting {
				entry.expiresAt = now.Add(time.Second)
				continue
			}
			routeReplaced := entry.state == popupStateReplaced
			entry.state = popupStateExpired
			delete(p.handoffs, id)
			p.addPopupRecoveryLocked(id, entry, routeReplaced)
			expired = append(expired, expiredPopup{host: p.windowHost, name: entry.popupWindowName})
		}
	}
	for id, recovery := range p.popupRecoveries {
		if !now.Before(recovery.expiresAt) {
			delete(p.popupRecoveries, id)
		}
	}
	p.mu.Unlock()
	for _, popup := range expired {
		if popup.host != nil {
			_ = popup.host.CloseWindow(popup.name)
		}
	}
	if len(expired) > 0 {
		p.wakeHandoffCleanupLoop()
	}
}

func (p *ProbeService) handlePopupClosed(handoffID string) {
	p.mu.Lock()
	entry := p.handoffs[handoffID]
	if entry != nil {
		if entry.state == popupStateReplacing || entry.state == popupStateCompleting || entry.state == popupStateAborting {
			entry.abortRequested = true
		} else {
			routeReplaced := entry.state == popupStateReplaced
			entry.state = popupStateExpired
			delete(p.handoffs, handoffID)
			p.addPopupRecoveryLocked(handoffID, entry, routeReplaced)
		}
	}
	p.mu.Unlock()
	if entry != nil {
		p.wakeHandoffCleanupLoop()
	}
}

func (p *ProbeService) addPopupRecoveryLocked(handoffID string, entry *popupHandoff, routeReplaced bool) {
	now := p.now()
	for id, recovery := range p.popupRecoveries {
		if !now.Before(recovery.expiresAt) {
			delete(p.popupRecoveries, id)
		}
	}
	if len(p.popupRecoveries) >= maxPopupHandoffs {
		var oldestID string
		var oldest time.Time
		for id, recovery := range p.popupRecoveries {
			if oldestID == "" || recovery.expiresAt.Before(oldest) {
				oldestID, oldest = id, recovery.expiresAt
			}
		}
		delete(p.popupRecoveries, oldestID)
	}
	p.popupRecoveries[handoffID] = &popupRecovery{
		cancelToken: entry.cancelToken, sessionID: entry.sessionID, lastApplied: entry.lastApplied,
		routeReplaced: routeReplaced, expiresAt: now.Add(p.popupRecoveryTTL),
	}
}

func (p *ProbeService) wakeHandoffCleanupLoop() {
	select {
	case p.handoffWake <- struct{}{}:
	default:
	}
}

func (p *ProbeService) runHandoffCleanupLoop() {
	defer p.wg.Done()
	defer close(p.handoffLoopDone)
	for {
		p.mu.Lock()
		now := p.now()
		var next time.Time
		for _, entry := range p.handoffs {
			if next.IsZero() || entry.expiresAt.Before(next) {
				next = entry.expiresAt
			}
		}
		for _, recovery := range p.popupRecoveries {
			if next.IsZero() || recovery.expiresAt.Before(next) {
				next = recovery.expiresAt
			}
		}
		p.mu.Unlock()

		var timer *time.Timer
		var timerChannel <-chan time.Time
		if !next.IsZero() {
			delay := next.Sub(now)
			if delay < 0 {
				delay = 0
			}
			timer = time.NewTimer(delay)
			timerChannel = timer.C
		}
		select {
		case <-p.ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case <-p.handoffWake:
			if timer != nil {
				timer.Stop()
			}
		case <-timerChannel:
			p.expirePopupHandoffs()
		}
	}
}
