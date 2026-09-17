package providers

import (
	"encoding/json"
	"fmt"
	"strings"
)

const anthropicProviderName = "anthropic"

// AnthropicStream assembles Anthropic Messages SSE events into unified
// events. Thinking blocks with signatures are surfaced as
// ProviderPrivate continuation records: the next same-family request
// consumes them verbatim; they never reach the UI and never cross to
// another family (T20).
type AnthropicStream struct {
	signatures map[int]*strings.Builder
	usage      Usage
	finishSeen bool
}

func NewAnthropicStream() *AnthropicStream {
	return &AnthropicStream{signatures: map[int]*strings.Builder{}}
}

type anthropicEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
	Message *struct {
		ID    string `json:"id"`
		Usage struct {
			InputTokens              int64 `json:"input_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
	ContentBlock *struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		Name      string `json:"name"`
		Data      string `json:"data"`
		Signature string `json:"signature"`
	} `json:"content_block"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		Signature   string `json:"signature"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Usage *struct {
		OutputTokens int64 `json:"output_tokens"`
	} `json:"usage"`
}

// Consume decodes one SSE event. The event name field (event: ...) is
// advisory; the data payload's type field drives dispatch, matching the
// official SDK behavior.
func (s *AnthropicStream) Consume(ev SSEEvent) ([]Event, error) {
	data := strings.TrimSpace(ev.Data)
	if data == "" {
		return nil, nil
	}

	var payload anthropicEvent
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return nil, &StreamError{Provider: anthropicProviderName, Message: fmt.Sprintf("malformed event: %v", err)}
	}

	switch payload.Type {
	case "ping":
		return nil, nil
	case "error":
		if payload.Error != nil {
			return nil, &StreamError{Provider: anthropicProviderName, Code: payload.Error.Type, Message: payload.Error.Message}
		}
		return nil, &StreamError{Provider: anthropicProviderName, Message: "unknown stream error"}
	case "message_start":
		// Input-side usage arrives here; output arrives at message_delta.
		// The stream merges both so one turn yields exactly one usage
		// observation (T22).
		if payload.Message != nil {
			s.usage.InputTokens = payload.Message.Usage.InputTokens
			s.usage.CacheReadTokens = payload.Message.Usage.CacheReadInputTokens
			s.usage.CacheWriteTokens = payload.Message.Usage.CacheCreationInputTokens
			s.usage.Known = true
		}
		return nil, nil
	case "content_block_start":
		return s.blockStart(payload)
	case "content_block_delta":
		return s.blockDelta(payload)
	case "content_block_stop":
		return s.blockStop(payload)
	case "message_delta":
		var events []Event
		if payload.Usage != nil && !s.finishSeen {
			s.usage.OutputTokens = payload.Usage.OutputTokens
			events = append(events, Event{Kind: EventUsage, Usage: s.usage})
		}
		if payload.Delta != nil && payload.Delta.StopReason != "" {
			s.finishSeen = true
			events = append(events, Event{Kind: EventFinish, Finish: mapAnthropicFinish(payload.Delta.StopReason)})
		}
		return events, nil
	case "message_stop":
		return nil, nil
	default:
		return nil, nil
	}
}

func (s *AnthropicStream) blockStart(payload anthropicEvent) ([]Event, error) {
	block := payload.ContentBlock
	if block == nil {
		return nil, nil
	}
	switch block.Type {
	case "tool_use":
		return []Event{{
			Kind:      EventToolCallStart,
			ToolIndex: payload.Index,
			ToolID:    block.ID,
			ToolName:  block.Name,
		}}, nil
	case "redacted_thinking":
		record, _ := json.Marshal(map[string]string{
			"type": "redacted_thinking", "data": block.Data,
		})
		return []Event{{Kind: EventPrivateRecord, ProviderPrivate: record}}, nil
	default:
		return nil, nil
	}
}

func (s *AnthropicStream) blockDelta(payload anthropicEvent) ([]Event, error) {
	delta := payload.Delta
	if delta == nil {
		return nil, nil
	}
	switch delta.Type {
	case "text_delta":
		if delta.Text == "" {
			return nil, nil
		}
		return []Event{{Kind: EventTextDelta, Text: delta.Text}}, nil
	case "thinking_delta":
		if delta.Thinking == "" {
			return nil, nil
		}
		return []Event{{Kind: EventReasoningDelta, Text: delta.Thinking}}, nil
	case "signature_delta":
		builder := s.signatures[payload.Index]
		if builder == nil {
			builder = &strings.Builder{}
			s.signatures[payload.Index] = builder
		}
		builder.WriteString(delta.Signature)
		return nil, nil
	case "input_json_delta":
		if delta.PartialJSON == "" {
			return nil, nil
		}
		return []Event{{
			Kind:           EventToolCallDelta,
			ToolIndex:      payload.Index,
			ArgumentsDelta: delta.PartialJSON,
		}}, nil
	default:
		return nil, nil
	}
}

// blockStop emits the thinking-block continuation record when a signature
// was assembled for the block.
func (s *AnthropicStream) blockStop(payload anthropicEvent) ([]Event, error) {
	builder, ok := s.signatures[payload.Index]
	if !ok {
		return nil, nil
	}
	delete(s.signatures, payload.Index)
	record, _ := json.Marshal(map[string]string{
		"type": "thinking", "index": fmt.Sprint(payload.Index), "signature": builder.String(),
	})
	return []Event{{Kind: EventPrivateRecord, ProviderPrivate: record}}, nil
}

func mapAnthropicFinish(reason string) FinishReason {
	switch reason {
	case "end_turn", "stop_sequence":
		return FinishStop
	case "max_tokens":
		return FinishLength
	case "tool_use":
		return FinishToolCall
	case "refusal":
		return FinishContentFilter
	default:
		return FinishUnknown
	}
}
