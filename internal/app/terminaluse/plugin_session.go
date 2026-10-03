package terminaluse

import (
	"fmt"
	"io"

	"github.com/binaricat/lemonssh/internal/terminal/dataplane"
)

// PluginSessionHooks drive a plugin-protocol connection (an internal/plugin
// provider of kind "connection") from the plugin host. The session pool, the
// data-plane route, flow control, output observers and exit tracking stay
// owned by this service; the hooks only carry bytes and control operations
// between the pool and the plugin's live connection.
type PluginSessionHooks struct {
	// Write forwards renderer stdin bytes to the plugin connection.
	Write func(data []byte) (int, error)
	// Resize forwards terminal dimension changes (nil = ignored).
	Resize func(cols, rows uint16) error
	// Signal forwards POSIX-style signals, e.g. "KILL" (nil = ignored).
	Signal func(signal string) error
	// Close tears the plugin-side connection down. It must be idempotent:
	// the pool calls it on renderer-initiated closes AND on plugin-initiated
	// closes reported through PluginSessionClosed.
	Close func(reason string)
}

// stdinFunc adapts a plugin write hook to the session pool's stdin seam
// (terminaluse.Service.Write falls through to term.stdin after the transport
// branches).
type stdinFunc func(data []byte) (int, error)

func (f stdinFunc) Write(data []byte) (int, error) { return f(data) }
func (f stdinFunc) Close() error                   { return nil }

var _ io.WriteCloser = stdinFunc(nil)

// RegisterPluginSession registers a plugin-protocol connection as a first
// class terminal session: the renderer attaches through the ordinary
// data-plane bootstrap, and Write/Resize/Signal/Close route through the
// hooks. The returned bootstrap is the route credential the renderer
// exchanges for its data/urgent WebSockets — identical to native sessions.
func (s *Service) RegisterPluginSession(sessionID, label string, hooks PluginSessionHooks) (dataplane.RouteBootstrap, error) {
	if sessionID == "" {
		return dataplane.RouteBootstrap{}, fmt.Errorf("plugin session id is required")
	}
	if hooks.Write == nil {
		return dataplane.RouteBootstrap{}, fmt.Errorf("plugin session requires a write hook")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.sessions[sessionID]; ok && existing != nil && !existing.closing {
		return dataplane.RouteBootstrap{}, fmt.Errorf("session %q already exists", sessionID)
	}
	bootstrap, err := s.controller.Open(label)
	if err != nil {
		return dataplane.RouteBootstrap{}, err
	}
	// Copy the hooks so callers cannot mutate them after registration.
	stored := hooks
	s.sessions[sessionID] = &terminalSession{
		bootstrap:   bootstrap,
		pluginHooks: &stored,
		stdin:       stdinFunc(stored.Write),
	}
	return bootstrap, nil
}

// PublishPluginOutput forwards plugin connection output into the session's
// ordinary pipeline: zmodem detection, cwd OSC parsing, flow-control leases,
// output observers (script runner / session log) and the data-plane publish.
// It reports whether the session still exists and accepted the bytes.
func (s *Service) PublishPluginOutput(sessionID string, data []byte) bool {
	s.mu.Lock()
	term, ok := s.sessions[sessionID]
	s.mu.Unlock()
	if !ok || term == nil || term.closing {
		return false
	}
	return s.publishOutput(sessionID, data)
}

// PluginSessionClosed ends a plugin-protocol session from the plugin side.
// Exit tracking and the terminal:exit event run exactly like a
// renderer-initiated close; the plugin Close hook is still invoked (it must
// be idempotent) so host-side state is released through one path.
func (s *Service) PluginSessionClosed(sessionID, reason string) {
	if reason == "" {
		reason = "closed"
	}
	s.beginSessionClose(sessionID, TerminalExitStatus{SessionID: sessionID, Reason: reason}, true)
}
