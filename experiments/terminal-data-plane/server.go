package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

var errRouteTeardown = errors.New("route teardown in progress")

func (p *ProbeService) startLoopbackServer() error {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("bind terminal data-plane loopback listener: %w", err)
	}
	p.listenerAddr = listener.Addr().String()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/data/", func(writer http.ResponseWriter, request *http.Request) {
		p.serveWebSocket(writer, request, channelData)
	})
	mux.HandleFunc("/v1/urgent/", func(writer http.ResponseWriter, request *http.Request) {
		p.serveWebSocket(writer, request, channelUrgent)
	})
	p.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		if serveErr := p.httpServer.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			p.mu.Lock()
			for _, session := range p.sessions {
				if session.metrics.Error == "" {
					session.metrics.Error = "loopback listener stopped"
				}
			}
			p.mu.Unlock()
		}
	}()
	return nil
}

func (p *ProbeService) serveWebSocket(writer http.ResponseWriter, request *http.Request, channel routeChannel) {
	p.wg.Add(1)
	defer p.wg.Done()
	if request.Host != p.listenerAddr {
		http.Error(writer, "invalid Host", http.StatusForbidden)
		return
	}
	origin := request.Header.Get("Origin")
	if _, allowed := p.allowedOrigins[origin]; !allowed {
		http.Error(writer, "invalid Origin", http.StatusForbidden)
		return
	}
	prefix := "/v1/data/"
	if channel == channelUrgent {
		prefix = "/v1/urgent/"
	}
	routeID := strings.TrimPrefix(request.URL.Path, prefix)
	if routeID == "" || strings.Contains(routeID, "/") {
		http.Error(writer, "invalid route", http.StatusNotFound)
		return
	}
	token, protocolsValid := offeredRouteToken(request.Header.Values("Sec-WebSocket-Protocol"))
	if !protocolsValid {
		http.Error(writer, "base and route token subprotocols required", http.StatusUnauthorized)
		return
	}
	p.mu.Lock()
	ticket, exists := p.tickets[routeID]
	if !exists || ticket.channel != channel || ticket.token != token {
		p.mu.Unlock()
		http.Error(writer, "invalid route credentials", http.StatusUnauthorized)
		return
	}
	delete(p.tickets, routeID)
	session := p.sessions[ticket.sessionID]
	valid := session != nil && !session.stopped && session.route != nil &&
		session.route.generation == ticket.generation
	p.mu.Unlock()
	if !valid {
		http.Error(writer, "stale route", http.StatusGone)
		return
	}
	connection, err := websocket.Accept(writer, request, &websocket.AcceptOptions{
		Subprotocols:    []string{dataSubprotocol},
		OriginPatterns:  p.originPatterns,
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return
	}
	if connection.Subprotocol() != dataSubprotocol {
		_ = connection.Close(websocket.StatusPolicyViolation, "subprotocol negotiation failed")
		return
	}
	connection.SetReadLimit(maxFrameBytes)
	if channel == channelData {
		p.handleDataConnection(session, ticket.generation, connection)
	} else {
		p.handleUrgentConnection(session, ticket.generation, connection)
	}
}

func offeredRouteToken(headers []string) (string, bool) {
	var protocols []string
	for _, header := range headers {
		for _, value := range strings.Split(header, ",") {
			value = strings.TrimSpace(value)
			if value != "" {
				protocols = append(protocols, value)
			}
		}
	}
	if len(protocols) != 2 || protocols[0] != dataSubprotocol || !strings.HasPrefix(protocols[1], tokenPrefix) {
		return "", false
	}
	token := strings.TrimPrefix(protocols[1], tokenPrefix)
	decoded, err := hex.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return "", false
	}
	return token, true
}

