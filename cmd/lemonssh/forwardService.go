package main

import (
	lemonsshssh "github.com/binaricat/lemonssh/internal/terminal/ssh"

	"github.com/binaricat/lemonssh/internal/app/forwarduse"
	"github.com/binaricat/lemonssh/internal/terminal/sshpool"
)

// Shell-facing DTOs. The canonical definitions (and JSON contracts) live in
// internal/app/forwarduse; these aliases keep the Wails API names stable.
type (
	PortForwardResult          = forwarduse.Result
	PortForwardListItem        = forwarduse.ListItem
	PortForwardRuntimeRecord   = forwarduse.RuntimeRecord
	PortForwardRuntimeSnapshot = forwarduse.RuntimeSnapshot
)

// portForwardRuntimeEvent is the Wails event carrying ordered tunnel-table
// changes to the renderer's runtime subscription (see
// infrastructure/runtime/wails/wailsRuntimeClient.ts). The payload is a
// forwarduse.RuntimeEvent whose revisions are contiguous per epoch.
const portForwardRuntimeEvent = "lemonssh:port-forward:runtime"

// runtimeShellEpoch is the shell identity stamped onto runtime snapshots and
// runtime events so stale rows from a previous process are detectable.
const runtimeShellEpoch = "wails"

// ForwardService is the Wails-facing facade over internal/app/forwarduse. It
// owns only shell wiring: renderer service registration, the shell epoch
// label and the ordered runtime-event transport. All forward rules (tunnel
// lifecycle, SOCKS5 handling, port binding, relaying) live in the shared
// forwarduse.Service, so a future capability dispatch entry point can call
// the same instance.
type ForwardService struct {
	core *forwarduse.Service
}

// NewForwardService wires the shared SSH pool and known-hosts store.
func NewForwardService(pool *sshpool.Pool, knownHosts *lemonsshssh.KnownHosts) *ForwardService {
	return &ForwardService{core: forwarduse.New(pool, knownHosts)}
}

// setEventEmitter wires the shell event transport (unexported on purpose:
// main-process wiring, not a renderer-callable API). Every tunnel-table
// change in the core is then broadcast as one ordered RuntimeEvent so the
// renderer stops relying on its 4s heartbeat poll alone.
func (s *ForwardService) setEventEmitter(emit func(name string, payload any)) {
	s.core.SetRuntimeEpoch(runtimeShellEpoch)
	s.core.OnRuntimeEvent(func(event forwarduse.RuntimeEvent) {
		if emit == nil {
			return
		}
		emit(portForwardRuntimeEvent, event)
	})
}

// Start starts (or reuses) one tunnel for the rule encoded in id.
func (s *ForwardService) Start(id, kind, bindHost string, bindPort uint16, targetHost string, targetPort uint16, request SSHConnectRequest) PortForwardResult {
	return s.core.Start(id, kind, bindHost, bindPort, targetHost, targetPort, request)
}

// Stop stops one tunnel and returns its lease to the pool.
func (s *ForwardService) Stop(id string) PortForwardResult { return s.core.Stop(id) }

// StopByRuleId stops every tunnel started for one rule.
func (s *ForwardService) StopByRuleId(ruleID string) map[string]any {
	return s.core.StopByRuleId(ruleID)
}

// List reports all active tunnels with their live status.
func (s *ForwardService) List() []PortForwardListItem { return s.core.List() }

// Snapshot reports one tunnel's status.
func (s *ForwardService) Snapshot(id string) PortForwardResult { return s.core.Snapshot(id) }

// RuntimeSnapshot renders the tunnel table for the renderer poll, stamped
// with this shell's epoch.
func (s *ForwardService) RuntimeSnapshot() PortForwardRuntimeSnapshot {
	return s.core.RuntimeSnapshot(runtimeShellEpoch)
}
