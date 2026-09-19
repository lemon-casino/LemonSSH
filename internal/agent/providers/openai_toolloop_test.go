package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sseHandler responds to POSTs with scripted SSE bodies, one per call.
func sseHandler(t *testing.T, bodies []string) http.Handler {
	t.Helper()
	call := 0
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if call >= len(bodies) {
			call = len(bodies) - 1
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(bodies[call]))
		call++
	})
}

func TestToolLoopExecutesToolsAndAnswers(t *testing.T) {
	first := `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"terminal_execute","arguments":"{\"sessionId\":\"s1\",\"command\":\"ls\"}"}}]}}]}

data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

`
	second := `data: {"choices":[{"index":0,"delta":{"content":"listing done"}}]}

data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

`
	server := httptest.NewServer(sseHandler(t, []string{first, second}))
	defer server.Close()

	var executedArgs string
	loop := &ToolLoop{
		Client:        server.Client(),
		Endpoint:      server.URL + "/v1/chat/completions",
		APIKeyHeader:  "Authorization",
		APIKeyValue:   "Bearer sk-test",
		Model:         "test-model",
		System:        "be helpful",
		MaxIterations: 4,
		ExecuteTool: func(ctx context.Context, toolName string, args json.RawMessage) (json.RawMessage, error) {
			executedArgs = string(args)
			return json.RawMessage(`{"output":"files"}`), nil
		},
	}

	result, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("loop: %v", err)
	}
	if result.ToolCalls != 1 {
		t.Errorf("tool calls = %d, want 1", result.ToolCalls)
	}
	if executedArgs != `{"sessionId":"s1","command":"ls"}` {
		t.Errorf("dispatched args = %s", executedArgs)
	}
	if result.FinalText != "listing done" {
		t.Errorf("final text = %q", result.FinalText)
	}
	if !result.FinishKnown {
		t.Errorf("finish must be known")
	}
}

func TestToolLoopTextOnlyTurn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`data: {"choices":[{"index":0,"delta":{"content":"plain answer"}}]}

data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

`))
	}))
	defer server.Close()

	loop := &ToolLoop{
		Client:   server.Client(),
		Endpoint: server.URL + "/v1/chat/completions",
	}
	result, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("loop: %v", err)
	}
	if result.FinalText != "plain answer" {
		t.Errorf("text = %q", result.FinalText)
	}
	if result.ToolCalls != 0 {
		t.Errorf("tool calls = %d, want 0", result.ToolCalls)
	}
}

func TestToolLoopHTTPErrorSurfaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer server.Close()

	loop := &ToolLoop{Client: server.Client(), Endpoint: server.URL}
	_, err := loop.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("http error must surface, got %v", err)
	}
}
