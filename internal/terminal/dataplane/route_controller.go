package dataplane

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

const ReceiveWindowBytes uint64 = 1024 * 1024

var (
	ErrRouteNotFound      = errors.New("terminal route not found")
	ErrStaleGeneration    = errors.New("terminal route generation is stale")
	ErrInsufficientCredit = errors.New("terminal route has insufficient credit")
	ErrCreditSequence     = errors.New("terminal credit sequence is invalid")
)

// RouteBootstrap is the shell-neutral route handoff returned to a Wails/UI
// adapter. Tokens are one-use route credentials; the listener is responsible
// for binding them to Host/Origin before calling Authenticate.
type RouteBootstrap struct {
	SessionID   string
	Generation  uint32
	DataToken   string
	UrgentToken string
	WindowBytes uint32
}

type routeState struct {
	sessionID   string
	generation  uint32
	dataToken   string
	urgentToken string
	creditReady bool
	available   uint64
	sentThrough uint64
	applied     uint64
}

// RouteController owns route generations and credit admission. It has no
// WebSocket dependency; transport adapters call Authenticate, ApplyCredit and
// AdmitOutput under their own connection lifecycle.
type RouteController struct {
	mu     sync.Mutex
	routes map[string]*routeState
}

func NewRouteController() *RouteController {
	return &RouteController{routes: make(map[string]*routeState)}
}

func (c *RouteController) Open(sessionID string) (RouteBootstrap, error) {
	if sessionID == "" {
		return RouteBootstrap{}, ErrRouteNotFound
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.routes[sessionID]
	generation := uint32(1)
	if state != nil {
		generation = state.generation + 1
	}
	dataToken, err := randomToken()
	if err != nil {
		return RouteBootstrap{}, err
	}
	urgentToken, err := randomToken()
	if err != nil {
		return RouteBootstrap{}, err
	}
	state = &routeState{sessionID: sessionID, generation: generation, dataToken: dataToken, urgentToken: urgentToken}
	c.routes[sessionID] = state
	return RouteBootstrap{SessionID: sessionID, Generation: generation, DataToken: dataToken, UrgentToken: urgentToken, WindowBytes: uint32(ReceiveWindowBytes)}, nil
}

func (c *RouteController) Close(sessionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.routes[sessionID]; !ok {
		return ErrRouteNotFound
	}
	delete(c.routes, sessionID)
	return nil
}

func (c *RouteController) Authenticate(sessionID string, generation uint32, token string, urgent bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	state, ok := c.routes[sessionID]
	if !ok {
		return ErrRouteNotFound
	}
	if state.generation != generation {
		return ErrStaleGeneration
	}
	expected := state.dataToken
	if urgent {
		expected = state.urgentToken
	}
	if len(token) != len(expected) || subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
		return errors.New("invalid route token")
	}
	return nil
}

// ApplyCredit returns output credit after xterm has applied through sequence.
// The first credit grant must exactly open the bounded receive window.
func (c *RouteController) ApplyCredit(sessionID string, generation uint32, appliedSequence uint64, credit uint32) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	state, ok := c.routes[sessionID]
	if !ok {
		return ErrRouteNotFound
	}
	if state.generation != generation {
		return ErrStaleGeneration
	}
	if appliedSequence < state.applied || appliedSequence > state.sentThrough {
		return ErrCreditSequence
	}
	if !state.creditReady {
		if credit != uint32(ReceiveWindowBytes) || appliedSequence != state.applied {
			return fmt.Errorf("initial credit must grant %d at sequence %d", ReceiveWindowBytes, state.applied)
		}
		state.creditReady = true
	}
	state.applied = appliedSequence
	state.available += uint64(credit)
	return nil
}

// AdmitOutput reserves credit for one output frame.
func (c *RouteController) AdmitOutput(sessionID string, generation uint32, payloadBytes uint32) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state, ok := c.routes[sessionID]
	if !ok {
		return 0, ErrRouteNotFound
	}
	if state.generation != generation {
		return 0, ErrStaleGeneration
	}
	if !state.creditReady || uint64(payloadBytes) > state.available {
		return 0, ErrInsufficientCredit
	}
	state.available -= uint64(payloadBytes)
	state.sentThrough++
	return state.sentThrough, nil
}

func (c *RouteController) Snapshot(sessionID string) (generation uint32, available uint64, sentThrough uint64, applied uint64, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state, ok := c.routes[sessionID]
	if !ok {
		return 0, 0, 0, 0, ErrRouteNotFound
	}
	return state.generation, state.available, state.sentThrough, state.applied, nil
}

func randomToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
