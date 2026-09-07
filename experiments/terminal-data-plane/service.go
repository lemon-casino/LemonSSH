package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"math"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	receiveWindowBytes    = 1024 * 1024
	maxConcurrentSessions = 8
	maxSessionRecords     = 16
	maxPopupHandoffs      = 8
	maxHandoffStateBytes  = 128 * 1024
	dataSubprotocol       = "netcatty-terminal-v1"
	tokenPrefix           = "route."
)

type SessionInfo struct {
	SessionID string `json:"sessionId"`
	Workload  string `json:"workload"`
}

type RouteBootstrap struct {
	SessionID   string `json:"sessionId"`
	Generation  uint32 `json:"generation"`
	DataURL     string `json:"dataUrl"`
	UrgentURL   string `json:"urgentUrl"`
	DataToken   string `json:"dataToken"`
	UrgentToken string `json:"urgentToken"`
	Subprotocol string `json:"subprotocol"`
	WindowBytes uint32 `json:"windowBytes"`
}

type SessionMetrics struct {
	SessionID               string `json:"sessionId"`
	Workload                string `json:"workload"`
	Generation              uint32 `json:"generation"`
	Sequence                uint64 `json:"sequence"`
	AppliedSequence         uint64 `json:"appliedSequence"`
	PayloadBytes            uint64 `json:"payloadBytes"`
	CreditBytes             uint64 `json:"creditBytes"`
	FramesSent              uint64 `json:"framesSent"`
	FramesAcked             uint64 `json:"framesAcked"`
	FramesReplayed          uint64 `json:"framesReplayed"`
	OutstandingBytes        uint64 `json:"outstandingBytes"`
	BackendMaxOutstanding   uint64 `json:"backendMaxOutstanding"`
	PauseCount              uint64 `json:"pauseCount"`
	ResumeCount             uint64 `json:"resumeCount"`
	PauseDurationMillis     int64  `json:"pauseDurationMillis"`
	UrgentCount             uint64 `json:"urgentCount"`
	UrgentServiceMicros     int64  `json:"urgentServiceMicros"`
	UrgentReceiptUnixMillis int64  `json:"urgentReceiptUnixMillis"`
	ExpectedPayloadBytes    uint64 `json:"expectedPayloadBytes"`
	ExpectedCreditBytes     uint64 `json:"expectedCreditBytes"`
	ExpectedFrames          uint64 `json:"expectedFrames"`
	ExpectedPayloadSHA256   string `json:"expectedPayloadSha256"`
	DrainCount              uint64 `json:"drainCount"`
	DrainDurationMillis     int64  `json:"drainDurationMillis"`
	RouteInterruptionCount  uint64 `json:"routeInterruptionCount"`
	FrameWriteAttempts      uint64 `json:"frameWriteAttempts"`
	PreStopOutstandingBytes uint64 `json:"preStopOutstandingBytes"`
	StartedAtUnixMillis     int64  `json:"startedAtUnixMillis"`
	CompletedAtUnixMillis   int64  `json:"completedAtUnixMillis"`
	ProducerSteps           uint64 `json:"producerSteps"`
	Running                 bool   `json:"running"`
	Complete                bool   `json:"complete"`
	Error                   string `json:"error"`
	CloseStatus             string `json:"closeStatus"`
}

type ProbeSnapshot struct {
	ListenerAddress string           `json:"listenerAddress"`
	WindowBytes     uint32           `json:"windowBytes"`
	Memory          MemorySnapshot   `json:"memory"`
	Sessions        []SessionMetrics `json:"sessions"`
}

type routeChannel uint8

const (
	channelData routeChannel = iota + 1
	channelUrgent
)

type routeTicket struct {
	token      string
	sessionID  string
	generation uint32
	channel    routeChannel
}

type outputRecord struct {
	sequence uint64
	cost     uint32
	payload  []byte
	sent     bool
}

type sentCredit struct {
	sequence uint64
	cost     uint32
}

