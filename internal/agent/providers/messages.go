package providers

import (
	"context"
	"encoding/json"
	"strings"
)

// ChatMessage is the family-neutral conversation message. Role follows the
// OpenAI chat naming (system/user/assistant/tool); family adapters map to
// their wire shapes.
type ChatMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCallID string          `json:"toolCallId,omitempty"` // tool result correlation
	ToolCalls  []ToolCallRef   `json:"toolCalls,omitempty"`  // assistant-issued calls
	Name       string          `json:"name,omitempty"`
	Private    json.RawMessage `json:"-"` // provider-private continuation, never marshaled
}

// ToolCallRef is one tool invocation inside an assistant message.
type ToolCallRef struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolSpec is one tool exposed to the model.
type ToolSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Parameters is a JSON Schema object.
	Parameters json.RawMessage `json:"parameters"`
}

// ToolExecutor runs one tool call. Implementations route through the
// capability dispatcher so policy/approval apply identically to every
// caller (single path rule).
type ToolExecutor interface {
	ExecuteTool(ctx context.Context, toolName string, args json.RawMessage) (json.RawMessage, error)
}

// SystemPromptInput carries the dynamic host context sections (§W15:
// skills/system prompt/hostChain/activePortForwards).
type SystemPromptInput struct {
	Base             string
	TerminalSession  []SessionLine
	PortForwards     []string
	WorkingDirectory string
}

type SessionLine struct {
	ID    string
	Label string
	Kind  string
	CWD   string
}

// BuildSystemPrompt renders the dynamic system prompt: base identity plus
// live host context sections. Empty sections are omitted.
func BuildSystemPrompt(input SystemPromptInput) string {
	prompt := input.Base
	if input.Base == "" {
		prompt = "You are Catty, the LemonSSH assistant."
	}
	var sections []string
	if len(input.TerminalSession) > 0 {
		var lines []string
		for _, session := range input.TerminalSession {
			line := "- " + session.ID
			if session.Label != "" {
				line += " (" + session.Label + ")"
			}
			if session.Kind != "" {
				line += " [" + session.Kind + "]"
			}
			if session.CWD != "" {
				line += " cwd=" + session.CWD
			}
			lines = append(lines, line)
		}
		sections = append(sections, "## Terminal sessions\n"+strings.Join(lines, "\n"))
	}
	if len(input.PortForwards) > 0 {
		var forwardLines []string
		for _, forward := range input.PortForwards {
			forwardLines = append(forwardLines, "- "+forward)
		}
		sections = append(sections, "## Active port forwards\n"+strings.Join(forwardLines, "\n"))
	}
	if input.WorkingDirectory != "" {
		sections = append(sections, "Working directory: "+input.WorkingDirectory)
	}
	if len(sections) == 0 {
		return prompt
	}
	return prompt + "\n\n" + strings.Join(sections, "\n\n")
}
