package contracts

import "github.com/binaricat/netcatty/internal/app"

// AllErrorCodes lists every stable error code. The contracts test verifies it
// stays in sync with the constants in errors.go, and the contracts codegen
// emits the TypeScript union from this list.
func AllErrorCodes() []ErrorCode {
	return []ErrorCode{
		CodeUnknown,
		CodeInvalidRequest,
		CodeNotFound,
		CodeConflict,
		CodeDeadlineExceeded,
		CodeCancelled,
		CodeUnavailable,
		CodeInternal,
	}
}

// WireTypes lists every base contract type crossing a shell boundary. The
// contracts codegen emits TypeScript declarations for exactly these types;
// adding a type here without regenerating fails the drift check.
func WireTypes() []any {
	return []any{
		Error{},
		Request{},
		SubscriptionOpen{},
		SubscriptionEvent{},
		app.HealthStatus{},
		app.VersionInfo{},
		app.WindowRoleInfo{},
	}
}