type activeRoute struct {
	generation       uint32
	dataID           string
	urgentID         string
	ctx              context.Context
	cancel           context.CancelFunc
	dataConn         *websocket.Conn
	urgentConn       *websocket.Conn
	creditReady      bool
	available        uint64
	sentThrough      uint64
	inflight         []sentCredit
	completeSent     bool
	draining         bool
	drainMarkerSent  bool
	drainReadySent   bool
	drainReady       bool
	drainCorrelation uint32
	drainTarget      uint64
	drainStartedAt   time.Time
	handoffPending   bool
}

type probeSession struct {
	id          string
	workloadID  string
	workload    workloadIterator
	ctx         context.Context
	cancel      context.CancelFunc
	notify      chan struct{}
	route       *activeRoute
	generation  uint32
	sequence    uint64
	applied     uint64
	records     []*outputRecord
	outstanding uint64
	pending     *generatedChunk
	exhausted   bool
	started     bool
	stopped     bool
	complete    bool
	paused      bool
	pausedAt    time.Time
	metrics     SessionMetrics
	payloadHash hash.Hash
}

type ProbeConfig struct {
	AllowedOrigins []string
	Autorun        AutorunConfig
}

type ProbeService struct {
	mu                                         sync.Mutex
	ctx                                        context.Context
	cancel                                     context.CancelFunc
	fixture                                    SustainedFixture
	sessions                                   map[string]*probeSession
	tickets                                    map[string]routeTicket
	allowedOrigins                             map[string]struct{}
	originPatterns                             []string
	listenerAddr                               string
	httpServer                                 *http.Server
	handoffs                                   map[string]*popupHandoff
	popupRecoveries                            map[string]*popupRecovery
	handoffWake                                chan struct{}
	handoffLoopDone                            chan struct{}
	handoffTTL                                 time.Duration
	popupRecoveryTTL                           time.Duration
	windowHost                                 probeWindowHost
	now                                        func() time.Time
	serviceLifetimeObservedMaxGoHeapAllocBytes uint64
	closed                                     bool
	wg                                         sync.WaitGroup
	autorun                                    *autorunCoordinator
}

func DefaultProbeConfig() ProbeConfig {
	return ProbeConfig{AllowedOrigins: []string{
		"http://wails.localhost",
		"wails://localhost",
		"http://127.0.0.1:9246",
		"http://localhost:9246",
	}, Autorun: defaultAutorunConfig()}
}

