package providers

import (
	"encoding/json"
	"fmt"
	"strings"
)

const googleProviderName = "google"

// GoogleStream assembles Gemini GenerateContent SSE chunks into unified
// events. Function calls arrive whole (no argument deltas), so the stream
// emits a ToolCallStart followed by one ToolCallDelta carrying the complete
// marshaled arguments, keeping downstream assembly uniform across families.
type GoogleStream struct {
	usage        Usage
	usageSeen    bool
	usageEmitted bool
	nextIndex    int
	finishSeen   bool
}

func NewGoogleStream() *GoogleStream { return &GoogleStream{} }

type googleChunk struct {
	Candidates []struct {
		Content *struct {
			Parts []struct {
				Text             string `json:"text"`
				Thought          bool   `json:"thought"`
				ThoughtSignature string `json:"thoughtSignature"`
				FunctionCall     *struct {
					Name string          `json:"name"`
					Args json.RawMessage `json:"args"`
				} `json:"functionCall"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason *string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount     int64 `json:"promptTokenCount"`
		CandidatesTokenCount int64 `json:"candidatesTokenCount"`
		CachedContentCount   int64 `json:"cachedContentTokenCount"`
		ThoughtsTokenCount   int64 `json:"thoughtsTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// Consume decodes one SSE data payload. Usage metadata is cumulative per
// the Gemini protocol: the stream overwrites its observation and emits at
// most one usage event per turn, so totals are never double-counted (T22).
func (s *GoogleStream) Consume(ev SSEEvent) ([]Event, error) {
	data := strings.TrimSpace(ev.Data)
	if data == "" {
		return nil, nil
	}

	var chunk googleChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return nil, &StreamError{Provider: googleProviderName, Message: fmt.Sprintf("malformed chunk: %v", err)}
	}
	if chunk.Error != nil {
		code := chunk.Error.Status
		if code == "" {
			code = fmt.Sprint(chunk.Error.Code)
		}
		return nil, &StreamError{Provider: googleProviderName, Code: code, Message: chunk.Error.Message}
	}

	// Apply usage before candidates so a chunk carrying both finishReason
	// and updated counts emits the final observation, not the previous one.
	if chunk.UsageMetadata != nil {
		s.usage = Usage{
			InputTokens:     chunk.UsageMetadata.PromptTokenCount,
			OutputTokens:    chunk.UsageMetadata.CandidatesTokenCount,
			CacheReadTokens: chunk.UsageMetadata.CachedContentCount,
			ReasoningTokens: chunk.UsageMetadata.ThoughtsTokenCount,
			Known:           true,
		}
		s.usageSeen = true
	}

	var events []Event
	for _, candidate := range chunk.Candidates {
		if candidate.Content != nil {
			for _, part := range candidate.Content.Parts {
				events = append(events, s.partEvents(part)...)
			}
		}
		if candidate.FinishReason != nil {
			s.finishSeen = true
			events = append(events, Event{Kind: EventFinish, Finish: mapGoogleFinish(*candidate.FinishReason)})
			events = append(events, s.pendingUsage()...)
		}
	}

	if s.finishSeen && s.usageSeen && !s.usageEmitted {
		events = append(events, s.pendingUsage()...)
	}

	return events, nil
}

func (s *GoogleStream) partEvents(part struct {
	Text             string `json:"text"`
	Thought          bool   `json:"thought"`
	ThoughtSignature string `json:"thoughtSignature"`
	FunctionCall     *struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	} `json:"functionCall"`
}) []Event {
	var events []Event
	if part.Text != "" {
		if part.Thought {
			events = append(events, Event{Kind: EventReasoningDelta, Text: part.Text})
		} else {
			events = append(events, Event{Kind: EventTextDelta, Text: part.Text})
		}
	}
	if part.ThoughtSignature != "" {
		record, _ := json.Marshal(map[string]string{"type": "thought_signature", "signature": part.ThoughtSignature})
		events = append(events, Event{Kind: EventPrivateRecord, ProviderPrivate: record})
	}
	if part.FunctionCall != nil {
		index := s.nextIndex
		s.nextIndex++
		args := part.FunctionCall.Args
		if args == nil {
			args = json.RawMessage("{}")
		}
		events = append(events,
			Event{Kind: EventToolCallStart, ToolIndex: index, ToolID: fmt.Sprintf("google-%d", index), ToolName: part.FunctionCall.Name},
			Event{Kind: EventToolCallDelta, ToolIndex: index, ArgumentsDelta: string(args)},
		)
	}
	return events
}

func (s *GoogleStream) pendingUsage() []Event {
	if !s.usageSeen || s.usageEmitted {
		return nil
	}
	s.usageEmitted = true
	return []Event{{Kind: EventUsage, Usage: s.usage}}
}

func mapGoogleFinish(reason string) FinishReason {
	switch reason {
	case "STOP":
		return FinishStop
	case "MAX_TOKENS":
		return FinishLength
	case "SAFETY", "RECITATION", "PROHIBITED_CONTENT", "BLOCKLIST":
		return FinishContentFilter
	case "MALFORMED_FUNCTION_CALL":
		return FinishUnknown
	default:
		return FinishUnknown
	}
}
