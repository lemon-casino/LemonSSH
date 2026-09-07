package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const testOrigin = "http://probe.test"

func newTestService(t *testing.T) *ProbeService {
	t.Helper()
	service, err := NewProbeService(ProbeConfig{AllowedOrigins: []string{testOrigin}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.shutdown(); err != nil {
			t.Errorf("close service: %v", err)
		}
	})
	return service
}

func createRoute(t *testing.T, service *ProbeService, workload string, options WorkloadOptions) (SessionInfo, RouteBootstrap) {
	t.Helper()
	session, err := service.CreateSession(workload, options)
	if err != nil {
		t.Fatal(err)
	}
	route, err := service.ResumeSession(session.SessionID, 0)
	if err != nil {
		t.Fatal(err)
	}
	return session, route
}

func dialRoute(ctx context.Context, service *ProbeService, url, token string) (*websocket.Conn, *http.Response, error) {
	return websocket.Dial(ctx, url, dialOptions(testOrigin, service.listenerAddr, token))
}

func mustDialRoute(t *testing.T, service *ProbeService, url, token string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	connection, response, err := dialRoute(ctx, service, url, token)
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("dial route (status %d): %v", status, err)
	}
	connection.SetReadLimit(maxFrameBytes)
	t.Cleanup(func() { _ = connection.CloseNow() })
	return connection
}

func sendInitialCredit(t *testing.T, connection *websocket.Conn, route RouteBootstrap, sequence uint64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := writeFrame(ctx, connection, Frame{
		Kind:       FrameCredit,
		Generation: route.Generation,
		Sequence:   sequence,
		CreditCost: route.WindowBytes,
	}); err != nil {
		t.Fatal(err)
	}
}

