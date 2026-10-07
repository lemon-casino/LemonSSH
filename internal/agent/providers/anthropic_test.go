package providers

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAnthropicStreamFullTurn(t *testing.T) {
	raw := loadFixture(t, "anthropic-messages.sse")
	stream := NewAnthropicStream()
	parser := &SSEParser{}

	var events []Event
	for len(raw) > 0 {
		size := 5
		if size > len(raw) {
			size = len(raw)
		}
		for _, sse := range parser.Feed(raw[:size]) {
			unified, err := stream.Consume(sse)
			if err != nil {
				t.Fatalf("consume: %v", err)
			}
			events = append(events, unified...)
		}
		raw = raw[size:]
	}

	var text strings.Builder
	var reasoning strings.Builder
	var usage *Usage
	var finish FinishReason
	var privateRecords []json.RawMessage
	for _, ev := range events {
		switch ev.Kind {
		case EventTextDelta:
			text.WriteString(ev.Text)
		case EventReasoningDelta:
			reasoning.WriteString(ev.Text)
		case EventUsage:
			usage = &ev.Usage
		case EventFinish:
			finish = ev.Finish
		case EventPrivateRecord:
			privateRecords = append(privateRecords, ev.ProviderPrivate)
		case EventToolCallStart:
			if ev.ToolID != "toolu_9" || ev.ToolName != "sftp_list" || ev.ToolIndex != 2 {
				t.Errorf("tool start = %+v", ev)
			}
		case EventToolCallDelta:
			if ev.ArgumentsDelta != `{"sessionId":"s1"}` {
				t.Errorf("tool arguments delta = %q", ev.ArgumentsDelta)
			}
		}
	}
	if text.String() != "Answer" {
		t.Errorf("text = %q", text.String())
	}
	if reasoning.String() != "思考..." {
		t.Errorf("reasoning = %q", reasoning.String())
	}
	if usage == nil || usage.InputTokens != 25 || usage.OutputTokens != 41 ||
		usage.CacheReadTokens != 3 || usage.CacheWriteTokens != 7 || !usage.Known {
		t.Errorf("merged usage = %+v", usage)
	}
	if finish != FinishToolCall {
		t.Errorf("finish = %q", finish)
	}
	if len(privateRecords) != 1 {
		t.Fatalf("expected one signature continuation record, got %d", len(privateRecords))
	}
	if !strings.Contains(string(privateRecords[0]), `"signature":"sigABC"`) {
		t.Errorf("signature record = %s", privateRecords[0])
	}
}

func TestAnthropicStreamError(t *testing.T) {
	stream := NewAnthropicStream()
	_, err := stream.Consume(SSEEvent{Data: `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`})
	if err == nil {
		t.Fatal("error event must surface")
	}
	var streamErr *StreamError
	if !errorsAs(err, &streamErr) || streamErr.Code != "overloaded_error" {
		t.Fatalf("expected StreamError with overloaded_error code, got %v", err)
	}
}

func errorsAs(err error, target **StreamError) bool {
	if e, ok := err.(*StreamError); ok {
		*target = e
		return true
	}
	return false
}