func (p *ProbeService) handleDataConnection(session *probeSession, generation uint32, connection *websocket.Conn) {
	p.mu.Lock()
	if session.route == nil || session.route.generation != generation || session.stopped {
		p.mu.Unlock()
		_ = connection.Close(websocket.StatusPolicyViolation, "stale generation")
		return
	}
	route := session.route
	route.dataConn = connection
	p.wakeLocked(session)
	p.mu.Unlock()

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.pumpOutput(session, route, connection)
	}()

	defer func() {
		p.mu.Lock()
		if session.route == route && route.dataConn == connection {
			route.dataConn = nil
			if !session.stopped && !session.complete && route.ctx.Err() == nil {
				session.metrics.CloseStatus = "data-disconnected"
			}
			p.wakeLocked(session)
		}
		p.mu.Unlock()
		_ = connection.CloseNow()
	}()
	for {
		messageType, data, err := connection.Read(route.ctx)
		if err != nil {
			return
		}
		if messageType != websocket.MessageBinary {
			if p.routeTearingDown(session, route) {
				return
			}
			p.rejectConnection(session, connection, "text frames are not allowed")
			return
		}
		frame, err := UnmarshalFrame(data)
		if err != nil {
			if p.routeTearingDown(session, route) {
				return
			}
			p.rejectConnection(session, connection, err.Error())
			return
		}
		var frameErr error
		switch frame.Kind {
		case FrameCredit:
			if len(frame.Payload) != 0 || frame.Correlation != 0 || frame.TimestampMicros != 0 {
				frameErr = errors.New("invalid credit frame fields")
			} else {
				frameErr = p.applyCredit(session, route, frame)
			}
		case FrameDrainRequest:
			if len(frame.Payload) != 0 || frame.CreditCost != 0 || frame.Correlation == 0 || frame.TimestampMicros != 0 {
				frameErr = errors.New("invalid drain request fields")
			} else {
				frameErr = p.applyDrainRequest(session, route, frame)
			}
		default:
			frameErr = errors.New("data channel accepts credit and drain request frames only")
		}
		if frameErr != nil {
			if errors.Is(frameErr, errRouteTeardown) {
				return
			}
			p.rejectConnection(session, connection, frameErr.Error())
			return
		}
	}
}

func (p *ProbeService) applyDrainRequest(session *probeSession, route *activeRoute, frame Frame) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if session.stopped || route.ctx.Err() != nil || session.route != route {
		return errRouteTeardown
	}
	if route.generation != frame.Generation {
		return errors.New("stale generation drain rejected")
	}
	if !route.creditReady {
		return errors.New("route must have credit before draining")
	}
	if frame.Sequence < session.applied || frame.Sequence > route.sentThrough {
		return errors.New("drain request sequence is outside the sent prefix")
	}
	if route.draining {
		return errors.New("route drain is already active")
	}
	route.draining = true
	route.drainCorrelation = frame.Correlation
	route.drainStartedAt = time.Now()
	session.metrics.DrainCount++
	p.wakeLocked(session)
	return nil
}

func (p *ProbeService) applyCredit(session *probeSession, route *activeRoute, frame Frame) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if session.stopped || route.ctx.Err() != nil || session.route != route {
		return errRouteTeardown
	}
	if route.generation != frame.Generation {
		return errors.New("stale generation credit rejected")
	}
	if !route.creditReady {
		if frame.Sequence != session.applied || frame.CreditCost != receiveWindowBytes {
			return fmt.Errorf("initial credit must grant exactly %d bytes at applied sequence %d", receiveWindowBytes, session.applied)
		}
		route.creditReady = true
		route.available = receiveWindowBytes
		p.wakeLocked(session)
		return nil
	}
	if len(route.inflight) == 0 {
		return errors.New("credit ACK has no matching output frame")
	}
	expected := route.inflight[0]
	if frame.Sequence != expected.sequence || frame.CreditCost != expected.cost {
		return fmt.Errorf("credit ACK must match sequence %d and cost %d", expected.sequence, expected.cost)
	}
	if len(session.records) == 0 || session.records[0].sequence != expected.sequence {
		return errors.New("credit ACK is outside retained replay order")
	}
	route.inflight = route.inflight[1:]
	session.records = session.records[1:]
	session.outstanding -= uint64(expected.cost)
	session.applied = expected.sequence
	route.available += uint64(expected.cost)
	if route.available > receiveWindowBytes {
		return errors.New("returned credit exceeds receive window")
	}
	session.metrics.FramesAcked++
	session.metrics.AppliedSequence = session.applied
	p.wakeLocked(session)
	return nil
}

type outboundFrame struct {
	frame    Frame
	replayed bool
	complete bool
	record   *outputRecord
}

