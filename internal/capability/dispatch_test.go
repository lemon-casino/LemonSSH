package capability

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubApproval struct {
	approved bool
	err      error
	calls    int
}

func (s *stubApproval) RequestApproval(ctx context.Context, req Request, def *Definition) (bool, error) {
	s.calls++
	return s.approved, s.err
}

type mutatingGrants struct {
	grants    []Grant
	afterCall func()
}

func (m *mutatingGrants) Grants() []Grant {
	current := m.grants
	if m.afterCall != nil {
		m.afterCall()
		m.afterCall = nil
	}
	return current
}

func newTestDispatcher(registry *Registry, approval ApprovalGate, grants func() []Grant) *Dispatcher {
	return &Dispatcher{
		Registry: registry,
		Surface:  SurfaceBuiltin,
		Handlers: map[string]Handler{
			"terminal.execute": func(ctx context.Context, params map[string]any, def *Definition) (any, error) {
				return "executed", nil
			},
			"meta.status": func(ctx context.Context, params map[string]any, def *Definition) (any, error) {
				return "status", nil
			},
		},
		Grants:   grants,
		Approval: approval,
	}
}

func TestDispatchUnknownMethodDenied(t *testing.T) {
	dispatcher := newTestDispatcher(Default(), nil, nil)
	_, err := dispatcher.Dispatch(context.Background(), "auth/verify", map[string]any{})
	if ErrDispatchCode(err) != CodeUnknownCapability {
		t.Fatalf("unknown method must fail with UNKNOWN_CAPABILITY, got %v", err)
	}
}

func TestDispatchMissingHandlerFailsClosed(t *testing.T) {
	dispatcher := newTestDispatcher(Default(), nil, nil)
	delete(dispatcher.Handlers, "meta.status")
	_, err := dispatcher.Dispatch(context.Background(), "netcatty/getStatus", map[string]any{})
	if ErrDispatchCode(err) != CodeHandlerMissing {
		t.Fatalf("unregistered capability must fail with HANDLER_MISSING, got %v", err)
	}
}

func TestDispatchPlannedCapabilityNotImplemented(t *testing.T) {
	registry := NewRegistry([]Definition{
		{
			ID: "demo.planned", Domain: "demo", Status: StatusPlanned,
			Policy:   policy(true, false, false, false, false, false, false),
			Surfaces: map[Surface]SurfaceBinding{SurfaceBuiltin: {RPCMethod: "demo/planned"}},
		},
	})
	dispatcher := newTestDispatcher(registry, nil, nil)
	_, err := dispatcher.Dispatch(context.Background(), "demo/planned", map[string]any{})
	if ErrDispatchCode(err) != CodeCapabilityNotImplemented {
		t.Fatalf("planned capability must fail with CAPABILITY_NOT_IMPLEMENTED, got %v", err)
	}
}

func TestDispatchObserverDenied(t *testing.T) {
	dispatcher := newTestDispatcher(Default(), nil, nil)
	_, err := dispatcher.Dispatch(context.Background(), "netcatty/exec", map[string]any{"chatSessionId": "chat-1"})
	// confirm mode with no approval gate configured fails closed.
	if ErrDispatchCode(err) != CodeApprovalGateUnavailable {
		t.Fatalf("confirm write without gate must fail closed, got %v", err)
	}

	observer := dispatcher
	observer.Approval = &stubApproval{approved: false}
	req := Request{RPCMethod: "netcatty/exec", PermissionMode: ModeObserver, Params: map[string]any{"chatSessionId": "chat-1"}}
	if decision := Default().Evaluate(req); decision.Allowed {
		t.Fatalf("observer must deny exec at policy layer")
	}
}

func TestDispatchApprovalApprovedThenExecutes(t *testing.T) {
	approval := &stubApproval{approved: true}
	dispatcher := newTestDispatcher(Default(), approval, nil)
	result, err := dispatcher.Dispatch(context.Background(), "netcatty/exec", map[string]any{"chatSessionId": "chat-1"})
	if err != nil || result != "executed" {
		t.Fatalf("approved exec must run, got %v, %v", result, err)
	}
	if approval.calls != 1 {
		t.Errorf("approval gate must be called once, got %d", approval.calls)
	}
}

func TestDispatchApprovalDenied(t *testing.T) {
	approval := &stubApproval{approved: false}
	dispatcher := newTestDispatcher(Default(), approval, nil)
	_, err := dispatcher.Dispatch(context.Background(), "netcatty/exec", map[string]any{"chatSessionId": "chat-1"})
	if ErrDispatchCode(err) != CodeUserDenied {
		t.Fatalf("denied exec must fail with USER_DENIED, got %v", err)
	}
}

func TestDispatchApprovalGateErrorFailsClosed(t *testing.T) {
	approval := &stubApproval{err: errors.New("gate exploded")}
	dispatcher := newTestDispatcher(Default(), approval, nil)
	_, err := dispatcher.Dispatch(context.Background(), "netcatty/exec", map[string]any{"chatSessionId": "chat-1"})
	if ErrDispatchCode(err) != CodeUserDenied {
		t.Fatalf("gate error must fail closed like a denial, got %v", err)
	}
}

