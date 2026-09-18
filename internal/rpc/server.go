package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// Handler executes one authorized request. Params is the raw JSON envelope
// payload; handlers decode their own typed shapes so unknown fields fail at
// the domain boundary, not the transport.
type Handler func(ctx context.Context, principal *Principal, params json.RawMessage) (any, error)

// ServerOptions bounds and shapes a Server.
type ServerOptions struct {
	MaxFrameBytes int64
	// MaxDeadline caps client-requested deadlines; zero disables the cap
	// and DeadlineDefault applies when the client sends none.
	MaxDeadline     time.Duration
	DefaultDeadline time.Duration
}

func (o ServerOptions) withDefaults() ServerOptions {
	if o.MaxFrameBytes <= 0 {
		o.MaxFrameBytes = DefaultMaxFrameBytes
	}
	if o.DefaultDeadline <= 0 {
		o.DefaultDeadline = 30 * time.Second
	}
	if o.MaxDeadline < 0 {
		o.MaxDeadline = 0
	}
	return o
}

// Server serves authenticated host RPC over byte connections (stdio, loopback
// TCP). One Server owns one TokenStore; Close revokes every token and tears
// down tracked connections, so app shutdown and lock-screen revocation have
// a single entry point.
type Server struct {
	tokens   *TokenStore
	handlers map[string]Handler
	options  ServerOptions

	mu      sync.Mutex
	closed  bool
	conns   map[string]chan struct{} // per-connection stop signal
	closers []io.Closer
}

// NewServer wires the server to its token store and handler table.
func NewServer(tokens *TokenStore, handlers map[string]Handler, options ServerOptions) *Server {
	return &Server{
		tokens:   tokens,
		handlers: handlers,
		options:  options.withDefaults(),
		conns:    map[string]chan struct{}{},
	}
}

// ServeConn runs one connection until EOF, a fatal transport error, or
// Close. Requests on one connection are answered in order; oversized or
// malformed frames produce typed error responses and the connection stays
// usable. The writer receives frames only — never logs.
func (s *Server) ServeConn(ctx context.Context, remote string, reader io.Reader, writer io.Writer) {
	stop := make(chan struct{})
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = EncodeResponse(writer, &Response{Version: ProtocolVersion, Error: &ResponseError{Code: CodeServerClosing, Message: "server is shutting down"}})
		return
	}
	s.conns[remote] = stop
	s.mu.Unlock()
	// The stop channel is closed only by Close; on normal EOF the channel
	// is simply abandoned with the connection entry removed.
	defer func() {
		s.mu.Lock()
		delete(s.conns, remote)
		s.mu.Unlock()
	}()

	buffered := bufio.NewReader(reader)
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		default:
		}

		response, fatal := s.readAndDispatch(ctx, buffered)
		if response != nil {
			if err := EncodeResponse(writer, response); err != nil {
				return
			}
		}
		if fatal {
			return
		}
	}
}

// readAndDispatch reads one frame and produces its response. The bool
// reports a connection-fatal transport condition (EOF, closed writer,
// unrecoverable read error).
func (s *Server) readAndDispatch(ctx context.Context, buffered *bufio.Reader) (*Response, bool) {
	frame, err := ReadFrame(buffered, s.options.MaxFrameBytes)
	if err != nil {
		switch {
		case errors.Is(err, io.EOF):
			return nil, true
		case errors.Is(err, ErrFrameTooLarge), errors.Is(err, ErrFrameTruncated), errors.Is(err, ErrFrameMalformed):
			return s.errorResponse("", CodeBadRequest, err.Error()), false
		default:
			return nil, true
		}
	}

	envelope, err := DecodeEnvelope(frame)
	if err != nil {
		code := CodeBadRequest
		if errors.Is(err, ErrVersionUnsupported) {
			code = CodeVersionMismatch
		}
		return s.errorResponse("", code, err.Error()), false
	}

	return s.authorizedCall(ctx, envelope), false
}

// authorizedCall authenticates, resolves the deadline and executes the
// handler. Every failure path returns a typed code.
func (s *Server) authorizedCall(ctx context.Context, envelope *Envelope) *Response {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return s.errorResponse(envelope.ID, CodeServerClosing, "server is shutting down")
	}

	if envelope.Token == "" {
		return s.errorResponse(envelope.ID, CodeAuthFailed, "missing token")
	}
	principal, err := s.tokens.Verify(envelope.Token)
	if err != nil {
		return s.errorResponse(envelope.ID, CodeAuthFailed, "token rejected")
	}

	handler := s.handlers[envelope.Method]
	if handler == nil {
		return s.errorResponse(envelope.ID, CodeUnknownMethod, fmt.Sprintf("no handler for %q", envelope.Method))
	}

	callCtx, cancel := s.callContext(ctx, envelope)
	defer cancel()
	result, err := handler(callCtx, principal, envelope.Params)
	if err != nil {
		return s.errorResponse(envelope.ID, errorCodeFor(err), err.Error())
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return s.errorResponse(envelope.ID, CodeBadRequest, fmt.Sprintf("result encoding failed: %v", err))
	}
	return &Response{Version: ProtocolVersion, ID: envelope.ID, OK: true, Result: raw}
}

func (s *Server) callContext(ctx context.Context, envelope *Envelope) (context.Context, context.CancelFunc) {
	deadline := s.options.DefaultDeadline
	if envelope.DeadlineMS > 0 {
		deadline = time.Duration(envelope.DeadlineMS) * time.Millisecond
		if s.options.MaxDeadline > 0 && deadline > s.options.MaxDeadline {
			deadline = s.options.MaxDeadline
		}
	}
	return context.WithTimeout(ctx, deadline)
}

// errorCodeFor maps handler errors to stable wire codes. Errors exposing
// their own code (capability dispatch failures, contracts errors) win;
// scope refusals keep SCOPE_DENIED; context deadlines map to the deadline
// code.
func errorCodeFor(err error) string {
	var scopeErr *ScopeError
	if errors.As(err, &scopeErr) {
		return scopeErr.Code
	}
	var coded interface{ ErrorCode() string }
	if errors.As(err, &coded) {
		return coded.ErrorCode()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return CodeDeadline
	}
	return CodeBadRequest
}

// ScopeError lets handlers deny out-of-scope session/chat references with
// the typed SCOPE_DENIED code.
type ScopeError struct {
	Code    string
	Message string
}

func (e *ScopeError) Error() string { return e.Code + ": " + e.Message }

// RequireSession enforces principal scope on a session/chat reference
// (T42: forged session parameters must not escalate).
func RequireSession(principal *Principal, sessionID string) error {
	if sessionID == "" {
		return &ScopeError{Code: CodeBadRequest, Message: "sessionId is required"}
	}
	if !principal.AllowsSession(sessionID) {
		return &ScopeError{Code: CodeScopeDenied, Message: fmt.Sprintf("session %q is not in scope", sessionID)}
	}
	return nil
}

func (s *Server) errorResponse(id, code, message string) *Response {
	return &Response{Version: ProtocolVersion, ID: id, Error: &ResponseError{Code: code, Message: message}}
}

// Close revokes all tokens and stops tracked connections. In-flight handler
// calls observe their context cancellation on the next scheduling point.
func (s *Server) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	stops := make([]chan struct{}, 0, len(s.conns))
	for _, stop := range s.conns {
		stops = append(stops, stop)
	}
	s.conns = map[string]chan struct{}{}
	s.mu.Unlock()

	s.tokens.RevokeAll()
	for _, stop := range stops {
		close(stop)
	}
}
