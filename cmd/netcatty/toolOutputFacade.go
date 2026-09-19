package main

import (
	"fmt"
	"strings"

	"github.com/binaricat/netcatty/internal/agent/tools"
)

// ToolOutputRead implements harness.tool_output.read over the W14 handle
// store: bounded head/tail/range/full/search reads with owner chat scope
// and explicit expired/not-found/evicted outcomes.
func (s *AgentService) ToolOutputRead(chatSessionID string, input tools.ReadOptions) (map[string]any, error) {
	if s.outputStore == nil {
		return nil, fmt.Errorf("tool output store is not available")
	}
	input.HandleID = strings.TrimSpace(input.HandleID)
	input.ChatSessionID = chatSessionID
	read, err := s.outputStore.Read(input)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"ok":         true,
		"content":    read.Content,
		"totalUnits": read.TotalUnits,
		"status":     read.Status,
	}
	if len(read.Matches) > 0 {
		payload["matches"] = read.Matches
	}
	return payload, nil
}

// ToolOutputStore stores tool output under a fresh handle.
func (s *AgentService) ToolOutputStore(chatSessionID, capabilityID, sessionID, content string, previewChars int) tools.StoreInputResult {
	return s.outputStore.Store(tools.StoreInput{
		Content:       content,
		ChatSessionID: chatSessionID,
		CapabilityID:  capabilityID,
		SessionID:     sessionID,
		PreviewChars:  previewChars,
	})
}