func (p *ProbeService) pumpOutput(session *probeSession, route *activeRoute, connection *websocket.Conn) {
	for {
		outbound, ready := p.reserveOutput(session, route)
		if !ready {
			select {
			case <-route.ctx.Done():
				return
			case <-session.notify:
				continue
			}
		}
		outbound.frame.TimestampMicros = uint64(time.Now().UnixMicro())
		data, err := outbound.frame.MarshalBinary()
		if err != nil {
			p.setSessionError(session, err)
			return
		}
		if err := connection.Write(route.ctx, websocket.MessageBinary, data); err != nil {
			return
		}
		p.commitOutboundWrite(session, outbound)
	}
}

func (p *ProbeService) reserveOutput(session *probeSession, route *activeRoute) (outboundFrame, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if session.route != route || route.dataConn == nil || session.stopped || route.ctx.Err() != nil {
		return outboundFrame{}, false
	}
	if !session.started || !route.creditReady {
		p.beginPauseLocked(session)
		return outboundFrame{}, false
	}
	if route.handoffPending {
		p.beginPauseLocked(session)
		return outboundFrame{}, false
	}
	if route.draining {
		if !route.drainMarkerSent {
			route.drainMarkerSent = true
			route.drainTarget = route.sentThrough
			return outboundFrame{frame: Frame{
				Kind: FrameDrainMarker, Generation: route.generation,
				Sequence: route.drainTarget, Correlation: route.drainCorrelation,
			}}, true
		}
		if !route.drainReadySent && len(route.inflight) == 0 && session.applied == route.drainTarget {
			route.drainReadySent = true
			route.drainReady = true
			session.metrics.DrainDurationMillis += time.Since(route.drainStartedAt).Milliseconds()
			return outboundFrame{frame: Frame{
				Kind: FrameDrainReady, Generation: route.generation,
				Sequence: route.drainTarget, Correlation: route.drainCorrelation,
			}}, true
		}
		p.beginPauseLocked(session)
		return outboundFrame{}, false
	}

	for _, record := range session.records {
		if record.sequence <= route.sentThrough {
			continue
		}
		if uint64(record.cost) > route.available {
			p.beginPauseLocked(session)
			return outboundFrame{}, false
		}
		p.finishPauseLocked(session)
		route.available -= uint64(record.cost)
		route.sentThrough = record.sequence
		route.inflight = append(route.inflight, sentCredit{sequence: record.sequence, cost: record.cost})
		replayed := record.sent
		session.metrics.FrameWriteAttempts++
		return outboundFrame{frame: Frame{Kind: FrameOutput, Generation: route.generation, Sequence: record.sequence, CreditCost: record.cost, Payload: record.payload}, replayed: replayed, record: record}, true
	}

	if session.exhausted {
		if len(session.records) == 0 && !route.completeSent {
			route.completeSent = true
			session.complete = true
			session.metrics.Complete = true
			session.metrics.Running = false
			session.metrics.CompletedAtUnixMillis = time.Now().UnixMilli()
			p.finishPauseLocked(session)
			return outboundFrame{frame: Frame{Kind: FrameComplete, Generation: route.generation, Sequence: session.applied}, complete: true}, true
		}
		p.beginPauseLocked(session)
		return outboundFrame{}, false
	}

	if session.pending == nil && route.available >= maxPayloadBytes &&
		session.outstanding <= receiveWindowBytes-maxPayloadBytes {
		chunk, ok, err := session.workload.Next()
		if err != nil {
			session.metrics.Error = err.Error()
			session.stopped = true
			return outboundFrame{}, false
		}
		if !ok {
			session.exhausted = true
			session.metrics.ExpectedPayloadBytes = session.metrics.PayloadBytes
			session.metrics.ExpectedCreditBytes = session.metrics.CreditBytes
			session.metrics.ExpectedFrames = session.sequence
			session.metrics.ExpectedPayloadSHA256 = fmt.Sprintf("%x", session.payloadHash.Sum(nil))
			p.wakeLocked(session)
			return outboundFrame{}, false
		}
		session.pending = &chunk
	}
	if session.pending == nil || uint64(session.pending.cost) > route.available ||
		session.outstanding+uint64(session.pending.cost) > receiveWindowBytes {
		p.beginPauseLocked(session)
		return outboundFrame{}, false
	}
	chunk := *session.pending
	session.pending = nil
	session.sequence++
	record := &outputRecord{sequence: session.sequence, cost: chunk.cost, payload: chunk.payload}
	session.records = append(session.records, record)
	session.outstanding += uint64(chunk.cost)
	if session.outstanding > session.metrics.BackendMaxOutstanding {
		session.metrics.BackendMaxOutstanding = session.outstanding
	}
	route.available -= uint64(chunk.cost)
	route.sentThrough = record.sequence
	route.inflight = append(route.inflight, sentCredit{sequence: record.sequence, cost: record.cost})
	session.metrics.Sequence = session.sequence
	session.metrics.PayloadBytes += uint64(len(chunk.payload))
	session.metrics.CreditBytes += uint64(chunk.cost)
	session.metrics.FrameWriteAttempts++
	_, _ = session.payloadHash.Write(chunk.payload)
	p.finishPauseLocked(session)
	return outboundFrame{frame: Frame{Kind: FrameOutput, Generation: route.generation, Sequence: record.sequence, CreditCost: record.cost, Payload: record.payload}, record: record}, true
}

