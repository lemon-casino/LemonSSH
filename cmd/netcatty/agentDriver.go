package main

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/binaricat/netcatty/internal/agent/providers"
	"github.com/binaricat/netcatty/internal/agent/runtime"
	"github.com/binaricat/netcatty/internal/capability"
)

// ProviderDriver adapts providers.ToolLoop to the runtime.TurnDriver seam
// (W15 slice 2). It resolves the model-facing tool list from the
// capability authority, executes model tool calls through the shared
// capability dispatcher (policy + approval inside) and streams text
// deltas into the runtime event sink.
type ProviderDriver struct {
	client        providers.HTTPClient
	endpoint      string
	apiKeyHeader  string
	apiKeyValue   string
	model         string
	systemBase    string
	maxIterations int
	sessions      func() []SessionEntry
	portForwards  func() []string
	dispatcher    *capability.Dispatcher
	// streamErrorHook observes loop failures (test diagnostics; the
	// runtime already records the error as an interrupted turn).
	streamErrorHook func(error)
}

func NewProviderDriver(
	client providers.HTTPClient,
	endpoint, apiKeyHeader, apiKeyValue, model, systemBase string,
	maxIterations int,
	sessions func() []SessionEntry,
	portForwards func() []string,
	dispatcher *capability.Dispatcher,
) *ProviderDriver {
	if maxIterations <= 0 {
		maxIterations = 8
	}
	return &ProviderDriver{
		client: client, endpoint: endpoint,
		apiKeyHeader: apiKeyHeader, apiKeyValue: apiKeyValue,
		model: model, systemBase: systemBase, maxIterations: maxIterations,
		sessions: sessions, portForwards: portForwards, dispatcher: dispatcher,
	}
}

// Stream implements runtime.TurnDriver: one user message in, multi-step
// tool loop out, deltas streamed through the runtime sink.
func (d *ProviderDriver) Stream(ctx context.Context, session *runtime.DriverSession) error {
	loop := &providers.ToolLoop{
		Client:        d.client,
		Endpoint:      d.endpoint,
		APIKeyHeader:  d.apiKeyHeader,
		APIKeyValue:   d.apiKeyValue,
		Model:         d.model,
		System:        d.systemPrompt(),
		Messages:      []providers.ChatMessage{{Role: "user", Content: session.Input.Text}},
		Tools:         d.modelTools(),
		MaxIterations: d.maxIterations,
		ExecuteTool: func(ctx context.Context, toolName string, args json.RawMessage) (json.RawMessage, error) {
			return d.executeTool(ctx, string(session.ChatID), toolName, args)
		},
		OnTextDelta: func(text string) {
			raw, _ := json.Marshal(map[string]string{"text": text})
			_ = session.Emit("text_delta", raw)
		},
	}
	result, err := loop.Run(ctx)
	if err != nil {
		if d.streamErrorHook != nil {
			d.streamErrorHook(err)
		}
		return err
	}
	summary := map[string]any{"status": "completed", "finalText": result.FinalText, "iterations": result.Iterations}
	raw, _ := json.Marshal(summary)
	return session.Emit("turn_summary", raw)
}

// modelTools projects the catalog sidebar/global tool list into model
// tool specs.
func (d *ProviderDriver) modelTools() []providers.ToolSpec {
	var specs []providers.ToolSpec
	for _, def := range capability.Default().List(capability.ListOptions{Status: capability.StatusImplemented}) {
		if !def.IsAgentEligibleForKind(capability.AgentKindSidebar) {
			continue
		}
		method := def.ResolveCLIRPCMethod()
		if method == "" {
			continue
		}
		fields, _ := capability.ToolInputFields(def.ID)
		properties := make(map[string]any, len(fields))
		var required []string
		for name, field := range fields {
			property := map[string]any{"type": field.Type}
			if field.Description != "" {
				property["description"] = field.Description
			}
			properties[name] = property
			if !field.Optional {
				required = append(required, name)
			}
		}
		schema := map[string]any{"type": "object", "properties": properties}
		if len(required) > 0 {
			sort.Strings(required)
			schema["required"] = required
		}
		raw, _ := json.Marshal(schema)
		specs = append(specs, providers.ToolSpec{
			Name:        def.CattyToolName(),
			Description: def.ToolDescription(),
			Parameters:  raw,
		})
	}
	return specs
}

// executeTool resolves a model tool name to its capability and dispatches
// through the shared capability dispatcher (policy + approval inside).
func (d *ProviderDriver) executeTool(ctx context.Context, chatSessionID, toolName string, args json.RawMessage) (json.RawMessage, error) {
	def := capability.Default().GetByToolName(toolName)
	if def == nil {
		// The model may use the dotted capability id as a name too.
		def = capability.Default().GetByID(strings.ReplaceAll(toolName, "_", "."))
	}
	if def == nil {
		return nil, &capability.DispatchError{Code: "UNKNOWN_TOOL", Message: "unknown tool " + toolName}
	}
	method := def.AgentRPCMethod()
	if method == "" {
		return nil, &capability.DispatchError{Code: "UNKNOWN_TOOL", Message: "capability has no served method"}
	}
	params := map[string]any{"chatSessionId": chatSessionID}
	if len(args) > 0 {
		_ = json.Unmarshal(args, &params)
	}
	raw, err := d.dispatcher.Dispatch(ctx, method, params)
	if err != nil {
		return nil, err
	}
	rawString, ok := raw.(string)
	if !ok {
		rawBytes, _ := json.Marshal(raw)
		rawString = string(rawBytes)
	}
	return json.RawMessage(rawString), nil
}

// systemPrompt renders the dynamic host context for the turn.
func (d *ProviderDriver) systemPrompt() string {
	var sessions []providers.SessionLine
	if d.sessions != nil {
		for _, entry := range d.sessions() {
			sessions = append(sessions, providers.SessionLine{ID: entry.ID, Label: entry.Label, Kind: entry.Kind})
		}
	}
	var forwards []string
	if d.portForwards != nil {
		forwards = d.portForwards()
	}
	return providers.BuildSystemPrompt(providers.SystemPromptInput{
		TerminalSession: sessions,
		PortForwards:    forwards,
	})
}
