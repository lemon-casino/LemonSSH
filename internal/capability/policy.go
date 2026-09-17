package capability

// User-facing decision strings; kept byte-identical to electron/capabilities
// policy.cjs so both runtimes surface the same messages.
const (
	ObserverDenyMessage        = "Operation denied: permission mode is \"observer\" (read-only). Change to \"confirm\" or \"auto\" in Settings → AI → Safety to allow this action."
	ChatSessionRequiredMessage = "chatSessionId is required for write operations."
	ChatSessionCancelledMsg    = "Operation cancelled: the SDK agent session was stopped."
	UserDeniedMessage          = "Operation denied by user."
)

// Request is one policy evaluation input. Zero-value Surface and
// PermissionMode fall back to the CJS defaults (builtin / confirm).
type Request struct {
	RPCMethod            string
	Surface              Surface
	PermissionMode       PermissionMode
	Params               map[string]any
	ChatSessionCancelled bool
}

// Decision mirrors the CJS RpcPermissionDecision. MatchedGrantID is set when
// a stored grant satisfied the approval requirement.
type Decision struct {
	Allowed          bool
	RequiresApproval bool
	Error            string
	Capability       *Definition
	MatchedGrantID   string
}

func confirmFlag(binding SurfaceBinding) (present, value bool) {
	if binding.ConfirmInConfirmMode == nil {
		return false, false
	}
	return true, *binding.ConfirmInConfirmMode
}

// RequiresApprovalInConfirmMode reports whether the capability must clear an
// approval prompt before executing in confirm mode. The sensitiveRead branch
// is exhaustive-false today (an explicit true binding returns earlier) but is
// kept as the documented rule table for policy audits.
func RequiresApprovalInConfirmMode(def *Definition, surface Surface) bool {
	if def == nil {
		return false
	}
	binding := def.Surfaces[surface]
	if present, value := confirmFlag(binding); present && value {
		return true
	}
	if def.Policy.BypassesApproval {
		return false
	}
	if def.Policy.Write {
		return true
	}
	if def.Policy.SensitiveRead {
		present, value := confirmFlag(binding)
		if !present || value {
			return present && value
		}
	}
	return false
}

// IsBlockedInObserverMode reports whether observer mode refuses the
// capability outright.
func IsBlockedInObserverMode(def *Definition) bool {
	if def == nil {
		return false
	}
	if def.Policy.BypassesObserverBlock {
		return false
	}
	return def.Policy.Write
}

// paramString extracts a non-empty string parameter; anything else counts as
// absent, matching the falsy check in policy.cjs.
func paramString(params map[string]any, key string) string {
	if s, ok := params[key].(string); ok {
		return s
	}
	return ""
}

// Evaluate resolves the policy decision for one request on the default
// registry's catalog. Unknown methods are allowed here with a nil capability
// — matching policy.cjs — because default-deny for unknown capabilities is
// enforced by Dispatch, which is the only execution path.
func (r *Registry) Evaluate(req Request) Decision {
	surface := req.Surface
	if surface == "" {
		surface = SurfaceBuiltin
	}
	mode := req.PermissionMode
	if mode == "" {
		mode = ModeConfirm
	}

	def := r.GetByRPCMethod(req.RPCMethod, surface)
	if def == nil {
		return Decision{Allowed: true}
	}

	if def.Policy.Write && paramString(req.Params, "chatSessionId") == "" && surface == SurfaceBuiltin {
		return Decision{Error: ChatSessionRequiredMessage, Capability: def}
	}

	if def.Policy.Write && !def.Policy.BypassesChatCancel && req.ChatSessionCancelled && surface == SurfaceBuiltin {
		return Decision{Error: ChatSessionCancelledMsg, Capability: def}
	}

	if mode == ModeObserver && IsBlockedInObserverMode(def) {
		return Decision{Error: ObserverDenyMessage, Capability: def}
	}

	requiresApproval := mode == ModeConfirm && RequiresApprovalInConfirmMode(def, surface)
	return Decision{Allowed: true, RequiresApproval: requiresApproval, Capability: def}
}

// EvaluateWithGrants applies stored permission grants on top of Evaluate: a
// matching grant clears RequiresApproval and records its ID.
func (r *Registry) EvaluateWithGrants(req Request, grants []Grant) Decision {
	base := r.Evaluate(req)
	if !base.Allowed || !base.RequiresApproval || base.Capability == nil {
		return base
	}
	matched := MatchPermissionGrant(grants, MatchContext{
		CapabilityID: base.Capability.ID,
		Args:         req.Params,
	})
	if matched != nil {
		base.RequiresApproval = false
		base.MatchedGrantID = matched.ID
	}
	return base
}

// WriteRPCMethods lists implemented write methods bound on one surface.
func (r *Registry) WriteRPCMethods(surface Surface) map[string]bool {
	return methodSet(r, surface, func(def *Definition) bool { return def.Policy.Write })
}

// ApprovalRPCMethods lists implemented methods that demand approval in
// confirm mode on one surface.
func (r *Registry) ApprovalRPCMethods(surface Surface) map[string]bool {
	return methodSet(r, surface, func(def *Definition) bool {
		return RequiresApprovalInConfirmMode(def, surface)
	})
}

func methodSet(r *Registry, surface Surface, predicate func(*Definition) bool) map[string]bool {
	out := make(map[string]bool)
	for _, method := range r.RPCMethodsForSurface(surface, RPCFilter{Status: StatusImplemented}) {
		if def := r.GetByRPCMethod(method, surface); predicate(def) {
			out[method] = true
		}
	}
	return out
}
