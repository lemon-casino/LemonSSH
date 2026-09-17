package providers

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("../../../testdata/ai/provider/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return raw
}

// feedWhole vs feedByteByByte must produce identical events (T17: chunk
// boundaries are invisible, including inside CJK/emoji sequences).
func feedAll(t *testing.T, raw []byte, chunkSize int) []Event {
	t.Helper()
	parser := &SSEParser{}
	stream := NewOpenAIChatStream()

	var events []Event
	for len(raw) > 0 {
		size := chunkSize
		if size > len(raw) {
			size = len(raw)
		}
		for _, ev := range parser.Feed(raw[:size]) {
			unified, err := stream.Consume(ev)
			if err != nil {
				t.Fatalf("consume: %v", err)
			}
			events = append(events, unified...)
		}
		raw = raw[size:]
	}
	return events
}

func eventsEqual(a, b []Event) bool {
	if len(a) != len(b) {
		return false
	}
	rawA, _ := json.Marshal(a)
	rawB, _ := json.Marshal(b)
	return string(rawA) == string(rawB)
}

func TestOpenAIChatBasicStream(t *testing.T) {
	raw := loadFixture(t, "openai-chat-basic.sse")

	whole := feedAll(t, raw, len(raw))
	byByte := feedAll(t, raw, 1)
	if !eventsEqual(whole, byByte) {
		t.Fatalf("chunk boundaries changed the stream:\nwhole %+v\nbyByte %+v", whole, byByte)
	}

	var text strings.Builder
	var usage *Usage
	var finish FinishReason
	for _, ev := range whole {
		switch ev.Kind {
		case EventTextDelta:
			text.WriteString(ev.Text)
		case EventUsage:
			usage = &ev.Usage
		case EventFinish:
			finish = ev.Finish
		}
	}
	if text.String() != "你好 🌍" {
		t.Errorf("text = %q, want %q", text.String(), "你好 🌍")
	}
	if usage == nil || !usage.Known || usage.InputTokens != 12 || usage.OutputTokens != 3 ||
		usage.CacheReadTokens != 4 || usage.ReasoningTokens != 2 {
		t.Errorf("usage = %+v", usage)
	}
	if finish != FinishStop {
		t.Errorf("finish = %q", finish)
	}
}

func TestOpenAIChatInterleavedToolCalls(t *testing.T) {
	raw := loadFixture(t, "openai-chat-toolcalls.sse")
	events := feedAll(t, raw, 3)

	// Assemble per-index argument fragments.
	arguments := map[int]*strings.Builder{}
	names := map[int]string{}
	var sawText bool
	var finish FinishReason
	for _, ev := range events {
		switch ev.Kind {
		case EventTextDelta:
			sawText = true
		case EventToolCallStart:
			names[ev.ToolIndex] = ev.ToolName
			arguments[ev.ToolIndex] = &strings.Builder{}
		case EventToolCallDelta:
			if arguments[ev.ToolIndex] == nil {
				t.Fatalf("delta before start for index %d", ev.ToolIndex)
			}
			arguments[ev.ToolIndex].WriteString(ev.ArgumentsDelta)
		case EventFinish:
			finish = ev.Finish
		}
	}
	if sawText {
		t.Errorf("tool-only stream must not carry text (T19)")
	}
	if names[0] != "terminal_execute" || names[1] != "sftp_list" {
		t.Errorf("tool names = %v", names)
	}
	if arguments[0].String() != `{"sessionId":"s1"}` {
		t.Errorf("index 0 arguments = %q", arguments[0].String())
	}
	if arguments[1].String() != `{"sessionId":"s2"}` {
		t.Errorf("index 1 arguments = %q", arguments[1].String())
	}
	if finish != FinishToolCall {
		t.Errorf("finish = %q", finish)
	}
}

func TestOpenAIChatCRLFAndMultiLineData(t *testing.T) {
	stream := `data: {"choices":[{"index":0,"delta":{"content":"a"}}]}` + "\r\n" +
		"\r\n" +
		"data: line1\n" +
		"data: line2\n" +
		"\r\n" +
		"data: [DONE]\r\n"
	parser := &SSEParser{}
	events := parser.Feed([]byte(stream))
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].Data != `{"choices":[{"index":0,"delta":{"content":"a"}}]}` {
		t.Errorf("first event data = %q", events[0].Data)
	}
	if events[1].Data != "line1\nline2" {
		t.Errorf("multi-line data = %q", events[1].Data)
	}
}

func TestOpenAIChatErrorChunk(t *testing.T) {
	stream := NewOpenAIChatStream()
	_, err := stream.Consume(SSEEvent{Data: `{"error":{"code":"rate_limit_exceeded","message":"slow down"}}`})
	var streamErr *StreamError
	if err == nil {
		t.Fatal("error chunk must surface")
	}
	if !asStreamError(err, &streamErr) || streamErr.Code != "rate_limit_exceeded" {
		t.Fatalf("expected StreamError with code, got %v", err)
	}
}

func TestOpenAIChatUnknownUsageStaysUnknown(t *testing.T) {
	stream := NewOpenAIChatStream()
	events, err := stream.Consume(SSEEvent{Data: `{"choices":[{"index":0,"delta":{"content":"hi"}}]}`})
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	for _, ev := range events {
		if ev.Kind == EventUsage {
			t.Errorf("no usage chunk arrived: usage must stay unknown, got %+v", ev.Usage)
		}
	}
}

func asStreamError(err error, target **StreamError) bool {
	if e, ok := err.(*StreamError); ok {
		*target = e
		return true
	}
	return false
}
