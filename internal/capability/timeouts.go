package capability

import "time"

// RPC timeout defaults, mirroring electron/capabilities rpcTimeouts.cjs.
const (
	DefaultRPCTimeout      = 30 * time.Second
	DefaultOperationTime   = 60 * time.Second
	RPCTimeoutBuffer       = 5 * time.Second
	DefaultApprovalTimeout = 110 * time.Second
)

// IsLongRunningMethod reports whether the method's capability is flagged
// long-running on the given surface.
func (r *Registry) IsLongRunningMethod(method string, surface Surface) bool {
	def := r.GetByRPCMethod(method, surface)
	return def != nil && def.Policy.LongRunning
}

// IsApprovalWaitMethod reports whether a call on this surface waits for an
// approval prompt under the given permission mode.
func (r *Registry) IsApprovalWaitMethod(method string, surface Surface, mode PermissionMode) bool {
	if mode == "" {
		mode = ModeConfirm
	}
	if mode != ModeConfirm {
		return false
	}
	return RequiresApprovalInConfirmMode(r.GetByRPCMethod(method, surface), surface)
}

// TimeoutOptions carries the bridge-provided overrides; zero values fall
// back to the defaults above (a CJS null/NaN override behaves as zero).
type TimeoutOptions struct {
	BridgeCommandTimeout   time.Duration
	BridgePermissionMode   PermissionMode
	BridgeApprovalTimeout  time.Duration
	DefaultOperationTime   time.Duration
	DefaultApprovalTimeout time.Duration
	DefaultRPCTimeout      time.Duration
	TimeoutBuffer          time.Duration
}

func (o TimeoutOptions) withDefaults() TimeoutOptions {
	if o.DefaultOperationTime <= 0 {
		o.DefaultOperationTime = DefaultOperationTime
	}
	if o.DefaultApprovalTimeout <= 0 {
		o.DefaultApprovalTimeout = DefaultApprovalTimeout
	}
	if o.DefaultRPCTimeout <= 0 {
		o.DefaultRPCTimeout = DefaultRPCTimeout
	}
	if o.TimeoutBuffer <= 0 {
		o.TimeoutBuffer = RPCTimeoutBuffer
	}
	return o
}

// ResolveRPCTimeout computes the deadline for one method call: the sum of
// the operation and approval windows (when both apply) plus buffer, never
// below the default RPC timeout.
func (r *Registry) ResolveRPCTimeout(method string, opts TimeoutOptions) time.Duration {
	o := opts.withDefaults()

	operationTimeout := time.Duration(0)
	if r.IsLongRunningMethod(method, SurfaceBuiltin) {
		if o.BridgeCommandTimeout > 0 {
			operationTimeout = o.BridgeCommandTimeout
		} else {
			operationTimeout = o.DefaultOperationTime
		}
	}

	approvalTimeout := time.Duration(0)
	if r.IsApprovalWaitMethod(method, SurfaceBuiltin, o.BridgePermissionMode) {
		if o.BridgeApprovalTimeout > 0 {
			approvalTimeout = o.BridgeApprovalTimeout
		} else {
			approvalTimeout = o.DefaultApprovalTimeout
		}
	}

	var total time.Duration
	switch {
	case operationTimeout > 0 && approvalTimeout > 0:
		total = approvalTimeout + operationTimeout + o.TimeoutBuffer
	case operationTimeout > 0:
		total = operationTimeout + o.TimeoutBuffer
	case approvalTimeout > 0:
		total = approvalTimeout + o.TimeoutBuffer
	default:
		total = o.DefaultRPCTimeout
	}
	if total < o.DefaultRPCTimeout {
		return o.DefaultRPCTimeout
	}
	return total
}
