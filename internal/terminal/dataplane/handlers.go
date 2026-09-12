package dataplane

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// UrgentHandler receives urgent payloads (e.g. Ctrl-C) from the renderer and
// returns the ACK payload.
type UrgentHandler func(sessionID string, payload []byte) []byte

var (
	ErrOutputBackpressure = errors.New("terminal output buffer full")
	ErrOutputClosed       = errors.New("terminal output queue closed")
)

// outputQueue bounds queued and writer-pending bytes to one receive window.
type outputQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	chunks [][]byte
	bytes  int
	closed bool
}

func newOutputQueue() *outputQueue {
	queue := &outputQueue{}
	queue.cond = sync.NewCond(&queue.mu)
	return queue
}

func (q *outputQueue) push(data []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return ErrOutputClosed
	}
	if len(data) > int(ReceiveWindowBytes)-q.bytes {
		return ErrOutputBackpressure
	}
	q.bytes += len(data)
	for len(data) > 0 {
		n := min(len(data), MaxPayloadBytes)
		q.chunks = append(q.chunks, append([]byte(nil), data[:n]...))
		data = data[n:]
	}
	q.cond.Signal()
	return nil
}

func (q *outputQueue) release(size int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		q.bytes -= size
	}
}

func (q *outputQueue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.chunks = nil
	q.bytes = 0
	q.cond.Broadcast()
}

func (q *outputQueue) isClosed() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.closed
}

func (q *outputQueue) pop() ([]byte, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.chunks) == 0 && !q.closed {
		q.cond.Wait()
	}
	if len(q.chunks) == 0 {
		return nil, false
	}
	chunk := q.chunks[0]
	q.chunks[0] = nil
	q.chunks = q.chunks[1:]
	return chunk, true
}

// Publish queues renderer-bound output for the session's current generation.
// Data larger than MaxPayloadBytes is split into frame-bounded chunks. The
// queue is created lazily so output produced before the renderer attaches is
// buffered (admitted only after the first credit grant) instead of dropped.
// Admission owns a copy and is all-or-nothing. Producers must propagate errors;
// rejected bytes have not been queued and must not be treated as delivered.
func (s *Server) Publish(sessionID string, data []byte) error {
	return s.queueFor(sessionID).push(data)
}

// DropOutput discards the session's output queue (session teardown).
func (s *Server) DropOutput(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if queue, ok := s.outputs[sessionID]; ok {
		queue.close()
		delete(s.outputs, sessionID)
	}
}

// SetUrgentHandler installs the urgent payload callback.
func (s *Server) SetUrgentHandler(handler UrgentHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.urgentHandler = handler
}

func (s *Server) sessionIDFromPath(path, prefix string) string {
	return strings.TrimPrefix(path, prefix)
}

func (s *Server) handleData(writer http.ResponseWriter, request *http.Request) {
	sessionID := s.sessionIDFromPath(request.URL.Path, "/v1/data/")
	if sessionID == "" {
		http.Error(writer, "missing session", http.StatusBadRequest)
		return
	}
	if !s.authorize(writer, request, sessionID, false) {
		return
	}
	generation := s.currentGeneration(sessionID)
	queue := s.queueFor(sessionID)

	options := &websocket.AcceptOptions{
		Subprotocols:    []string{s.dataSubprotocol, "route." + s.currentToken(sessionID, false)},
		CompressionMode: websocket.CompressionDisabled,
		// Origin is validated explicitly in authorize(); the library's pattern
		// check is skipped in favour of that boundary.
		InsecureSkipVerify: true,
	}
	connection, err := websocket.Accept(writer, request, options)
	if err != nil {
		return
	}
	connection.SetReadLimit(MaxFrameBytes)
	defer connection.Close(websocket.StatusNormalClosure, "")
	s.writeLoop(connection, sessionID, generation, queue)
}

func (s *Server) handleUrgent(writer http.ResponseWriter, request *http.Request) {
	sessionID := s.sessionIDFromPath(request.URL.Path, "/v1/urgent/")
	if sessionID == "" {
		http.Error(writer, "missing session", http.StatusBadRequest)
		return
	}
	if !s.authorize(writer, request, sessionID, true) {
		return
	}
	options := &websocket.AcceptOptions{
		Subprotocols:       []string{s.dataSubprotocol, "route." + s.currentToken(sessionID, true)},
		CompressionMode:    websocket.CompressionDisabled,
		InsecureSkipVerify: true,
	}
	connection, err := websocket.Accept(writer, request, options)
	if err != nil {
		return
	}
	connection.SetReadLimit(MaxFrameBytes)
	defer connection.Close(websocket.StatusNormalClosure, "")
	s.urgentLoop(connection, sessionID)
}

