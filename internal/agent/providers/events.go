package providers

import "encoding/json"

// FinishReason normalizes the terminal reasons across families. The zero
// value "" means not finished; FinishUnknown preserves an unrecognized
// upstream reason instead of guessing.
type FinishReason string

const (
	FinishStop          FinishReason = "stop"
	FinishLength        FinishReason = "length"
	FinishToolCall      FinishReason = "tool_call"
	FinishContentFilter FinishReason = "content_filter"
	FinishUnknown       FinishReason = "unknown"
)

// Usage is one usage observation. Known distinguishes "provider reported
// usage" from "no usage arrived": unknown usage is preserved as unknown,
// never filled with zeros (T22).
type Usage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	Known            bool
}

// EventKind enumerates the unified provider events.
type EventKind string

const (
	EventTextDelta      EventKind = "text_delta"
	EventReasoningDelta EventKind = "reasoning_delta"
	EventToolCallStart  EventKind = "tool_call_start"
	EventToolCallDelta  EventKind = "tool_call_delta"
	EventFinish         EventKind = "finish"
	EventUsage          EventKind = "usage"
	EventPrivateRecord  EventKind = "private_record"
)

// Event is one unified provider event. ProviderPrivate carries opaque
// continuation records (thinking signatures, encrypted reasoning) that the
// next request of the SAME family consumes; it is never rendered to the UI
// and never sent to a different family (T20/T21).
type Event struct {
	Kind            EventKind
	Text            string
	ToolIndex       int
	ToolID          string
	ToolName        string
	ArgumentsDelta  string
	Finish          FinishReason
	Usage           Usage
	ProviderPrivate json.RawMessage
}

// StreamError is a provider-side failure surfaced mid-stream.
type StreamError struct {
	Provider string
	Code     string
	Message  string
}

func (e *StreamError) Error() string {
	if e.Code != "" {
		return e.Provider + " stream error " + e.Code + ": " + e.Message
	}
	return e.Provider + " stream error: " + e.Message
}
