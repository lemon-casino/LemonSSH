package providers

import (
	"encoding/json"
	"fmt"
	"strings"
)

const openAIProviderName = "openai"

// OpenAIChatStream assembles OpenAI Chat Completions SSE chunks into
// unified events. Tool call fragments may interleave across indices and
// arrive in any chunk order; each index keeps its own start/delta state
// (T18).
type OpenAIChatStream struct {
	started   map[int]bool
	idsByIndx map[int]string
	sawFinish bool
	sawDone   bool
	sawUsage  bool
}

func NewOpenAIChatStream() *OpenAIChatStream {
	return &OpenAIChatStream{started: map[int]bool{}, idsByIndx: map[int]string{}}
}

type openAIChatChunk struct {
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens        int64 `json:"prompt_tokens"`
		CompletionTokens    int64 `json:"completion_tokens"`
		PromptTokensDetails *struct {
			CachedTokens int64 `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		CompletionTokensDetails *struct {
			ReasoningTokens int64 `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
	Error *struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Consume decodes one SSE event into zero or more unified events. The
// "data: [DONE]" terminator is swallowed (it carries no semantics beyond
// stream end); malformed JSON and provider error payloads surface as
// StreamError instead of being dropped.
func (s *OpenAIChatStream) Consume(ev SSEEvent) ([]Event, error) {
	data := strings.TrimSpace(ev.Data)
	if data == "" {
		return nil, nil
	}
	if data == "[DONE]" {
		s.sawDone = true
		return nil, nil
	}

	var chunk openAIChatChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return nil, &StreamError{Provider: openAIProviderName, Message: fmt.Sprintf("malformed chunk: %v", err)}
	}
	if chunk.Error != nil {
		code := ""
		if chunk.Error.Code != nil {
			code = fmt.Sprint(chunk.Error.Code)
		}
		return nil, &StreamError{Provider: openAIProviderName, Code: code, Message: chunk.Error.Message}
	}

	var events []Event
	for _, choice := range chunk.Choices {
		delta := choice.Delta
		if delta.Content != "" {
			events = append(events, Event{Kind: EventTextDelta, Text: delta.Content})
		}
		if delta.ReasoningContent != "" {
			events = append(events, Event{Kind: EventReasoningDelta, Text: delta.ReasoningContent})
		}
		for _, fragment := range delta.ToolCalls {
			events = append(events, s.toolCallEvents(fragment)...)
		}
		if choice.FinishReason != nil {
			s.sawFinish = true
			events = append(events, Event{Kind: EventFinish, Finish: mapOpenAIFinish(*choice.FinishReason)})
		}
	}

	if chunk.Usage != nil && !s.sawUsage {
		s.sawUsage = true
		usage := Usage{
			InputTokens:  chunk.Usage.PromptTokens,
			OutputTokens: chunk.Usage.CompletionTokens,
			Known:        true,
		}
		if chunk.Usage.PromptTokensDetails != nil {
			usage.CacheReadTokens = chunk.Usage.PromptTokensDetails.CachedTokens
		}
		if chunk.Usage.CompletionTokensDetails != nil {
			usage.ReasoningTokens = chunk.Usage.CompletionTokensDetails.ReasoningTokens
		}
		events = append(events, Event{Kind: EventUsage, Usage: usage})
	}

	return events, nil
}

func (s *OpenAIChatStream) toolCallEvents(fragment struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}) []Event {
	var events []Event
	if !s.started[fragment.Index] {
		s.started[fragment.Index] = true
		s.idsByIndx[fragment.Index] = fragment.ID
		events = append(events, Event{
			Kind:      EventToolCallStart,
			ToolIndex: fragment.Index,
			ToolID:    fragment.ID,
			ToolName:  fragment.Function.Name,
		})
		// A start frame may already carry arguments (OpenAI single-shot
		// shape): surface them as the first delta so the accumulator sees
		// the complete stream.
		if fragment.Function.Arguments != "" {
			events = append(events, Event{
				Kind:           EventToolCallDelta,
				ToolIndex:      fragment.Index,
				ToolID:         fragment.ID,
				ArgumentsDelta: fragment.Function.Arguments,
			})
		}
		return events
	}
	if fragment.Function.Arguments != "" || fragment.ID != "" || fragment.Function.Name != "" {
		// Deltas carry only the index; the id recorded at start is filled
		// in so downstream consumers can correlate every event.
		events = append(events, Event{
			Kind:           EventToolCallDelta,
			ToolIndex:      fragment.Index,
			ToolID:         s.idsByIndx[fragment.Index],
			ToolName:       fragment.Function.Name,
			ArgumentsDelta: fragment.Function.Arguments,
		})
	}
	return events
}

func mapOpenAIFinish(reason string) FinishReason {
	switch reason {
	case "stop":
		return FinishStop
	case "length":
		return FinishLength
	case "tool_calls", "function_call":
		return FinishToolCall
	case "content_filter":
		return FinishContentFilter
	default:
		return FinishUnknown
	}
}
