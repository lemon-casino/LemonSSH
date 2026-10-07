package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ToolLoop drives one multi-step OpenAI Chat tool turn: stream a request,
// accumulate text, execute tool calls through the injected executor, feed
// results back and repeat until a final text answer or the iteration cap.
// The HTTP client is the netpolicy-enforced client.
type ToolLoop struct {
	Client        HTTPClient
	Endpoint      string // full chat completions URL
	APIKeyHeader  string // header name, e.g. Authorization
	APIKeyValue   string // header value, e.g. Bearer sk-...
	Model         string
	System        string
	Messages      []ChatMessage
	Tools         []ToolSpec
	MaxIterations int
	// ExecuteTool runs one model-requested tool through the capability
	// dispatcher (policy + approval inside).
	ExecuteTool func(ctx context.Context, toolName string, args json.RawMessage) (json.RawMessage, error)
	// OnTextDelta streams assistant text to the UI.
	OnTextDelta func(text string)
}

// LoopResult is the final outcome of one tool turn.
type LoopResult struct {
	FinalText   string
	Iterations  int
	ToolCalls   int
	FinishKnown bool
}

// Run executes the loop. Every iteration posts the full conversation;
// tool results are appended as tool-role messages keyed by call id.
func (l *ToolLoop) Run(ctx context.Context) (LoopResult, error) {
	maxIterations := l.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 8
	}

	conversation := make([]ChatMessage, 0, len(l.Messages)+8)
	if l.System != "" {
		conversation = append(conversation, ChatMessage{Role: "system", Content: l.System})
	}
	conversation = append(conversation, l.Messages...)

	var result LoopResult
	for iteration := 0; iteration < maxIterations; iteration++ {
		result.Iterations = iteration + 1

		assistantText, calls, finish, err := l.streamOnce(ctx, conversation)
		if err != nil {
			return result, err
		}
		if len(calls) == 0 {
			result.FinalText = assistantText
			result.FinishKnown = finish != ""
			return result, nil
		}
		result.ToolCalls += len(calls)

		// Execute each call and append assistant + tool messages.
		conversation = append(conversation, ChatMessage{
			Role: "assistant", Content: assistantText, ToolCalls: calls,
		})
		for _, call := range calls {
			payload, execErr := l.ExecuteTool(ctx, call.Name, call.Arguments)
			toolMessage := ChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Name:       call.Name,
			}
			if execErr != nil {
				toolMessage.Content = fmt.Sprintf(`{"error":%q}`, execErr.Error())
			} else {
				toolMessage.Content = string(payload)
			}
			conversation = append(conversation, toolMessage)
		}
		if assistantText != "" {
			result.FinalText = assistantText
		}
	}
	result.FinishKnown = true
	return result, nil
}

// streamOnce posts one chat completion request and streams the response,
// returning the assistant text, the completed tool calls (arguments
// merged across chunks) and the finish reason.
func (l *ToolLoop) streamOnce(ctx context.Context, conversation []ChatMessage) (string, []ToolCallRef, string, error) {
	body, err := json.Marshal(map[string]any{
		"model":          l.Model,
		"messages":       conversation,
		"tools":          openAITools(l.Tools),
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	})
	if err != nil {
		return "", nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if l.APIKeyHeader != "" && l.APIKeyValue != "" {
		req.Header.Set(l.APIKeyHeader, l.APIKeyValue)
	}
	resp, err := l.Client.Do(req)
	if err != nil {
		return "", nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return "", nil, "", fmt.Errorf("provider http %d: %s", resp.StatusCode, truncateForLog(string(raw)))
	}

	parser := &SSEParser{}
	stream := NewOpenAIChatStream()
	acc := &toolCallAccumulator{byID: map[string]*strings.Builder{}}

	var assistantText string
	var ordered []ToolCallRef
	var finish string

	buf := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			for _, event := range parser.Feed(buf[:n]) {
				events, consumeErr := stream.Consume(event)
				if consumeErr != nil {
					return "", nil, "", consumeErr
				}
				assistantText, ordered, finish = l.applyEvents(events, assistantText, ordered, finish, acc)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return "", nil, "", readErr
		}
	}

	return assistantText, acc.executed(), finish, nil
}

// applyEvents routes unified events into the loop state.
func (l *ToolLoop) applyEvents(events []Event, assistantText string, toolCalls []ToolCallRef, finish string, acc *toolCallAccumulator) (string, []ToolCallRef, string) {
	for _, event := range events {
		switch event.Kind {
		case EventTextDelta:
			assistantText += event.Text
			if l.OnTextDelta != nil {
				l.OnTextDelta(event.Text)
			}
		case EventToolCallStart:
			acc.start(event.ToolID, event.ToolName)
		case EventToolCallDelta:
			acc.appendArgs(event.ToolID, event.ArgumentsDelta)
		case EventFinish:
			finish = string(event.Finish)
		}
	}
	return assistantText, acc.executed(), finish
}

// toolCallAccumulator merges streamed argument fragments per call, in
// first-appearance order.
type toolCallAccumulator struct {
	order []ToolCallRef
	byID  map[string]*strings.Builder
}

func (a *toolCallAccumulator) start(id, name string) {
	if _, ok := a.byID[id]; ok {
		return
	}
	a.order = append(a.order, ToolCallRef{ID: id, Name: name})
	a.byID[id] = &strings.Builder{}
}

func (a *toolCallAccumulator) appendArgs(id, delta string) {
	builder := a.byID[id]
	if builder != nil {
		builder.WriteString(delta)
	}
}

// executed merges streamed argument fragments per call, in first
// appearance order; empty arguments normalize to {}. The per-id builders
// are seeded at Start so delta appends always find their target.
func (a *toolCallAccumulator) executed() []ToolCallRef {
	out := make([]ToolCallRef, 0, len(a.order))
	for _, ref := range a.order {
		args := "{}"
		if builder := a.byID[ref.ID]; builder != nil && builder.Len() > 0 {
			args = builder.String()
		}
		out = append(out, ToolCallRef{ID: ref.ID, Name: ref.Name, Arguments: json.RawMessage(args)})
	}
	return out
}

func openAITools(tools []ToolSpec) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  json.RawMessage(tool.Parameters),
			},
		})
	}
	return out
}

func truncateForLog(s string) string {
	if len(s) > 300 {
		return s[:300]
	}
	return s
}
