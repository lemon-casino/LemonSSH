package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// The application hook remains the vault mutation owner. This router carries
// existing vault operations across Wails with correlation and cancellation.
type AgentVaultRouter struct {
	mu      sync.Mutex
	pending map[string]chan map[string]any
	emit    func(string, any)
}

func newAgentVaultRouter(emit func(string, any)) *AgentVaultRouter {
	return &AgentVaultRouter{pending: map[string]chan map[string]any{}, emit: emit}
}

func (r *AgentVaultRouter) Call(ctx context.Context, op string, params map[string]any) (map[string]any, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, err
	}
	id := "vault_" + hex.EncodeToString(token[:])
	response := make(chan map[string]any, 1)
	r.mu.Lock()
	r.pending[id] = response
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.pending, id); r.mu.Unlock() }()
	if r.emit == nil {
		return nil, fmt.Errorf("vault application bridge is unavailable")
	}
	r.emit("agent:vault-request", map[string]any{"requestId": id, "op": op, "params": params})
	timer := time.NewTimer(10 * time.Minute)
	defer timer.Stop()
	select {
	case result := <-response:
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("vault operation timed out")
	}
}

func (r *AgentVaultRouter) IsPending(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pending[id] != nil
}

func (r *AgentVaultRouter) Respond(id string, result map[string]any) error {
	r.mu.Lock()
	response := r.pending[id]
	delete(r.pending, id)
	r.mu.Unlock()
	if response == nil {
		return fmt.Errorf("vault request %q is no longer pending", id)
	}
	response <- result
	return nil
}
