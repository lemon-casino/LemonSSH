package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/binaricat/netcatty/internal/capability"
)

// approvalTimeout is the bounded wait for a user decision (ported from
// DEFAULT_APPROVAL_TIMEOUT_MS).
const approvalTimeout = 110 * time.Second

// pendingApproval is one open approval prompt. respond carries exactly one
// boolean; the deadline and context both race against it.
type pendingApproval struct {
	id           string
	capabilityID string
	summary      map[string]any
	deadline     time.Time
	respond      chan bool
	respondOnce  sync.Once
}

// InteractionRouter is the host-side approval gate (W13): it turns
// capability approval demands into renderer-visible prompts and blocks the
// calling dispatch until the user decides, the deadline lapses or the
// context cancels. Decisions are consumed exactly once (design §6.1).
type InteractionRouter struct {
	timeout time.Duration
	emit    func(name string, payload any)
	now     func() time.Time

	mu      sync.Mutex
	pending map[string]*pendingApproval
	counter int
}

func newInteractionRouter(emit func(name string, payload any)) *InteractionRouter {
	return &InteractionRouter{
		timeout: approvalTimeout,
		emit:    emit,
		now:     time.Now,
		pending: map[string]*pendingApproval{},
	}
}

// RequestApproval implements capability.ApprovalGate.
func (r *InteractionRouter) RequestApproval(ctx context.Context, req capability.Request, def *capability.Definition) (bool, error) {
	interactionID := r.newID()
	pending := &pendingApproval{
		id:           interactionID,
		capabilityID: def.ID,
		deadline:     r.now().Add(r.timeout),
		respond:      make(chan bool, 1),
	}

	r.mu.Lock()
	r.pending[interactionID] = pending
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.pending, interactionID)
		r.mu.Unlock()
	}()

	if r.emit != nil {
		r.emit("agent:interaction", map[string]any{
			"interactionId": interactionID,
			"capabilityId":  def.ID,
			"description":   def.Description,
			"summary":       approvalSummary(req),
			"deadlineMs":    pending.deadline.UnixMilli(),
		})
	}

	timer := time.NewTimer(time.Until(pending.deadline))
	defer timer.Stop()
	select {
	case approved := <-pending.respond:
		return approved, nil
	case <-timer.C:
		return false, nil
	case <-ctx.Done():
		return false, nil
	}
}

// Respond resolves one pending approval. Unknown or already-resolved IDs
// fail typed instead of silently succeeding (double-response guard).
func (r *InteractionRouter) Respond(interactionID string, approved bool) error {
	r.mu.Lock()
	pending := r.pending[interactionID]
	r.mu.Unlock()
	if pending == nil {
		return fmt.Errorf("interaction %q is not pending (already resolved or unknown)", interactionID)
	}
	var delivered bool
	pending.respondOnce.Do(func() {
		delivered = true
		pending.respond <- approved
	})
	if !delivered {
		return fmt.Errorf("interaction %q was already resolved", interactionID)
	}
	return nil
}

// Pending lists open approvals for the settings UI. Summaries carry no
// secret material — params were already sanitized by policy.
func (r *InteractionRouter) Pending() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]map[string]any, 0, len(r.pending))
	for _, pending := range r.pending {
		out = append(out, map[string]any{
			"interactionId": pending.id,
			"capabilityId":  pending.capabilityID,
			"summary":       pending.summary,
			"deadlineMs":    pending.deadline.UnixMilli(),
		})
	}
	return out
}

// Resolve drains one pending approval directly (host shutdown path).
func (r *InteractionRouter) resolve(id string) bool {
	r.mu.Lock()
	pending := r.pending[id]
	r.mu.Unlock()
	if pending == nil {
		return false
	}
	pending.respondOnce.Do(func() { pending.respond <- false })
	return true
}

func (r *InteractionRouter) newID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("ia_%d", r.now().UnixNano())
	}
	return "ia_" + hex.EncodeToString(raw[:])
}

// approvalSummary projects the request into user-visible fields. The
// command/text bodies are shown (that is what the user is approving);
// nothing else is added here.
func approvalSummary(req capability.Request) map[string]any {
	summary := map[string]any{
		"method": req.RPCMethod,
	}
	for _, field := range []string{"sessionId", "command", "hostId", "ruleId", "path", "remotePath", "localPath"} {
		if value, ok := req.Params[field].(string); ok && value != "" {
			summary[field] = value
		}
	}
	return summary
}