func NewProbeService(config ProbeConfig) (*ProbeService, error) {
	fixture, err := loadCanonicalFixture()
	if err != nil {
		return nil, err
	}
	if len(config.AllowedOrigins) == 0 {
		return nil, errors.New("at least one explicit WebSocket origin is required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	service := &ProbeService{
		ctx:              ctx,
		cancel:           cancel,
		fixture:          fixture,
		sessions:         make(map[string]*probeSession),
		tickets:          make(map[string]routeTicket),
		handoffs:         make(map[string]*popupHandoff),
		popupRecoveries:  make(map[string]*popupRecovery),
		handoffWake:      make(chan struct{}, 1),
		handoffLoopDone:  make(chan struct{}),
		handoffTTL:       popupHandoffTTL,
		popupRecoveryTTL: popupRecoveryTTL,
		now:              time.Now,
		allowedOrigins:   make(map[string]struct{}, len(config.AllowedOrigins)),
		originPatterns:   append([]string(nil), config.AllowedOrigins...),
		autorun:          newAutorunCoordinator(config.Autorun, os.Stdout),
	}
	for _, origin := range config.AllowedOrigins {
		if origin == "" || origin == "*" {
			cancel()
			return nil, errors.New("WebSocket origins must be explicit and non-wildcard")
		}
		service.allowedOrigins[origin] = struct{}{}
	}
	if err := service.startLoopbackServer(); err != nil {
		cancel()
		return nil, err
	}
	service.wg.Add(1)
	go service.runHandoffCleanupLoop()
	return service, nil
}

func (p *ProbeService) Workloads() []WorkloadDescriptor {
	return append([]WorkloadDescriptor(nil), workloadDescriptors...)
}

func (p *ProbeService) CreateSession(workloadID string, options WorkloadOptions) (SessionInfo, error) {
	workload, err := newWorkload(workloadID, options, p.fixture)
	if err != nil {
		return SessionInfo{}, err
	}
	sessionID, err := randomHex(16)
	if err != nil {
		return SessionInfo{}, fmt.Errorf("create session identity: %w", err)
	}
	ctx, cancel := context.WithCancel(p.ctx)
	session := &probeSession{
		id:         sessionID,
		workloadID: workloadID,
		workload:   workload,
		ctx:        ctx,
		cancel:     cancel,
		notify:     make(chan struct{}, 1),
		metrics: SessionMetrics{
			SessionID: sessionID,
			Workload:  workloadID,
		},
		payloadHash: sha256.New(),
	}
	expectation := workload.Expectation()
	session.metrics.ExpectedPayloadBytes = expectation.payloadBytes
	session.metrics.ExpectedCreditBytes = expectation.creditBytes
	session.metrics.ExpectedFrames = expectation.frames
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		cancel()
		return SessionInfo{}, errors.New("probe service is closed")
	}
	activeSessions := 0
	for _, existing := range p.sessions {
		if !existing.stopped {
			activeSessions++
		}
	}
	if activeSessions >= maxConcurrentSessions {
		cancel()
		return SessionInfo{}, fmt.Errorf("probe supports at most %d active sessions", maxConcurrentSessions)
	}
	for len(p.sessions) >= maxSessionRecords {
		removed := false
		for id, existing := range p.sessions {
			if existing.stopped {
				delete(p.sessions, id)
				removed = true
				break
			}
		}
		if !removed {
			cancel()
			return SessionInfo{}, errors.New("bounded session history is full")
		}
	}
	p.sessions[sessionID] = session
	return SessionInfo{SessionID: sessionID, Workload: workloadID}, nil
}

func (p *ProbeService) ResumeSession(sessionID string, lastAppliedSequence uint64) (RouteBootstrap, error) {
	p.mu.Lock()
	replacement, err := p.replaceRouteLocked(sessionID, lastAppliedSequence, false)
	p.mu.Unlock()
	closeConnections(replacement.dataConn, replacement.urgentConn)
	return replacement.bootstrap, err
}

type routeReplacement struct {
	bootstrap  RouteBootstrap
	dataConn   *websocket.Conn
	urgentConn *websocket.Conn
}

