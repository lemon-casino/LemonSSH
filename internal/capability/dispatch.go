package capability

import (
	"context"
	"errors"
	"fmt"
)

// Typed dispatch failure codes. UNKNOWN_CAPABILITY implements the W05
// default-deny: methods outside the catalog never reach a handler.
const (
	CodeUnknownCapability        = "UNKNOWN_CAPABILITY"
	CodeCapabilityNotImplemented = "CAPABILITY_NOT_IMPLEMENTED"
	CodeHandlerMissing           = "HANDLER_MISSING"
	CodePolicyDenied             = "POLICY_DENIED"
	CodeUserDenied               = "USER_DENIED"
	CodeGrantRevoked             = "GRANT_REVOKED"
	CodeApprovalGateUnavailable  = "APPROVAL_GATE_UNAVAILABLE"
)

// DispatchError carries a stable code plus the human message.
type DispatchError struct {
	Code    string
	Message string
}

func (e *DispatchError) Error() string { return e.Code + ": " + e.Message }

// ErrorCode exposes the stable code to error mappers without a type
// dependency between packages.
func (e *DispatchError) ErrorCode() string { return e.Code }

// Handler executes one resolved capability on behalf of a dispatcher.
type Handler func(ctx context.Context, params map[string]any, def *Definition) (any, error)

// ApprovalGate clears one approval requirement. Implementations must block
// until the user decides or ctx is done; false without error means denied.
type ApprovalGate interface {
	RequestApproval(ctx context.Context, req Request, def *Definition) (bool, error)
}

// Dispatcher executes catalog methods on one surface. Handlers are injected
// per capability ID; renderer-local capabilities (harness) simply stay
// unregistered and fail closed with HANDLER_MISSING.
type Dispatcher struct {
	Registry *Registry
	Surface  Surface
	Handlers map[string]Handler
	// Grants returns the current grant list; nil disables grant matching.
	Grants func() []Grant
	// Approval clears confirm-mode prompts; nil fails closed when an
	// approval is required.
	Approval ApprovalGate
	// ChatCancelled reports the owning chat session's current cancel state.
	// It is sampled at entry and again after approval so a Stop that lands
	// while the prompt is open still refuses the write. nil means never
	// cancelled.
	ChatCancelled func() bool
}

// Dispatch resolves, authorizes and executes one request. The approval
// re-check after RequestApproval is the revocation/stop barrier: grants or
// chat state that changed while the prompt was open win over the stale
// approval, so no write starts on revoked authorization.
func (d *Dispatcher) Dispatch(ctx context.Context, rpcMethod string, params map[string]any) (any, error) {
	if d.Registry == nil {
		d.Registry = Default()
	}
	surface := d.Surface
	if surface == "" {
		surface = SurfaceBuiltin
	}

	def := d.Registry.GetByRPCMethod(rpcMethod, surface)
	if def == nil {
		return nil, &DispatchError{Code: CodeUnknownCapability, Message: fmt.Sprintf("unknown capability method %q", rpcMethod)}
	}
	if def.Status != StatusImplemented {
		return nil, &DispatchError{
			Code:    CodeCapabilityNotImplemented,
			Message: fmt.Sprintf("capability %q is not implemented yet", def.ID),
		}
	}
	handler := d.Handlers[def.ID]
	if handler == nil {
		return nil, &DispatchError{
			Code:    CodeHandlerMissing,
			Message: fmt.Sprintf("capability %q has no host handler registered", def.ID),
		}
	}

	req := Request{
		RPCMethod:            rpcMethod,
		Surface:              surface,
		Params:               params,
		ChatSessionCancelled: d.ChatCancelled != nil && d.ChatCancelled(),
	}
	base := d.Registry.Evaluate(req)
	if !base.Allowed {
		return nil, &DispatchError{Code: CodePolicyDenied, Message: base.Error}
	}

	decision := d.evaluate(req)
	grantAuthorized := decision.MatchedGrantID != ""

	if base.RequiresApproval && !grantAuthorized {
		if d.Approval == nil {
			return nil, &DispatchError{
				Code:    CodeApprovalGateUnavailable,
				Message: fmt.Sprintf("capability %q requires approval but no approval gate is configured", def.ID),
			}
		}
		approved, err := d.Approval.RequestApproval(ctx, req, def)
		if err != nil || !approved {
			// Approval errors (gate failure, cancelled prompt) fail closed
			// exactly like an explicit user denial.
			return nil, &DispatchError{Code: CodeUserDenied, Message: UserDeniedMessage}
		}
	}

	if base.RequiresApproval {
		// Re-run policy against fresh grant/chat state before executing so
		// revocations and Stops that landed during authorization win over
		// the stale decision. A prompt-approved request only fails on new
		// policy denials; a grant-authorized request that no longer matches
		// any grant is treated as revoked.
		req.ChatSessionCancelled = d.ChatCancelled != nil && d.ChatCancelled()
		recheck := d.evaluate(req)
		if !recheck.Allowed {
			return nil, &DispatchError{Code: CodePolicyDenied, Message: recheck.Error}
		}
		if grantAuthorized && recheck.RequiresApproval {
			return nil, &DispatchError{
				Code:    CodeGrantRevoked,
				Message: fmt.Sprintf("grant for capability %q was revoked during authorization", def.ID),
			}
		}
	}

	return handler(ctx, params, def)
}

func (d *Dispatcher) evaluate(req Request) Decision {
	if d.Grants == nil {
		return d.Registry.Evaluate(req)
	}
	return d.Registry.EvaluateWithGrants(req, d.Grants())
}

// ErrDispatchCode extracts the stable code from a Dispatch error, or "".
func ErrDispatchCode(err error) string {
	var dispatchErr *DispatchError
	if errors.As(err, &dispatchErr) {
		return dispatchErr.Code
	}
	return ""
}
