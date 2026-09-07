package contracts

import (
	"context"
	"errors"
	"fmt"
)

// ErrorCode is a stable, wire-visible error identity. Values are part of the
// cross-shell contract: never rename or reuse a code, only add new ones.
type ErrorCode string

const (
	// CodeUnknown covers errors that carry no more specific code.
	CodeUnknown ErrorCode = "netcatty.unknown"
	// CodeInvalidRequest marks a malformed or rejected request payload.
	CodeInvalidRequest ErrorCode = "netcatty.invalid_request"
	// CodeNotFound marks a missing resource.
	CodeNotFound ErrorCode = "netcatty.not_found"
	// CodeConflict marks a state conflict such as duplicate ownership.
	CodeConflict ErrorCode = "netcatty.conflict"
	// CodeDeadlineExceeded marks a request whose deadline elapsed.
	CodeDeadlineExceeded ErrorCode = "netcatty.deadline_exceeded"
	// CodeCancelled marks a request cancelled by its owner.
	CodeCancelled ErrorCode = "netcatty.cancelled"
	// CodeUnavailable marks a temporarily unavailable backend or shell.
	CodeUnavailable ErrorCode = "netcatty.unavailable"
	// CodeInternal marks an unexpected internal failure.
	CodeInternal ErrorCode = "netcatty.internal"
)

// Error is the structured error envelope crossing shell boundaries.
type Error struct {
	Code      ErrorCode         `json:"code"`
	Message   string            `json:"message"`
	Details   map[string]string `json:"details,omitempty"`
	Retryable bool              `json:"retryable,omitempty"`
}

// Error implements the error interface with a stable prefix.
func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewError builds an Error envelope.
func NewError(code ErrorCode, message string) *Error {
	return &Error{Code: code, Message: message}
}

// WithDetail attaches one detail entry and returns the same envelope.
func (e *Error) WithDetail(key, value string) *Error {
	if e.Details == nil {
		e.Details = map[string]string{}
	}
	e.Details[key] = value
	return e
}

// AsError converts any error into an Error envelope. Well-known sentinel
// errors map to their stable codes; everything else maps to CodeInternal so
// no unmapped error can cross a boundary without an explicit code.
func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var envelope *Error
	if errors.As(err, &envelope) {
		return envelope
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return &Error{Code: CodeDeadlineExceeded, Message: err.Error(), Retryable: true}
	case errors.Is(err, context.Canceled):
		return &Error{Code: CodeCancelled, Message: err.Error()}
	case errors.Is(err, ErrPayloadTooLarge), errors.Is(err, ErrDepthExceeded),
		errors.Is(err, ErrUnsafeInteger), errors.Is(err, ErrUnknownField):
		return &Error{Code: CodeInvalidRequest, Message: err.Error()}
	default:
		return &Error{Code: CodeInternal, Message: err.Error()}
	}
}