func (p *ProbeService) replaceRouteLocked(sessionID string, lastAppliedSequence uint64, allowPendingHandoff bool) (routeReplacement, error) {
	if p.closed {
		return routeReplacement{}, errors.New("probe service is closed")
	}
	session := p.sessions[sessionID]
	if session == nil || session.stopped {
		return routeReplacement{}, errors.New("session is not active")
	}
	if lastAppliedSequence < session.applied || lastAppliedSequence > session.sequence {
		return routeReplacement{}, fmt.Errorf("last-applied sequence %d is outside [%d,%d]", lastAppliedSequence, session.applied, session.sequence)
	}
	if session.route != nil && session.route.dataConn != nil {
		drained := session.route.drainReady && session.route.drainTarget == lastAppliedSequence && session.applied == lastAppliedSequence
		pendingHandoff := allowPendingHandoff && session.route.handoffPending && session.applied == lastAppliedSequence
		if !drained && !pendingHandoff {
			return routeReplacement{}, errors.New("live route replacement requires an ordered drain through the acknowledged prefix")
		}
	}
	if session.generation == math.MaxUint32 {
		return routeReplacement{}, errors.New("session generation exhausted")
	}
	dataID, err := randomHex(16)
	urgentID := ""
	if err == nil {
		urgentID, err = randomHex(16)
	}
	dataToken, urgentToken := "", ""
	if err == nil {
		dataToken, err = randomHex(32)
	}
	if err == nil {
		urgentToken, err = randomHex(32)
	}
	if err != nil {
		return routeReplacement{}, fmt.Errorf("create route credentials: %w", err)
	}

	p.applyReportedSequenceLocked(session, lastAppliedSequence)
	oldRoute := session.route
	var oldDataConn, oldUrgentConn *websocket.Conn
	if oldRoute != nil {
		session.metrics.RouteInterruptionCount++
		p.deleteRouteTicketsLocked(oldRoute)
		oldRoute.cancel()
		oldDataConn, oldUrgentConn = detachRouteConnectionsLocked(oldRoute)
	}
	session.generation++
	routeContext, routeCancel := context.WithCancel(session.ctx)
	route := &activeRoute{
		generation: session.generation, dataID: dataID, urgentID: urgentID,
		ctx: routeContext, cancel: routeCancel, sentThrough: lastAppliedSequence,
	}
	session.route = route
	session.metrics.Generation = session.generation
	p.tickets[route.dataID] = routeTicket{token: dataToken, sessionID: sessionID, generation: route.generation, channel: channelData}
	p.tickets[route.urgentID] = routeTicket{token: urgentToken, sessionID: sessionID, generation: route.generation, channel: channelUrgent}
	bootstrap := RouteBootstrap{
		SessionID:   sessionID,
		Generation:  route.generation,
		DataURL:     "ws://" + p.listenerAddr + "/v1/data/" + route.dataID,
		UrgentURL:   "ws://" + p.listenerAddr + "/v1/urgent/" + route.urgentID,
		DataToken:   dataToken,
		UrgentToken: urgentToken,
		Subprotocol: dataSubprotocol,
		WindowBytes: receiveWindowBytes,
	}
	p.wakeLocked(session)
	return routeReplacement{bootstrap: bootstrap, dataConn: oldDataConn, urgentConn: oldUrgentConn}, nil
}

func (p *ProbeService) StartSession(sessionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	session := p.sessions[sessionID]
	if session == nil || session.stopped {
		return errors.New("session is not active")
	}
	if session.route == nil {
		return errors.New("session has no bound route")
	}
	if !session.started {
		session.started = true
		session.metrics.Running = true
		session.metrics.StartedAtUnixMillis = time.Now().UnixMilli()
	}
	p.wakeLocked(session)
	return nil
}

func (p *ProbeService) StopSession(sessionID string) error {
	p.mu.Lock()
	session := p.sessions[sessionID]
	if session == nil {
		p.mu.Unlock()
		return errors.New("session not found")
	}
	if session.stopped {
		p.mu.Unlock()
		return nil
	}
	session.stopped = true
	session.metrics.Running = false
	session.metrics.CloseStatus = "stopped"
	dataConn, urgentConn := p.releaseSessionResourcesLocked(session)
	p.wakeLocked(session)
	p.mu.Unlock()
	closeConnections(dataConn, urgentConn)
	return nil
}

func (p *ProbeService) Snapshot() ProbeSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := ProbeSnapshot{ListenerAddress: p.listenerAddr, WindowBytes: receiveWindowBytes}
	result.Memory = p.memorySnapshotLocked()
	for _, session := range p.sessions {
		metrics := session.metrics
		metrics.Generation = session.generation
		metrics.Sequence = session.sequence
		metrics.AppliedSequence = session.applied
		metrics.OutstandingBytes = session.outstanding
		if session.workload != nil {
			metrics.ProducerSteps = session.workload.Steps()
		}
		if session.paused {
			metrics.PauseDurationMillis += time.Since(session.pausedAt).Milliseconds()
		}
		result.Sessions = append(result.Sessions, metrics)
	}
	slices.SortFunc(result.Sessions, func(a, b SessionMetrics) int {
		if a.SessionID < b.SessionID {
			return -1
		}
		if a.SessionID > b.SessionID {
			return 1
		}
		return 0
	})
	return result
}

