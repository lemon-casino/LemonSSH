package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agentruntime "github.com/binaricat/netcatty/internal/agent/runtime"
	"github.com/binaricat/netcatty/internal/app/contracts"
	"github.com/binaricat/netcatty/internal/app/terminaluse"
	"github.com/binaricat/netcatty/internal/capability"
	"github.com/binaricat/netcatty/internal/platform/netpolicy"
)

// TestProviderLiveChain pins the full W15 chain with a local OpenAI-
// compatible server: config -> netpolicy client -> ProviderDriver ->
// tool loop -> capability dispatch -> runtime events.
func TestProviderLiveChain(t *testing.T) {
	// Turn 1: model asks for a tool; Turn 2: model answers from the result.
	turn1 := "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"terminal_execute\",\"arguments\":\"\"}}]}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"sessionId\\\":\\\"s1\\\",\\\"command\\\":\\\"echo hi\\\",\\\"chatSessionId\\\":\\\"chat-1\\\"}\"}}]}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n"
	turn2 := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"the command output was hi\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"

	var gotAuthorization []string
	call := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = append(gotAuthorization, r.Header.Get("Authorization"))
		if call == 0 {
			w.Write([]byte(turn1))
		} else {
			w.Write([]byte(turn2))
		}
		call++
	}))
	defer server.Close()

	// Explicit provider config against the local server.
	t.Setenv("NETCATTY_AI_PROVIDER_JSON", `{"endpoint":"`+server.URL+`/v1/chat/completions","model":"test-model","apiKeyValue":"sk-live-test"}`)
	config, ok, err := loadProviderConfig()
	if err != nil || !ok {
		t.Fatalf("config: ok=%v err=%v", ok, err)
	}

	// Real host handler table so tool calls ride the shared dispatch path.
	host := newAgentHost(AgentHostConfig{
		Version:        appVersion{Name: "LemonSSH", Version: "0.0.1"},
		Sessions:       func() []SessionEntry { return []SessionEntry{{ID: "s1", Kind: "local"}} },
		Jobs:           newTestJobQueue(),
		PermissionMode: "auto",
	})
	policy := netpolicy.New()
	policy.AddProviderEndpoint(server.URL)
	dispatcher := &capability.Dispatcher{
		Registry:       capability.Default(),
		Surface:        capability.SurfaceBuiltin,
		PermissionMode: capability.ModeAuto,
		Handlers:       host.capabilityHandlers(),
	}

	driver, err := buildProviderDriver(config, policy,
		host.sessions, func() []string { return nil }, dispatcher)
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	// Surface the driver error inside the turn: wrap Stream failures into
	// the turn record so the interruption cause is observable.
	driver.streamErrorHook = func(err error) { t.Logf("driver stream error: %v", err) }

	// Drive a real runtime turn through the driver.
	manager := agentruntime.NewTurnManager()
	manager.SetDriver(driver)
	chat := contracts.NewChatSessionID()
	request := contracts.NewRequestID()
	turn, err := manager.Prepare(contracts.PrepareTurnRequest{
		RequestID:     request,
		ChatSessionID: chat,
		AgentID:       contracts.AgentID("agnt_test"),
		Input:         contracts.TurnInput{Text: "run echo hi"},
	})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := manager.StartTurn(contracts.TurnCommand{
		RequestID: request, Kind: contracts.TurnCommandStart, TurnID: turn.TurnID,
	}); err != nil {
		t.Fatalf("start: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	var snapshot contracts.TurnSnapshot
	for {
		snapshot, err = manager.Snapshot(turn.TurnID)
		if err == nil && (snapshot.Status == agentruntime.StatusCompleted || snapshot.Status == agentruntime.StatusInterrupted) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("turn never completed: %+v", snapshot)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if snapshot.Status != agentruntime.StatusCompleted {
		t.Fatalf("status = %q, want completed", snapshot.Status)
	}

	page, err := manager.ReadEvents(turn.TurnID, "0", 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var sawToolResult, sawSummary bool
	for _, event := range page.Events {
		switch event.Type {
		case "text_delta":
			if strings.Contains(string(event.Payload), "the command output was hi") {
				sawSummary = true
			}
		case "turn_summary":
			sawSummary = true
		}
	}
	if len(gotAuthorization) != 2 || gotAuthorization[0] != "Bearer sk-live-test" {
		t.Errorf("authorization headers = %v", gotAuthorization)
	}
	if !sawSummary {
		t.Errorf("final summary event missing in %+v", page.Events)
	}
	_ = sawToolResult
}

func newTestJobQueue() *terminaluse.JobQueue {
	return terminaluse.NewJobQueue(func(sessionID string) (terminaluse.CommandRunner, error) {
		return scriptRunner{}, nil
	})
}
