package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/binaricat/netcatty/internal/capability"
	"github.com/binaricat/netcatty/internal/rpc"
)

// AgentSession is renderer-owned metadata. NativeID is used only to resolve
// the terminal transport; tools continue to see the UI session identity.
type AgentSession struct {
	SessionID          string           `json:"sessionId"`
	NativeID           string           `json:"nativeSessionId,omitempty"`
	HostID             string           `json:"hostId,omitempty"`
	Hostname           string           `json:"hostname"`
	Label              string           `json:"label"`
	OS                 string           `json:"os,omitempty"`
	Username           string           `json:"username,omitempty"`
	Protocol           string           `json:"protocol,omitempty"`
	ShellType          string           `json:"shellType,omitempty"`
	DeviceType         string           `json:"deviceType,omitempty"`
	Connected          bool             `json:"connected"`
	HostChain          []map[string]any `json:"hostChain,omitempty"`
	ActivePortForwards []map[string]any `json:"activePortForwards,omitempty"`
}

type agentToolState struct {
	mu        sync.Mutex
	scopes    map[string][]AgentSession
	live      map[string]AgentSession
	aliases   map[string]string
	owned     map[string]map[string]bool
	cancelled map[string]bool
	active    map[uint64]activeToolCall
	next      uint64
	mode      capability.PermissionMode
	grants    []capability.Grant
	blocklist []string
	timeout   time.Duration
}

type activeToolCall struct {
	chat   string
	cancel context.CancelFunc
}

type agentPrincipalContextKey struct{}

func newAgentToolState(mode string) *agentToolState {
	return &agentToolState{scopes: map[string][]AgentSession{}, aliases: map[string]string{}, owned: map[string]map[string]bool{}, cancelled: map[string]bool{}, active: map[uint64]activeToolCall{}, mode: permissionModeOf(mode), timeout: time.Minute}
}

func (s *agentToolState) updateSessions(chat string, sessions []AgentSession, merge bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if chat == "" {
		s.live = map[string]AgentSession{}
		for _, session := range sessions {
			s.live[session.SessionID] = session
		}
	} else {
		out := []AgentSession{}
		if merge {
			out = append(out, s.scopes[chat]...)
		}
		for _, session := range sessions {
			if session.SessionID == "" {
				continue
			}
			found := false
			for i := range out {
				if out[i].SessionID == session.SessionID {
					out[i] = session
					found = true
					break
				}
			}
			if !found {
				out = append(out, session)
			}
		}
		s.scopes[chat] = out
	}
	for _, session := range sessions {
		if session.NativeID != "" {
			s.aliases[session.SessionID] = session.NativeID
		}
	}
}

func (s *agentToolState) visible(chat string, fallback []SessionEntry) []AgentSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	var entries []AgentSession
	if scoped, exists := s.scopes[chat]; exists {
		entries = append(entries, scoped...)
		if s.live != nil {
			for i, entry := range entries {
				if current, ok := s.live[entry.SessionID]; ok {
					entries[i] = current
				} else {
					entries[i].Connected = false
				}
			}
		}
	} else if chat == "" || chat == "__external_mcp__" {
		if s.live != nil {
			for _, entry := range s.live {
				entries = append(entries, entry)
			}
		} else {
			for _, entry := range fallback {
				entries = append(entries, AgentSession{SessionID: entry.ID, Label: entry.Label, HostID: entry.HostID, Protocol: entry.Kind, Connected: true})
			}
		}
	} else if s.live == nil && len(s.scopes) == 0 {
		// Headless hosts supply their own authoritative session inventory.
		for _, entry := range fallback {
			entries = append(entries, AgentSession{SessionID: entry.ID, Label: entry.Label, HostID: entry.HostID, Protocol: entry.Kind, Connected: true})
		}
	}
	for i := range entries {
		entries[i].NativeID = ""
	}
	if entries == nil {
		entries = []AgentSession{}
	}
	return entries
}

func (s *agentToolState) nativeID(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if native := s.aliases[id]; native != "" {
		return native
	}
	return id
}

func (s *agentToolState) isCancelled(chat string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancelled[chat]
}
func (s *agentToolState) permissionMode() capability.PermissionMode {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode
}
func (s *agentToolState) permissionGrants() []capability.Grant {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]capability.Grant(nil), s.grants...)
}

