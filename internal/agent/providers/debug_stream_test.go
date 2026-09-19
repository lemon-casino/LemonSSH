package providers

import (
	"testing"
)

func TestDebugStreamDeltas(t *testing.T) {
	first := "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"terminal_execute\",\"arguments\":\"\"}}]}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"sessionId\\\":\\\"s1\\\",\\\"command\\\":\\\"ls\\\"}\"}}]}}]}\n\n"
	parser := &SSEParser{}
	stream := NewOpenAIChatStream()
	for _, ev := range parser.Feed([]byte(first)) {
		events, err := stream.Consume(ev)
		if err != nil {
			t.Fatalf("consume: %v", err)
		}
		for _, e := range events {
			t.Logf("kind=%s id=%q name=%q delta=%q", e.Kind, e.ToolID, e.ToolName, e.ArgumentsDelta)
		}
	}
}