func (s *Server) writeLoop(connection *websocket.Conn, sessionID string, generation uint32, queue *outputQueue) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	defer queue.close()

	// Reader side: credit and drain-request frames from the renderer. Any
	// reader exit (error or peer close) unblocks the writer by closing the
	// queue and tearing the connection down.
	readErr := make(chan error, 1)
	go func() {
		defer connection.CloseNow()
		defer queue.close()
		var lastErr error
		for {
			_, data, err := connection.Read(ctx)
			if err != nil {
				lastErr = err
				break
			}
			frame, err := UnmarshalFrame(data)
			if err != nil {
				lastErr = err
				break
			}
			if frame.Generation != generation {
				lastErr = errors.New("stale generation credit")
				break
			}
			switch frame.Kind {
			case FrameCredit:
				if err := s.controller.ApplyCredit(sessionID, generation, frame.Sequence, frame.CreditCost); err != nil {
					lastErr = err
				}
			case FrameDrainRequest:
				s.sendFrame(ctx, connection, Frame{Kind: FrameDrainReady, Generation: generation, Correlation: frame.Correlation})
			default:
				lastErr = errors.New("data channel accepts credit and drain frames only")
			}
			if lastErr != nil {
				break
			}
		}
		readErr <- lastErr
	}()

	// Writer side: admit queued output against credit and send frames.
	var pending []byte
	for {
		select {
		case <-readErr:
			return
		default:
		}
		if pending == nil {
			chunk, ok := queue.pop()
			if !ok {
				s.sendFrame(ctx, connection, Frame{Kind: FrameComplete, Generation: generation})
				return
			}
			pending = chunk
		}
		sequence, err := s.controller.AdmitOutput(sessionID, generation, uint32(len(pending)))
		if err != nil {
			// No credit yet: keep the chunk and wait until the reader grants
			// credit or the connection errors. Bounded wait avoids a hot loop.
			time.Sleep(5 * time.Millisecond)
			continue
		}
		frame := Frame{
			Kind:       FrameOutput,
			Generation: generation,
			Sequence:   sequence,
			CreditCost: uint32(len(pending)),
			Payload:    pending,
		}
		pending = nil
		if !s.sendFrame(ctx, connection, frame) {
			return
		}
		queue.release(len(frame.Payload))
	}
}

func (s *Server) sendFrame(ctx context.Context, connection *websocket.Conn, frame Frame) bool {
	encoded, err := frame.MarshalBinary()
	if err != nil {
		return false
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if writeErr := connection.Write(writeCtx, websocket.MessageBinary, encoded); writeErr != nil {
		return false
	}
	return true
}

func (s *Server) urgentLoop(connection *websocket.Conn, sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	for {
		_, data, err := connection.Read(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			return
		}
		frame, err := UnmarshalFrame(data)
		if err != nil || frame.Kind != FrameUrgent {
			return
		}
		s.mu.Lock()
		handler := s.urgentHandler
		s.mu.Unlock()
		ackPayload := []byte{}
		if handler != nil {
			ackPayload = handler(sessionID, frame.Payload)
		}
		if !s.sendFrame(ctx, connection, Frame{
			Kind:        FrameUrgentACK,
			Generation:  frame.Generation,
			Correlation: frame.Correlation,
			Payload:     ackPayload,
		}) {
			return
		}
	}
}

func (s *Server) currentGeneration(sessionID string) uint32 {
	generation, _ := s.controller.Generation(sessionID)
	return generation
}

func (s *Server) currentToken(sessionID string, urgent bool) string {
	token, _ := s.controller.TokenFor(sessionID, urgent)
	return token
}

func (s *Server) queueFor(sessionID string) *outputQueue {
	s.mu.Lock()
	defer s.mu.Unlock()
	queue := s.outputs[sessionID]
	// A queue is closed when its WebSocket write loop exits; a reconnecting
	// renderer must get a fresh queue or it would observe an immediate
	// FrameComplete with no data.
	if queue == nil || queue.isClosed() {
		queue = newOutputQueue()
		s.outputs[sessionID] = queue
	}
	return queue
}