func TestDispatchGrantSkipsApproval(t *testing.T) {
	approval := &stubApproval{}
	grants := []Grant{terminalGrant("grant-1", "ls *")}
	dispatcher := newTestDispatcher(Default(), approval, func() []Grant { return grants })
	result, err := dispatcher.Dispatch(context.Background(), "netcatty/exec", map[string]any{
		"chatSessionId": "chat-1", "sessionId": "session-a", "command": "ls -la",
	})
	if err != nil || result != "executed" {
		t.Fatalf("granted exec must run without approval, got %v, %v", result, err)
	}
	if approval.calls != 0 {
		t.Errorf("granted exec must not hit the approval gate, got %d calls", approval.calls)
	}
}

// TestDispatchGrantRevokedDuringApproval covers the revocation barrier: the
// grant list is empty by the time approval returns, so the stale approval
// must not start the write.
func TestDispatchGrantRevokedDuringApproval(t *testing.T) {
	approval := &stubApproval{approved: true}
	store := &mutatingGrants{grants: []Grant{terminalGrant("grant-1", "ls *")}}
	store.afterCall = func() {
		store.grants = nil // revoked while the prompt was open
	}
	dispatcher := newTestDispatcher(Default(), approval, store.Grants)
	_, err := dispatcher.Dispatch(context.Background(), "netcatty/exec", map[string]any{
		"chatSessionId": "chat-1", "sessionId": "session-a", "command": "ls -la",
	})
	if ErrDispatchCode(err) != CodeGrantRevoked {
		t.Fatalf("revoked grant must fail with GRANT_REVOKED, got %v", err)
	}
}

// TestDispatchChatCancelledDuringApproval covers the stop race: the chat
// session's cancel flag flips while the approval prompt is open; the
// re-check must refuse to start the write even though the user approved.
func TestDispatchChatCancelledDuringApproval(t *testing.T) {
	cancelled := false
	approval := ApprovalGateFunc(func(ctx context.Context, req Request, def *Definition) (bool, error) {
		cancelled = true // user hits Stop while the prompt is open
		return true, nil
	})
	dispatcher := newTestDispatcher(Default(), approval, nil)
	dispatcher.ChatCancelled = func() bool { return cancelled }

	_, err := dispatcher.Dispatch(context.Background(), "netcatty/exec", map[string]any{"chatSessionId": "chat-1"})
	if ErrDispatchCode(err) != CodePolicyDenied {
		t.Fatalf("stop during approval must deny at re-check, got %v", err)
	}
}

// TestDispatchCancelBeforeEntryDeniesWithoutApproval verifies the cancel
// flag is honored at entry too, before any prompt is raised.
func TestDispatchCancelBeforeEntryDeniesWithoutApproval(t *testing.T) {
	approval := &stubApproval{}
	dispatcher := newTestDispatcher(Default(), approval, nil)
	dispatcher.ChatCancelled = func() bool { return true }

	_, err := dispatcher.Dispatch(context.Background(), "netcatty/exec", map[string]any{"chatSessionId": "chat-1"})
	if ErrDispatchCode(err) != CodePolicyDenied {
		t.Fatalf("cancelled chat must deny at entry, got %v", err)
	}
	if approval.calls != 0 {
		t.Errorf("cancelled request must not raise a prompt, got %d", approval.calls)
	}
}

// ApprovalGateFunc adapts a function to ApprovalGate.
type ApprovalGateFunc func(ctx context.Context, req Request, def *Definition) (bool, error)

func (f ApprovalGateFunc) RequestApproval(ctx context.Context, req Request, def *Definition) (bool, error) {
	return f(ctx, req, def)
}

func TestResolveRPCTimeouts(t *testing.T) {
	registry := Default()

	cases := []struct {
		name   string
		method string
		mode   PermissionMode
		opts   TimeoutOptions
		want   time.Duration
	}{
		{"short no approval", "netcatty/getStatus", ModeAuto, TimeoutOptions{}, DefaultRPCTimeout},
		{"short with approval wait", "netcatty/getStatus", ModeConfirm, TimeoutOptions{}, DefaultRPCTimeout},
		{"longrunning auto", "netcatty/exec", ModeAuto, TimeoutOptions{}, DefaultOperationTime + RPCTimeoutBuffer},
		{"longrunning confirm approval", "netcatty/exec", ModeConfirm, TimeoutOptions{}, DefaultApprovalTimeout + DefaultOperationTime + RPCTimeoutBuffer},
		{"bridge overrides", "netcatty/exec", ModeConfirm, TimeoutOptions{
			BridgeCommandTimeout:  200 * time.Second,
			BridgeApprovalTimeout: 20 * time.Second,
		}, 200*time.Second + 20*time.Second + RPCTimeoutBuffer},
		// sftp rows carry the long-running default, so they stack both windows.
		{"sftp write confirm approval", "netcatty/sftp/write", ModeConfirm, TimeoutOptions{}, DefaultApprovalTimeout + DefaultOperationTime + RPCTimeoutBuffer},
		{"sftp write observer", "netcatty/sftp/write", ModeObserver, TimeoutOptions{}, DefaultOperationTime + RPCTimeoutBuffer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := tc.opts
			opts.BridgePermissionMode = tc.mode
			if got := registry.ResolveRPCTimeout(tc.method, opts); got != tc.want {
				t.Errorf("ResolveRPCTimeout(%s) = %v, want %v", tc.method, got, tc.want)
			}
		})
	}
}
