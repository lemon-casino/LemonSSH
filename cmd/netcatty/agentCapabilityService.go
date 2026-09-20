package main

import (
	"context"
	"fmt"
	"time"

	"github.com/binaricat/netcatty/internal/capability"
)

func (s *AgentService) AgentCapability(ctx context.Context, method string, params map[string]any, chat string) (any, error) {
	if s.host == nil {
		return nil, fmt.Errorf("agent tool host is unavailable")
	}
	return s.host.dispatch(ctx, method, params, chat, nil)
}

func (s *AgentService) AgentUpdateSessions(chat string, sessions []AgentSession, merge bool) error {
	if s.host == nil {
		return fmt.Errorf("agent tool host is unavailable")
	}
	s.host.state.updateSessions(chat, sessions, merge)
	return nil
}

func (s *AgentService) AgentSetCancelled(chat string, cancelled bool) error {
	if s.host == nil || chat == "" {
		return fmt.Errorf("agent host and chatSessionId are required")
	}
	s.host.setChatCancelled(chat, cancelled)
	return nil
}

func (s *AgentService) AgentSetPermissionMode(mode string) error {
	if s.host == nil {
		return fmt.Errorf("agent tool host is unavailable")
	}
	if mode != "auto" && mode != "observer" && mode != "confirm" {
		return fmt.Errorf("invalid permission mode")
	}
	s.host.state.mu.Lock()
	s.host.state.mode = capability.PermissionMode(mode)
	s.host.state.mu.Unlock()
	return nil
}

func (s *AgentService) AgentSetCommandPolicy(blocklist []string, timeoutSeconds int) error {
	if s.host == nil {
		return fmt.Errorf("agent tool host is unavailable")
	}
	s.host.state.mu.Lock()
	defer s.host.state.mu.Unlock()
	if blocklist != nil {
		s.host.state.blocklist = append([]string(nil), blocklist...)
	}
	if timeoutSeconds > 0 {
		s.host.state.timeout = time.Duration(min(timeoutSeconds, 1800)) * time.Second
	}
	return nil
}

func (s *AgentService) AgentSyncPermissionGrants(grants []capability.Grant) error {
	if s.host == nil {
		return fmt.Errorf("agent tool host is unavailable")
	}
	s.host.state.mu.Lock()
	s.host.state.grants = append([]capability.Grant(nil), grants...)
	s.host.state.mu.Unlock()
	return nil
}

func (s *AgentService) AgentVaultRequestPending(id string) bool {
	return s.host != nil && s.host.vaultRouter != nil && s.host.vaultRouter.IsPending(id)
}

func (s *AgentService) AgentRespondVault(id string, result map[string]any) error {
	if s.host == nil || s.host.vaultRouter == nil {
		return fmt.Errorf("vault application bridge is unavailable")
	}
	return s.host.vaultRouter.Respond(id, result)
}