func (p *ProbeService) commitOutboundWrite(session *probeSession, outbound outboundFrame) {
	if outbound.record == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	outbound.record.sent = true
	session.metrics.FramesSent++
	if outbound.replayed {
		session.metrics.FramesReplayed++
	}
}

func (p *ProbeService) handleUrgentConnection(session *probeSession, generation uint32, connection *websocket.Conn) {
	p.mu.Lock()
	if session.route == nil || session.route.generation != generation || session.stopped {
		p.mu.Unlock()
		_ = connection.Close(websocket.StatusPolicyViolation, "stale generation")
		return
	}
	route := session.route
	route.urgentConn = connection
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		if session.route == route && route.urgentConn == connection {
			route.urgentConn = nil
			if !session.stopped && !session.complete && route.ctx.Err() == nil {
				session.metrics.CloseStatus = "urgent-disconnected"
			}
		}
		p.mu.Unlock()
		_ = connection.CloseNow()
	}()
	for {
		messageType, data, err := connection.Read(route.ctx)
		if err != nil {
			return
		}
		start := time.Now()
		if messageType != websocket.MessageBinary {
			if p.routeTearingDown(session, route) {
				return
			}
			p.rejectConnection(session, connection, "text frames are not allowed")
			return
		}
		frame, err := UnmarshalFrame(data)
		if err != nil || frame.Kind != FrameUrgent || frame.Generation != generation ||
			frame.Correlation == 0 || frame.CreditCost != 0 || frame.TimestampMicros != 0 ||
			len(frame.Payload) != 1 || frame.Payload[0] != 3 {
			if p.routeTearingDown(session, route) {
				return
			}
			p.rejectConnection(session, connection, "invalid urgent ETX frame")
			return
		}
		ack := Frame{
			Kind: FrameUrgentACK, Generation: generation, Sequence: frame.Sequence,
			Correlation: frame.Correlation, TimestampMicros: uint64(time.Now().UnixMicro()),
		}
		encoded, _ := ack.MarshalBinary()
		if err := connection.Write(route.ctx, websocket.MessageBinary, encoded); err != nil {
			return
		}
		p.mu.Lock()
		if session.route == route {
			session.metrics.UrgentCount++
			session.metrics.UrgentReceiptUnixMillis = start.UnixMilli()
			session.metrics.UrgentServiceMicros = time.Since(start).Microseconds()
		}
		p.mu.Unlock()
	}
}

func (p *ProbeService) routeTearingDown(session *probeSession, route *activeRoute) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return session.stopped || route.ctx.Err() != nil || session.route != route
}

func (p *ProbeService) rejectConnection(session *probeSession, connection *websocket.Conn, reason string) {
	p.setSessionError(session, errors.New(reason))
	p.mu.Lock()
	session.metrics.CloseStatus = "protocol-violation"
	p.mu.Unlock()
	_ = connection.Close(websocket.StatusPolicyViolation, "protocol violation")
}

func (p *ProbeService) setSessionError(session *probeSession, err error) {
	p.mu.Lock()
	if session.metrics.Error == "" {
		session.metrics.Error = err.Error()
	}
	p.mu.Unlock()
}

func dialOptions(origin, host, token string) *websocket.DialOptions {
	return &websocket.DialOptions{
		HTTPHeader:   http.Header{"Origin": []string{origin}},
		Host:         host,
		Subprotocols: []string{dataSubprotocol, tokenPrefix + token},
	}
}

func writeFrame(ctx context.Context, connection *websocket.Conn, frame Frame) error {
	data, err := frame.MarshalBinary()
	if err != nil {
		return err
	}
	return connection.Write(ctx, websocket.MessageBinary, data)
}