func (p *ProbeService) shutdown() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	if p.autorun.config.Enabled && p.autorun.outcomeSnapshot() == autorunOutcomePending {
		_ = p.autorun.completeFailure("shutdown", "incomplete", autorunOutcomeFailure)
	}
	p.cancel()
	server := p.httpServer
	connections := make([]*websocket.Conn, 0, 2*len(p.sessions))
	for _, session := range p.sessions {
		session.stopped = true
		session.metrics.Running = false
		if session.metrics.CloseStatus == "" {
			session.metrics.CloseStatus = "service-closed"
		}
		dataConn, urgentConn := p.releaseSessionResourcesLocked(session)
		if dataConn != nil {
			connections = append(connections, dataConn)
		}
		if urgentConn != nil {
			connections = append(connections, urgentConn)
		}
		p.wakeLocked(session)
	}
	p.tickets = make(map[string]routeTicket)
	p.handoffs = make(map[string]*popupHandoff)
	p.popupRecoveries = make(map[string]*popupRecovery)
	p.mu.Unlock()
	p.autorun.detachReadinessCallbacks()
	for _, connection := range connections {
		_ = connection.CloseNow()
	}
	var shutdownErr error
	if server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil && !errors.Is(err, context.Canceled) {
			shutdownErr = err
		}
	}
	p.wg.Wait()
	return shutdownErr
}

func (p *ProbeService) applyReportedSequenceLocked(session *probeSession, lastApplied uint64) {
	for len(session.records) > 0 && session.records[0].sequence <= lastApplied {
		record := session.records[0]
		session.records = session.records[1:]
		session.outstanding -= uint64(record.cost)
		session.metrics.FramesAcked++
	}
	session.applied = lastApplied
	session.metrics.AppliedSequence = lastApplied
}

func (p *ProbeService) deleteRouteTicketsLocked(route *activeRoute) {
	delete(p.tickets, route.dataID)
	delete(p.tickets, route.urgentID)
}

func (p *ProbeService) wakeLocked(session *probeSession) {
	select {
	case session.notify <- struct{}{}:
	default:
	}
}

func (p *ProbeService) beginPauseLocked(session *probeSession) {
	if !session.paused && session.started && !session.complete && !session.stopped {
		session.paused = true
		session.pausedAt = time.Now()
		session.metrics.PauseCount++
	}
}

func (p *ProbeService) finishPauseLocked(session *probeSession) {
	p.endPauseLocked(session, true)
}

func (p *ProbeService) endPauseLocked(session *probeSession, resumed bool) {
	if session.paused {
		session.metrics.PauseDurationMillis += time.Since(session.pausedAt).Milliseconds()
		if resumed {
			session.metrics.ResumeCount++
		}
		session.paused = false
	}
}

func (p *ProbeService) releaseSessionResourcesLocked(session *probeSession) (*websocket.Conn, *websocket.Conn) {
	p.endPauseLocked(session, false)
	if session.workload != nil {
		session.metrics.ProducerSteps = session.workload.Steps()
	}
	if session.outstanding > session.metrics.PreStopOutstandingBytes {
		session.metrics.PreStopOutstandingBytes = session.outstanding
	}
	for _, record := range session.records {
		record.payload = nil
	}
	session.records = nil
	session.pending = nil
	session.outstanding = 0
	session.workload = nil
	session.payloadHash = nil
	route := session.route
	var dataConn, urgentConn *websocket.Conn
	if route != nil {
		p.deleteRouteTicketsLocked(route)
		route.cancel()
		dataConn, urgentConn = detachRouteConnectionsLocked(route)
		route.inflight = nil
	}
	session.route = nil
	session.cancel()
	return dataConn, urgentConn
}

func randomHex(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func detachRouteConnectionsLocked(route *activeRoute) (*websocket.Conn, *websocket.Conn) {
	if route == nil {
		return nil, nil
	}
	dataConn, urgentConn := route.dataConn, route.urgentConn
	route.dataConn = nil
	route.urgentConn = nil
	return dataConn, urgentConn
}

func closeConnections(dataConn, urgentConn *websocket.Conn) {
	if dataConn != nil {
		_ = dataConn.CloseNow()
	}
	if urgentConn != nil {
		_ = urgentConn.CloseNow()
	}
}
