package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/binaricat/netcatty/internal/rpc"
)

const externalAgentChat = "__external_mcp__"

type ExternalAgentConfig struct {
	Mode                      string `json:"mode"`
	IdleTimeoutMinutes        int    `json:"idleTimeoutMinutes"`
	SessionIdleTimeoutMinutes int    `json:"sessionIdleTimeoutMinutes"`
}

type externalAgentAccess struct {
	mu           sync.Mutex
	enabled      bool
	token        string
	path         string
	lastActivity time.Time
	config       ExternalAgentConfig
	timer        *time.Timer
	generation   uint64
	sessions     map[string]time.Time
}

func (h *AgentHost) externalStatus() map[string]any {
	e := &h.external
	e.mu.Lock()
	enabled, config, path, activity := e.enabled, e.config, e.path, e.lastActivity
	e.mu.Unlock()
	state := "disabled"
	if enabled {
		state = "running"
	}
	exe, _ := os.Executable()
	name := "LemonSSH-mcp"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	launcher := filepath.Join(filepath.Dir(exe), name)
	if _, err := os.Stat(launcher); err != nil {
		if cwd, err := os.Getwd(); err == nil {
			for _, dir := range []string{"bin", filepath.Join("dist", "wails")} {
				candidate := filepath.Join(cwd, dir, name)
				if _, err := os.Stat(candidate); err == nil {
					launcher = candidate
					break
				}
			}
		}
	}
	var expires any
	if enabled && config.Mode == "temporary" {
		expires = activity.Add(time.Duration(config.IdleTimeoutMinutes) * time.Minute).UnixMilli()
	}
	count := 0
	if enabled {
		count = len(h.state.visible(externalAgentChat, h.sessions()))
	}
	return map[string]any{"ok": true, "enabled": enabled, "state": state, "host": "127.0.0.1", "hostRunning": h.listener != nil, "discoveryPath": path, "launcherPath": launcher, "chatSessionId": externalAgentChat, "exposedSessionCount": count, "permissionMode": h.state.permissionMode(), "mode": config.Mode, "idleTimeoutMinutes": config.IdleTimeoutMinutes, "sessionIdleTimeoutMinutes": config.SessionIdleTimeoutMinutes, "lastActivityAt": activity.UnixMilli(), "idleExpiresAt": expires}
}

func (h *AgentHost) setExternalEnabled(enabled bool) (map[string]any, error) {
	e := &h.external
	e.mu.Lock()
	if enabled && !e.enabled {
		if h.tokens == nil || h.listener == nil || h.discoveryPath == "" {
			e.mu.Unlock()
			return nil, fmt.Errorf("agent RPC host is not running")
		}
		token, err := h.tokens.Issue(rpc.Principal{ID: "external-mcp", Kind: rpc.PrincipalExternal})
		if err != nil {
			e.mu.Unlock()
			return nil, err
		}
		e.path = filepath.Join(filepath.Dir(h.discoveryPath), "external-mcp-discovery.json")
		err = rpc.WriteDiscovery(e.path, rpc.Discovery{Port: h.listener.Addr().(*net.TCPAddr).Port, Token: token, PID: os.Getpid(), PermissionMode: string(h.state.permissionMode())})
		if err != nil {
			h.tokens.Revoke(token)
			e.mu.Unlock()
			return nil, err
		}
		e.token, e.enabled, e.lastActivity = token, true, time.Now()
		h.setChatCancelled(externalAgentChat, false)
	} else if !enabled {
		h.disableExternalLocked()
	}
	h.armExternalTimerLocked()
	e.mu.Unlock()
	return h.externalStatus(), nil
}

func (h *AgentHost) disableExternalLocked() {
	e := &h.external
	e.enabled = false
	if e.token != "" && h.tokens != nil {
		h.tokens.Revoke(e.token)
	}
	e.token = ""
	if e.path != "" {
		_ = rpc.RemoveDiscovery(e.path)
	}
	h.setChatCancelled(externalAgentChat, true)
}

func (h *AgentHost) touchExternal() bool {
	e := &h.external
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.enabled {
		return false
	}
	e.lastActivity = time.Now()
	h.armExternalTimerLocked()
	return true
}

func (h *AgentHost) touchExternalSession(id string, opened bool) {
	if id == "" {
		return
	}
	e := &h.external
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, owned := e.sessions[id]; opened || owned {
		e.sessions[id] = time.Now()
		h.armExternalTimerLocked()
	}
}

// One timer covers access expiry and sessions created by host_open. It never
// closes the user's pre-existing terminals, and stale timers cannot revoke a
// newer enable decision.
func (h *AgentHost) armExternalTimerLocked() {
	e := &h.external
	if e.timer != nil {
		e.timer.Stop()
	}
	e.generation++
	generation := e.generation
	var next time.Time
	if e.enabled && e.config.Mode == "temporary" {
		next = e.lastActivity.Add(time.Duration(e.config.IdleTimeoutMinutes) * time.Minute)
	}
	for _, activity := range e.sessions {
		deadline := activity.Add(time.Duration(e.config.SessionIdleTimeoutMinutes) * time.Minute)
		if next.IsZero() || deadline.Before(next) {
			next = deadline
		}
	}
	if next.IsZero() {
		return
	}
	e.timer = time.AfterFunc(max(time.Millisecond, time.Until(next)), func() {
		e.mu.Lock()
		if e.generation != generation {
			e.mu.Unlock()
			return
		}
		now := time.Now()
		if e.enabled && e.config.Mode == "temporary" && now.Sub(e.lastActivity) >= time.Duration(e.config.IdleTimeoutMinutes)*time.Minute {
			h.disableExternalLocked()
		}
		var expired []string
		for id, activity := range e.sessions {
			if now.Sub(activity) >= time.Duration(e.config.SessionIdleTimeoutMinutes)*time.Minute {
				expired = append(expired, id)
				delete(e.sessions, id)
			}
		}
		h.armExternalTimerLocked()
		e.mu.Unlock()
		for _, id := range expired {
			if h.vaultRouter != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				_, _ = h.vaultRouter.Call(ctx, "session.close", map[string]any{"sessionId": id, "chatSessionId": externalAgentChat})
				cancel()
			}
		}
	})
}

func (s *AgentService) AgentExternalStatus() (map[string]any, error) {
	if s.host == nil {
		return nil, fmt.Errorf("agent host is unavailable")
	}
	return s.host.externalStatus(), nil
}
func (s *AgentService) AgentExternalSetEnabled(enabled bool) (map[string]any, error) {
	if s.host == nil {
		return nil, fmt.Errorf("agent host is unavailable")
	}
	return s.host.setExternalEnabled(enabled)
}
func (s *AgentService) AgentExternalSetConfig(config ExternalAgentConfig) (map[string]any, error) {
	if s.host == nil {
		return nil, fmt.Errorf("agent host is unavailable")
	}
	e := &s.host.external
	e.mu.Lock()
	if config.Mode == "temporary" || config.Mode == "persistent" {
		e.config.Mode = config.Mode
	}
	if config.IdleTimeoutMinutes > 0 {
		e.config.IdleTimeoutMinutes = min(1440, config.IdleTimeoutMinutes)
	}
	if config.SessionIdleTimeoutMinutes > 0 {
		e.config.SessionIdleTimeoutMinutes = min(1440, config.SessionIdleTimeoutMinutes)
	}
	s.host.armExternalTimerLocked()
	e.mu.Unlock()
	return s.host.externalStatus(), nil
}