func (s *agentToolState) begin(ctx context.Context, chat string, def *capability.Definition) (context.Context, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelled[chat] && !def.Policy.BypassesChatCancel {
		return nil, nil, &capability.DispatchError{Code: capability.CodePolicyDenied, Message: capability.ChatSessionCancelledMsg}
	}
	callCtx, cancel := context.WithCancel(ctx)
	s.next++
	id := s.next
	if !def.Policy.BypassesChatCancel {
		s.active[id] = activeToolCall{chat: chat, cancel: cancel}
	}
	return callCtx, func() { cancel(); s.mu.Lock(); delete(s.active, id); s.mu.Unlock() }, nil
}

func (h *AgentHost) setChatCancelled(chat string, cancelled bool) {
	h.state.mu.Lock()
	h.state.cancelled[chat] = cancelled
	var cancels []context.CancelFunc
	if cancelled {
		for _, call := range h.state.active {
			if call.chat == chat {
				cancels = append(cancels, call.cancel)
			}
		}
	}
	h.state.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	if cancelled && h.jobs != nil {
		h.jobs.CancelOwner(chat)
	}
}

func (h *AgentHost) requireScopedSession(chat, id string) error {
	for _, entry := range h.state.visible(chat, h.sessions()) {
		if entry.SessionID == id {
			return nil
		}
	}
	return &rpc.ScopeError{Code: rpc.CodeScopeDenied, Message: fmt.Sprintf("session %q is not in this chat's scope", id)}
}

// dispatch is shared by Wails, the provider driver, CLI and MCP. Callers may
// supply tool arguments, never a permission mode or a replacement chat scope.
func (h *AgentHost) dispatch(ctx context.Context, method string, input map[string]any, chat string, principal *rpc.Principal) (any, error) {
	params := make(map[string]any, len(input)+1)
	for key, value := range input {
		params[key] = value
	}
	params["chatSessionId"] = chat
	if principal != nil && principal.Kind == rpc.PrincipalExternal {
		if !h.touchExternal() {
			return nil, &rpc.ScopeError{Code: rpc.CodeAuthFailed, Message: "External MCP is disabled"}
		}
		if id, _ := params["sessionId"].(string); id != "" {
			h.touchExternalSession(id, false)
		}
	}
	ctx = context.WithValue(ctx, agentPrincipalContextKey{}, principal)
	var def *capability.Definition
	var surface capability.Surface
	for _, candidate := range []capability.Surface{capability.SurfaceBuiltin, capability.SurfaceGlobal, capability.SurfacePublic} {
		if resolved := capability.Default().GetByRPCMethod(method, candidate); resolved != nil {
			def, surface = resolved, candidate
			break
		}
	}
	if def == nil {
		return nil, &capability.DispatchError{Code: capability.CodeUnknownCapability, Message: "Unknown tool method: " + method}
	}
	if id, _ := params["sessionId"].(string); id != "" {
		if principal != nil && len(principal.Scope) > 0 && !principal.AllowsSession(id) {
			return nil, rpc.RequireSession(principal, id)
		}
		if err := h.requireScopedSession(chat, id); err != nil {
			return nil, err
		}
	}
	if strings.HasPrefix(def.ID, "terminal.") && (def.ID == "terminal.execute" || def.ID == "terminal.start") {
		command, _ := params["command"].(string)
		h.state.mu.Lock()
		patterns := append([]string(nil), h.state.blocklist...)
		h.state.mu.Unlock()
		for _, pattern := range patterns {
			re, err := regexp.Compile("(?i)" + pattern)
			if pattern != "" && err == nil && re.MatchString(command) {
				return nil, &capability.DispatchError{Code: capability.CodePolicyDenied, Message: "Command blocked by safety policy: " + pattern}
			}
		}
	}
	callCtx, finish, err := h.state.begin(ctx, chat, def)
	if err != nil {
		return nil, err
	}
	defer finish()
	dispatcher := &capability.Dispatcher{Registry: capability.Default(), Surface: surface, PermissionMode: h.state.permissionMode(), Handlers: h.capabilityHandlers(), Approval: h.approvals, Grants: h.state.permissionGrants, ChatCancelled: func() bool { return h.state.isCancelled(chat) }}
	return dispatcher.Dispatch(callCtx, method, params)
}