func readBinaryFrame(t *testing.T, connection *websocket.Conn) Frame {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	messageType, data, err := connection.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if messageType != websocket.MessageBinary {
		t.Fatalf("received non-binary message type %v", messageType)
	}
	frame, err := UnmarshalFrame(data)
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

func ackFrame(t *testing.T, connection *websocket.Conn, frame Frame) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := writeFrame(ctx, connection, Frame{
		Kind:       FrameCredit,
		Generation: frame.Generation,
		Sequence:   frame.Sequence,
		CreditCost: frame.CreditCost,
	}); err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, message string, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !predicate() {
		if time.Now().After(deadline) {
			t.Fatal(message)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func metricsFor(t *testing.T, service *ProbeService, sessionID string) SessionMetrics {
	t.Helper()
	for _, metrics := range service.Snapshot().Sessions {
		if metrics.SessionID == sessionID {
			return metrics
		}
	}
	t.Fatalf("metrics for session %s not found", sessionID)
	return SessionMetrics{}
}

func waitForDataDisconnect(t *testing.T, service *ProbeService, sessionID string) {
	t.Helper()
	waitFor(t, "data route did not disconnect", func() bool {
		service.mu.Lock()
		defer service.mu.Unlock()
		session := service.sessions[sessionID]
		return session != nil && session.route != nil && session.route.dataConn == nil
	})
}

func TestRouteAuthenticationRejectsHostOriginTokenAndReplay(t *testing.T) {
	service := newTestService(t)
	_, route := createRoute(t, service, WorkloadMetadataOnly, WorkloadOptions{MetadataFrames: 1})

	tests := []struct {
		name    string
		url     string
		options *websocket.DialOptions
	}{
		{
			name: "missing origin",
			url:  route.DataURL,
			options: &websocket.DialOptions{
				Host: service.listenerAddr, Subprotocols: []string{dataSubprotocol, tokenPrefix + route.DataToken},
			},
		},
		{
			name: "wrong origin",
			url:  route.DataURL,
			options: &websocket.DialOptions{
				HTTPHeader: http.Header{"Origin": []string{"http://unapproved.test"}},
				Host:       service.listenerAddr, Subprotocols: []string{dataSubprotocol, tokenPrefix + route.DataToken},
			},
		},
		{
			name:    "wrong host",
			url:     route.DataURL,
			options: dialOptions(testOrigin, "127.0.0.1:9", route.DataToken),
		},
		{
			name:    "wrong token",
			url:     route.DataURL,
			options: dialOptions(testOrigin, service.listenerAddr, strings.Repeat("0", len(route.DataToken))),
		},
		{
			name:    "wrong route",
			url:     route.UrgentURL,
			options: dialOptions(testOrigin, service.listenerAddr, route.DataToken),
		},
		{
			name: "base protocol only",
			url:  route.DataURL,
			options: &websocket.DialOptions{
				HTTPHeader: http.Header{"Origin": []string{testOrigin}}, Host: service.listenerAddr,
				Subprotocols: []string{dataSubprotocol},
			},
		},
		{
			name: "token protocol only",
			url:  route.DataURL,
			options: &websocket.DialOptions{
				HTTPHeader: http.Header{"Origin": []string{testOrigin}}, Host: service.listenerAddr,
				Subprotocols: []string{tokenPrefix + route.DataToken},
			},
		},
		{
			name: "wrong protocol ordering",
			url:  route.DataURL,
			options: &websocket.DialOptions{
				HTTPHeader: http.Header{"Origin": []string{testOrigin}}, Host: service.listenerAddr,
				Subprotocols: []string{tokenPrefix + route.DataToken, dataSubprotocol},
			},
		},
		{
			name: "malformed token protocol",
			url:  route.DataURL,
			options: &websocket.DialOptions{
				HTTPHeader: http.Header{"Origin": []string{testOrigin}}, Host: service.listenerAddr,
				Subprotocols: []string{dataSubprotocol, tokenPrefix + "not-hex"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			connection, _, err := websocket.Dial(ctx, test.url, test.options)
			if connection != nil {
				_ = connection.CloseNow()
			}
			if err == nil {
				t.Fatal("unauthorized route connected")
			}
		})
	}

	connection := mustDialRoute(t, service, route.DataURL, route.DataToken)
	if connection.Subprotocol() != dataSubprotocol {
		t.Fatalf("negotiated subprotocol %q", connection.Subprotocol())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	replay, _, err := dialRoute(ctx, service, route.DataURL, route.DataToken)
	if replay != nil {
		_ = replay.CloseNow()
	}
	if err == nil {
		t.Fatal("consumed token was replayed")
	}
}

func TestZeroCreditStartupPauseResumeAndBoundedWindow(t *testing.T) {
	service := newTestService(t)
	session, route := createRoute(t, service, WorkloadLongLine, WorkloadOptions{TotalBytes: receiveWindowBytes + 1})
	data := mustDialRoute(t, service, route.DataURL, route.DataToken)
	beforeStart := metricsFor(t, service, session.SessionID)
	if beforeStart.ExpectedCreditBytes != receiveWindowBytes+1 || beforeStart.ExpectedPayloadBytes != receiveWindowBytes+1 {
		t.Fatalf("deterministic expectation unavailable before start: %#v", beforeStart)
	}
	if err := service.StartSession(session.SessionID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	metrics := metricsFor(t, service, session.SessionID)
	if metrics.ProducerSteps != 0 || metrics.FramesSent != 0 || metrics.PauseCount == 0 {
		t.Fatalf("producer advanced without credit: %#v", metrics)
	}

	sendInitialCredit(t, data, route, 0)
	frames := make([]Frame, 0, receiveWindowBytes/maxPayloadBytes)
	for total := uint32(0); total < route.WindowBytes; {
		frame := readBinaryFrame(t, data)
		if frame.Kind != FrameOutput || frame.Sequence != uint64(len(frames)+1) {
			t.Fatalf("unexpected output frame %#v", frame)
		}
		frames = append(frames, frame)
		total += frame.CreditCost
	}
	waitFor(t, "backend did not reach bounded credit pause", func() bool {
		current := metricsFor(t, service, session.SessionID)
		return current.OutstandingBytes == receiveWindowBytes && current.PauseCount >= 2
	})
	metrics = metricsFor(t, service, session.SessionID)
	if metrics.BackendMaxOutstanding > receiveWindowBytes {
		t.Fatalf("outstanding exceeded window: %d", metrics.BackendMaxOutstanding)
	}

	ackFrame(t, data, frames[0])
	finalOutput := readBinaryFrame(t, data)
	if finalOutput.Sequence != uint64(len(frames)+1) || len(finalOutput.Payload) != 1 {
		t.Fatalf("producer did not resume at exact next byte: %#v", finalOutput)
	}
	for _, frame := range frames[1:] {
		ackFrame(t, data, frame)
	}
	ackFrame(t, data, finalOutput)
	complete := readBinaryFrame(t, data)
	if complete.Kind != FrameComplete || complete.Sequence != finalOutput.Sequence {
		t.Fatalf("invalid completion frame: %#v", complete)
	}
	waitFor(t, "resume was not observed", func() bool {
		return metricsFor(t, service, session.SessionID).ResumeCount > 0
	})
}

func TestMetadataOnlyIngressConsumesAndReturnsCredit(t *testing.T) {
	service := newTestService(t)
	session, route := createRoute(t, service, WorkloadMetadataOnly, WorkloadOptions{MetadataFrames: 3})
	data := mustDialRoute(t, service, route.DataURL, route.DataToken)
	sendInitialCredit(t, data, route, 0)
	if err := service.StartSession(session.SessionID); err != nil {
		t.Fatal(err)
	}
	for sequence := uint64(1); sequence <= 3; sequence++ {
		frame := readBinaryFrame(t, data)
		if frame.Kind != FrameOutput || frame.Sequence != sequence || len(frame.Payload) != 0 || frame.CreditCost != 4096 {
			t.Fatalf("invalid metadata frame: %#v", frame)
		}
		ackFrame(t, data, frame)
	}
	if complete := readBinaryFrame(t, data); complete.Kind != FrameComplete {
		t.Fatalf("missing completion: %#v", complete)
	}
	waitFor(t, "metadata metrics not finalized", func() bool {
		return metricsFor(t, service, session.SessionID).Complete
	})
	metrics := metricsFor(t, service, session.SessionID)
	if metrics.PayloadBytes != 0 || metrics.CreditBytes != 3*4096 || metrics.FramesAcked != 3 || metrics.OutstandingBytes != 0 ||
		metrics.ExpectedPayloadSHA256 != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" ||
		metrics.ExpectedCreditBytes != 3*4096 || metrics.ExpectedFrames != 3 {
		t.Fatalf("metadata credit mismatch: %#v", metrics)
	}
}

func TestUrgentACKIsIndependentOfStalledOutput(t *testing.T) {
	service := newTestService(t)
	session, route := createRoute(t, service, WorkloadLongLine, WorkloadOptions{TotalBytes: 2 * receiveWindowBytes})
	_ = mustDialRoute(t, service, route.DataURL, route.DataToken)
	urgent := mustDialRoute(t, service, route.UrgentURL, route.UrgentToken)
	if err := service.StartSession(session.SessionID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "zero-credit producer did not stall", func() bool {
		return metricsFor(t, service, session.SessionID).PauseCount > 0
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := time.Now()
	if err := writeFrame(ctx, urgent, Frame{
		Kind: FrameUrgent, Generation: route.Generation, Sequence: 0, Correlation: 77, Payload: []byte{3},
	}); err != nil {
		t.Fatal(err)
	}
	messageType, data, err := urgent.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ack, err := UnmarshalFrame(data)
	if err != nil {
		t.Fatal(err)
	}
	if messageType != websocket.MessageBinary || ack.Kind != FrameUrgentACK || ack.Correlation != 77 ||
		ack.Generation != route.Generation || ack.TimestampMicros == 0 {
		t.Fatalf("invalid urgent ACK: %#v", ack)
	}
	t.Logf("urgent ACK observed in %s while output credit was zero", time.Since(started))
	waitFor(t, "urgent metrics were not recorded", func() bool {
		metrics := metricsFor(t, service, session.SessionID)
		return metrics.UrgentCount == 1 && metrics.UrgentReceiptUnixMillis > 0
	})
	metrics := metricsFor(t, service, session.SessionID)
	if metrics.FramesSent != 0 || metrics.ProducerSteps != 0 {
		t.Fatalf("urgent traffic released stalled producer: %#v", metrics)
	}
}

func TestStaleGenerationTrafficIsRejected(t *testing.T) {
	service := newTestService(t)
	session, firstRoute := createRoute(t, service, WorkloadMetadataOnly, WorkloadOptions{MetadataFrames: 1})
	firstData := mustDialRoute(t, service, firstRoute.DataURL, firstRoute.DataToken)
	_ = firstData.CloseNow()
	waitForDataDisconnect(t, service, session.SessionID)
	secondRoute, err := service.ResumeSession(session.SessionID, 0)
	if err != nil {
		t.Fatal(err)
	}
	secondData := mustDialRoute(t, service, secondRoute.DataURL, secondRoute.DataToken)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := writeFrame(ctx, secondData, Frame{
		Kind: FrameCredit, Generation: firstRoute.Generation, Sequence: 0, CreditCost: receiveWindowBytes,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := secondData.Read(ctx); err == nil {
		t.Fatal("stale generation credit was accepted")
	}
	waitFor(t, "stale generation rejection was not observable", func() bool {
		return strings.Contains(metricsFor(t, service, session.SessionID).Error, "stale generation")
	})
}

func TestReconnectRebindPreservesExactAcceptedStream(t *testing.T) {
	service := newTestService(t)
	session, firstRoute := createRoute(t, service, WorkloadSustained, WorkloadOptions{ChunkCount: 40})
	firstData := mustDialRoute(t, service, firstRoute.DataURL, firstRoute.DataToken)
	sendInitialCredit(t, firstData, firstRoute, 0)
	if err := service.StartSession(session.SessionID); err != nil {
		t.Fatal(err)
	}
	var accepted bytes.Buffer
	for sequence := uint64(1); sequence <= 5; sequence++ {
		frame := readBinaryFrame(t, firstData)
		if frame.Sequence != sequence {
			t.Fatalf("first route sequence %d, want %d", frame.Sequence, sequence)
		}
		accepted.Write(frame.Payload)
		ackFrame(t, firstData, frame)
	}
	// Read but deliberately do not apply three frames. The replacement must replay them.
	for sequence := uint64(6); sequence <= 8; sequence++ {
		frame := readBinaryFrame(t, firstData)
		if frame.Sequence != sequence {
			t.Fatalf("unapplied sequence %d, want %d", frame.Sequence, sequence)
		}
	}
	waitFor(t, "backend did not observe applied prefix", func() bool {
		return metricsFor(t, service, session.SessionID).AppliedSequence == 5
	})
	_ = firstData.CloseNow()
	waitForDataDisconnect(t, service, session.SessionID)

	secondRoute, err := service.ResumeSession(session.SessionID, 5)
	if err != nil {
		t.Fatal(err)
	}
	secondData := mustDialRoute(t, service, secondRoute.DataURL, secondRoute.DataToken)
	sendInitialCredit(t, secondData, secondRoute, 5)
	for sequence := uint64(6); sequence <= 40; sequence++ {
		frame := readBinaryFrame(t, secondData)
		if frame.Kind != FrameOutput || frame.Generation != secondRoute.Generation || frame.Sequence != sequence {
			t.Fatalf("rebound stream mismatch at %d: %#v", sequence, frame)
		}
		accepted.Write(frame.Payload)
		ackFrame(t, secondData, frame)
	}
	if complete := readBinaryFrame(t, secondData); complete.Kind != FrameComplete || complete.Sequence != 40 {
		t.Fatalf("invalid rebound completion: %#v", complete)
	}

	fixture, err := loadCanonicalFixture()
	if err != nil {
		t.Fatal(err)
	}
	expectedWorkload, err := newWorkload(WorkloadSustained, WorkloadOptions{ChunkCount: 40}, fixture)
	if err != nil {
		t.Fatal(err)
	}
	expectedHash := sha256.New()
	for {
		chunk, ok, err := expectedWorkload.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		_, _ = expectedHash.Write(chunk.payload)
	}
	actualDigest := sha256.Sum256(accepted.Bytes())
	if hex.EncodeToString(actualDigest[:]) != hex.EncodeToString(expectedHash.Sum(nil)) {
		t.Fatalf("accepted stream digest mismatch: %x", actualDigest)
	}
	waitFor(t, "rebind metrics not finalized", func() bool {
		return metricsFor(t, service, session.SessionID).Complete
	})
	metrics := metricsFor(t, service, session.SessionID)
	if metrics.AppliedSequence != 40 || metrics.FramesReplayed < 3 || metrics.OutstandingBytes != 0 ||
		metrics.ExpectedPayloadSHA256 == "" || metrics.ExpectedPayloadBytes == 0 {
		t.Fatalf("rebind metrics mismatch: %#v", metrics)
	}
}

func TestOrderedDrainFlushesDelayedACKBeforeLiveRebind(t *testing.T) {
	service := newTestService(t)
	session, firstRoute := createRoute(t, service, WorkloadLongLine, WorkloadOptions{TotalBytes: receiveWindowBytes + maxPayloadBytes})
	firstData := mustDialRoute(t, service, firstRoute.DataURL, firstRoute.DataToken)
	sendInitialCredit(t, firstData, firstRoute, 0)
	if err := service.StartSession(session.SessionID); err != nil {
		t.Fatal(err)
	}

	first := readBinaryFrame(t, firstData)
	if first.Kind != FrameOutput || first.Sequence != 1 {
		t.Fatalf("unexpected first output: %#v", first)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := writeFrame(ctx, firstData, Frame{
		Kind: FrameDrainRequest, Generation: firstRoute.Generation,
		Sequence: first.Sequence, Correlation: 91,
	}); err != nil {
		t.Fatal(err)
	}

	frames := []Frame{first}
	var marker Frame
	for {
		frame := readBinaryFrame(t, firstData)
		if frame.Kind == FrameDrainMarker {
			marker = frame
			break
		}
		if frame.Kind != FrameOutput || frame.Sequence != uint64(len(frames)+1) {
			t.Fatalf("output arriving during drain was reordered: %#v", frame)
		}
		frames = append(frames, frame)
	}
	if marker.Correlation != 91 || marker.Sequence != frames[len(frames)-1].Sequence {
		t.Fatalf("drain marker does not follow reserved output: %#v", marker)
	}
	service.mu.Lock()
	drainReadyBeforeACK := service.sessions[session.SessionID].route.drainReady
	service.mu.Unlock()
	if drainReadyBeforeACK {
		t.Fatal("route became drain-ready before xterm-equivalent ACKs")
	}
	if _, err := service.ResumeSession(session.SessionID, marker.Sequence); err == nil {
		t.Fatal("live route replacement succeeded before delayed ACK flush")
	}
	for _, frame := range frames {
		ackFrame(t, firstData, frame)
	}
	ready := readBinaryFrame(t, firstData)
	if ready.Kind != FrameDrainReady || ready.Sequence != marker.Sequence || ready.Correlation != marker.Correlation {
		t.Fatalf("invalid drain-ready frame: %#v", ready)
	}

	secondRoute, err := service.ResumeSession(session.SessionID, ready.Sequence)
	if err != nil {
		t.Fatal(err)
	}
	secondData := mustDialRoute(t, service, secondRoute.DataURL, secondRoute.DataToken)
	sendInitialCredit(t, secondData, secondRoute, ready.Sequence)
	nextSequence := ready.Sequence + 1
	for {
		frame := readBinaryFrame(t, secondData)
		if frame.Kind == FrameComplete {
			if frame.Sequence != nextSequence-1 {
				t.Fatalf("completion sequence %d, want %d", frame.Sequence, nextSequence-1)
			}
			break
		}
		if frame.Kind != FrameOutput || frame.Sequence != nextSequence {
			t.Fatalf("replacement stream duplicated or rejected prefix: %#v", frame)
		}
		ackFrame(t, secondData, frame)
		nextSequence++
	}
	metrics := metricsFor(t, service, session.SessionID)
	if metrics.DrainCount != 1 || metrics.RouteInterruptionCount == 0 || metrics.OutstandingBytes != 0 {
		t.Fatalf("drain metrics mismatch: %#v", metrics)
	}
}

func TestCancellationUnblocksCreditWaitAndCleansUp(t *testing.T) {
	service := newTestService(t)
	session, route := createRoute(t, service, WorkloadLongLine, WorkloadOptions{TotalBytes: 4 * receiveWindowBytes})
	data := mustDialRoute(t, service, route.DataURL, route.DataToken)
	if err := service.StartSession(session.SessionID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "session did not enter credit wait", func() bool {
		return metricsFor(t, service, session.SessionID).PauseCount > 0
	})
	if err := service.StopSession(session.SessionID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, _, err := data.Read(ctx); err == nil {
		t.Fatal("stopped session left data connection open")
	}
	metrics := metricsFor(t, service, session.SessionID)
	if metrics.Running || metrics.CloseStatus != "stopped" || metrics.ProducerSteps != 0 {
		t.Fatalf("session cleanup mismatch: %#v", metrics)
	}
	if err := service.shutdown(); err != nil {
		t.Fatal(err)
	}
}

func TestBoundedConcurrentSessionsAtHarnessCounts(t *testing.T) {
	for _, count := range []int{1, 4, 8} {
		t.Run(fmt.Sprintf("sessions-%d", count), func(t *testing.T) {
			service := newTestService(t)
			type client struct {
				session SessionInfo
				route   RouteBootstrap
				data    *websocket.Conn
			}
			clients := make([]client, 0, count)
			for index := 0; index < count; index++ {
				session, route := createRoute(t, service, WorkloadMetadataOnly, WorkloadOptions{MetadataFrames: 2})
				data := mustDialRoute(t, service, route.DataURL, route.DataToken)
				sendInitialCredit(t, data, route, 0)
				if err := service.StartSession(session.SessionID); err != nil {
					t.Fatal(err)
				}
				clients = append(clients, client{session: session, route: route, data: data})
			}
			if count == maxConcurrentSessions {
				if _, err := service.CreateSession(WorkloadMetadataOnly, WorkloadOptions{MetadataFrames: 1}); err == nil {
					t.Fatal("ninth active harness session was accepted")
				}
			}
			var wg sync.WaitGroup
			errorsByClient := make(chan error, count)
			for _, item := range clients {
				wg.Add(1)
				go func(item client) {
					defer wg.Done()
					for sequence := uint64(1); sequence <= 2; sequence++ {
						ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
						messageType, data, err := item.data.Read(ctx)
						cancel()
						if err != nil {
							errorsByClient <- err
							return
						}
						frame, err := UnmarshalFrame(data)
						if err != nil || messageType != websocket.MessageBinary || frame.Sequence != sequence {
							errorsByClient <- fmt.Errorf("invalid concurrent frame: %#v (%v)", frame, err)
							return
						}
						ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
						err = writeFrame(ctx, item.data, Frame{Kind: FrameCredit, Generation: frame.Generation, Sequence: frame.Sequence, CreditCost: frame.CreditCost})
						cancel()
						if err != nil {
							errorsByClient <- err
							return
						}
					}
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					_, data, err := item.data.Read(ctx)
					cancel()
					if err != nil {
						errorsByClient <- err
						return
					}
					frame, err := UnmarshalFrame(data)
					if err != nil || frame.Kind != FrameComplete {
						errorsByClient <- fmt.Errorf("missing concurrent completion: %#v (%v)", frame, err)
					}
				}(item)
			}
			wg.Wait()
			close(errorsByClient)
			for err := range errorsByClient {
				if err != nil {
					t.Fatal(err)
				}
			}
			snapshot := service.Snapshot()
			if len(snapshot.Sessions) != count {
				t.Fatalf("session count %d, want %d", len(snapshot.Sessions), count)
			}
			for _, metrics := range snapshot.Sessions {
				if !metrics.Complete || metrics.BackendMaxOutstanding > receiveWindowBytes || metrics.OutstandingBytes != 0 {
					t.Fatalf("unbounded concurrent session: %#v", metrics)
				}
			}
		})
	}
}

func TestInitialCreditAndACKMustBeExact(t *testing.T) {
	service := newTestService(t)
	session, route := createRoute(t, service, WorkloadMetadataOnly, WorkloadOptions{MetadataFrames: 1})
	data := mustDialRoute(t, service, route.DataURL, route.DataToken)
	if err := service.StartSession(session.SessionID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := writeFrame(ctx, data, Frame{Kind: FrameCredit, Generation: route.Generation, CreditCost: route.WindowBytes - 1}); err != nil {
		t.Fatal(err)
	}
	_, _, err := data.Read(ctx)
	if err == nil {
		t.Fatal("inexact initial credit was accepted")
	}
	if !errors.Is(ctx.Err(), nil) && errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("server did not actively reject inexact initial credit")
	}
}

func TestStoppedSessionHistoryRemainsBounded(t *testing.T) {
	service := newTestService(t)
	for round := 0; round < 3; round++ {
		for index := 0; index < maxConcurrentSessions; index++ {
			session, err := service.CreateSession(WorkloadMetadataOnly, WorkloadOptions{MetadataFrames: 1})
			if err != nil {
				t.Fatal(err)
			}
			if err := service.StopSession(session.SessionID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if count := len(service.Snapshot().Sessions); count > maxSessionRecords {
		t.Fatalf("session history grew to %d records, limit is %d", count, maxSessionRecords)
	}
}

func TestStopReleasesReplayPayloadAndPreservesScalarEvidence(t *testing.T) {
	service := newTestService(t)
	session, route := createRoute(t, service, WorkloadLongLine, WorkloadOptions{TotalBytes: 2 * receiveWindowBytes})
	data := mustDialRoute(t, service, route.DataURL, route.DataToken)
	sendInitialCredit(t, data, route, 0)
	if err := service.StartSession(session.SessionID); err != nil {
		t.Fatal(err)
	}
	_ = readBinaryFrame(t, data)
	waitFor(t, "session did not retain outstanding replay payload", func() bool {
		metrics := metricsFor(t, service, session.SessionID)
		return metrics.OutstandingBytes > 0 && metrics.PauseCount > 0
	})
	before := metricsFor(t, service, session.SessionID)
	if err := service.StopSession(session.SessionID); err != nil {
		t.Fatal(err)
	}
	after := metricsFor(t, service, session.SessionID)
	if after.OutstandingBytes != 0 || after.PreStopOutstandingBytes == 0 || after.ResumeCount != before.ResumeCount {
		t.Fatalf("stopped scalar evidence mismatch: before=%#v after=%#v", before, after)
	}
	service.mu.Lock()
	released := service.sessions[session.SessionID]
	if released.workload != nil || released.route != nil || released.pending != nil || len(released.records) != 0 || released.payloadHash != nil {
		service.mu.Unlock()
		t.Fatal("stopped session retained payload-producing resources")
	}
	service.mu.Unlock()
	_ = service.Snapshot()
}

func TestOutputMetricsCommitOnlyAfterSuccessfulWrite(t *testing.T) {
	service := newTestService(t)
	sessionInfo, _ := createRoute(t, service, WorkloadMetadataOnly, WorkloadOptions{MetadataFrames: 1})
	service.mu.Lock()
	session := service.sessions[sessionInfo.SessionID]
	route := session.route
	session.started = true
	route.creditReady = true
	route.available = receiveWindowBytes
	route.dataConn = &websocket.Conn{}
	service.mu.Unlock()

	outbound, ready := service.reserveOutput(session, route)
	if !ready || outbound.record == nil {
		t.Fatal("output reservation failed")
	}
	metrics := metricsFor(t, service, sessionInfo.SessionID)
	if metrics.FrameWriteAttempts != 1 || metrics.FramesSent != 0 || outbound.record.sent {
		t.Fatalf("failed/uncommitted write was counted as sent: %#v", metrics)
	}
	service.commitOutboundWrite(session, outbound)
	metrics = metricsFor(t, service, sessionInfo.SessionID)
	if metrics.FramesSent != 1 || !outbound.record.sent {
		t.Fatalf("successful write commit was not counted: %#v", metrics)
	}
	service.mu.Lock()
	route.dataConn = nil
	service.mu.Unlock()
}

func TestLateCreditDuringStopIsGraceful(t *testing.T) {
	service := newTestService(t)
	session, route := createRoute(t, service, WorkloadLongLine, WorkloadOptions{TotalBytes: receiveWindowBytes})
	data := mustDialRoute(t, service, route.DataURL, route.DataToken)
	sendInitialCredit(t, data, route, 0)
	if err := service.StartSession(session.SessionID); err != nil {
		t.Fatal(err)
	}
	frame := readBinaryFrame(t, data)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_ = service.StopSession(session.SessionID)
	}()
	go func() {
		defer wg.Done()
		<-start
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = writeFrame(ctx, data, Frame{
			Kind: FrameCredit, Generation: frame.Generation,
			Sequence: frame.Sequence, CreditCost: frame.CreditCost,
		})
	}()
	close(start)
	wg.Wait()
	waitFor(t, "session did not stop", func() bool {
		return metricsFor(t, service, session.SessionID).CloseStatus == "stopped"
	})
	metrics := metricsFor(t, service, session.SessionID)
	if metrics.Error != "" {
		t.Fatalf("late teardown credit was recorded as protocol violation: %#v", metrics)
	}
}
