package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"
)

const codebuddyInitializeID = "lemonssh-initialize"

type codebuddyPending struct {
	controlID  string
	protocolID string
	timer      *time.Timer
}

type codebuddyRunState struct {
	mu                sync.Mutex
	writeMu           sync.Mutex
	stdin             io.WriteCloser
	request           ExternalAgentStreamRequest
	prompt            string
	pending           map[string]*codebuddyPending
	closed            bool
	initialized       bool
	initTimer         *time.Timer
	streamedText      bool
	streamedReasoning bool
}

func newCodebuddyRun(stdin io.WriteCloser, request ExternalAgentStreamRequest, prompt string) *codebuddyRunState {
	return &codebuddyRunState{stdin: stdin, request: request, prompt: prompt, pending: make(map[string]*codebuddyPending)}
}

func codebuddyArgs(request ExternalAgentStreamRequest) []string {
	args := []string{"--input-format=stream-json", "--output-format=stream-json", "--verbose", "--include-partial-messages", "--setting-sources", "none", "--strict-mcp-config", "--disallowedTools", "AskUserQuestion"}
	tools := ""
	if request.ToolIntegrationMode == "skills" {
		tools = "Bash"
	}
	args = append(args, "--tools", tools)
	model, effort := request.Model, ""
	if index := strings.LastIndex(model, "/"); index > 0 {
		switch model[index+1:] {
		case "low", "medium", "high", "xhigh":
			model, effort = model[:index], model[index+1:]
		}
	}
	args = appendModel(args, "codebuddy", model)
	if effort == "" {
		effort = stringAt(request.CodebuddyOptions, "effort")
	}
	if effort != "" {
		args = append(args, "--effort", effort)
	}
	if id := decodeExternalSessionID(request.ExistingSessionID, "codebuddy"); id != "" {
		args = append(args, "--resume", id)
	}
	if maxTurns := numberAt(request.CodebuddyOptions, "maxTurns"); maxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprint(int(maxTurns)))
	}
	if fallback := stringAt(request.CodebuddyOptions, "fallbackModel"); fallback != "" {
		args = append(args, "--fallback-model", fallback)
	}
	return args
}

func (c *codebuddyRunState) write(message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return errors.New("CodeBuddy turn is no longer active")
	}
	_, err = c.stdin.Write(append(data, '\n'))
	return err
}

func (c *codebuddyRunState) respond(controlID string, response map[string]any) error {
	return c.write(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": controlID, "response": response}})
}

func (s *ExternalAgentService) initializeCodebuddy(run *externalAgentRun) error {
	c := run.codebuddy
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("CodeBuddy turn ended before initialization")
	}
	c.initTimer = time.AfterFunc(30*time.Second, func() {
		c.mu.Lock()
		waiting := !c.initialized && !c.closed
		c.mu.Unlock()
		if waiting {
			run.mu.Lock()
			run.protocolError = "CodeBuddy control initialization timed out"
			run.mu.Unlock()
			s.closeCodebuddy(run)
			if run.cancel != nil {
				run.cancel()
			}
		}
	})
	c.mu.Unlock()
	request := map[string]any{
		"subtype": "initialize", "hasPrompt": true,
		"capabilities": map[string]any{"elicitation": map[string]any{"form": true}},
		"hooks":        map[string]any{"PreToolUse": []any{map[string]any{"hookCallbackIds": []string{"lemonssh-pre-tool"}}}},
	}
	if prompt := c.request.CodebuddyOptions["systemPrompt"]; prompt != nil {
		request["appendSystemPrompt"] = prompt
	}
	if agents := c.request.CodebuddyOptions["agents"]; agents != nil {
		request["agents"] = agents
	}
	return c.write(map[string]any{"type": "control_request", "request_id": codebuddyInitializeID, "request": request})
}

func codebuddyElicitationID(run *externalAgentRun, protocolID string) string {
	return "codebuddy:" + url.PathEscape(run.chatSessionID) + ":" + url.PathEscape(run.requestID) + ":" + url.PathEscape(protocolID)
}

func (s *ExternalAgentService) handleCodebuddyControl(run *externalAgentRun, event map[string]any) bool {
	c := run.codebuddy
	if c == nil {
		return false
	}
	switch stringAt(event, "type") {
	case "control_response":
		if stringAt(event, "response", "request_id") != codebuddyInitializeID {
			return true
		}
		c.mu.Lock()
		if c.initialized || c.closed {
			c.mu.Unlock()
			return true
		}
		c.initialized = true
		if c.initTimer != nil {
			c.initTimer.Stop()
		}
		c.mu.Unlock()
		if stringAt(event, "response", "subtype") == "error" {
			run.mu.Lock()
			run.protocolError = "CodeBuddy initialization failed: " + stringAt(event, "response", "error")
			run.mu.Unlock()
			s.closeCodebuddy(run)
			if run.cancel != nil {
				run.cancel()
			}
			return true
		}
		if err := c.write(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": c.prompt}, "parent_tool_use_id": nil}); err != nil {
			run.mu.Lock()
			run.stderr.WriteString(err.Error())
			run.mu.Unlock()
			if run.cancel != nil {
				run.cancel()
			}
		}
		return true
	case "control_request":
		id := stringAt(event, "request_id")
		request, ok := event["request"].(map[string]any)
		if !ok || id == "" {
			return true
		}
		switch stringAt(request, "subtype") {
		case "elicitation_create":
			params := make(map[string]any, len(request))
			for k, v := range request {
				if k != "subtype" {
					params[k] = v
				}
			}
			if mode := stringAt(params, "mode"); mode != "" && mode != "form" {
				_ = c.respond(id, map[string]any{"action": "cancel"})
				return true
			}
			protocolID := stringAt(params, "_meta", "codebuddy.ai", "elicitationId")
			if protocolID == "" {
				protocolID = id
			}
			key := codebuddyElicitationID(run, protocolID)
			pending := &codebuddyPending{controlID: id, protocolID: protocolID}
			c.mu.Lock()
			if c.closed || len(c.pending) >= 128 {
				c.mu.Unlock()
				_ = c.respond(id, map[string]any{"action": "cancel"})
				return true
			}
			previous := c.pending[key]
			if previous != nil && previous.timer != nil {
				previous.timer.Stop()
			}
			c.pending[key] = pending
			pending.timer = time.AfterFunc(10*time.Minute, func() { s.expireCodebuddyElicitation(run, key, pending) })
			c.mu.Unlock()
			if previous != nil {
				_ = c.respond(previous.controlID, map[string]any{"action": "cancel"})
			}
			s.emitEvent(run.requestID, map[string]any{"type": "elicitation-create", "elicitationId": key, "request": params})
		case "can_use_tool":
			allowed := codebuddyToolAllowed(c.request, stringAt(request, "tool_name"), request["input"])
			_ = c.respond(id, map[string]any{"allowed": allowed, "reason": "Only the configured LemonSSH tools are allowed; host approvals still apply.", "tool_use_id": request["tool_use_id"]})
		case "hook_callback":
			input, _ := request["input"].(map[string]any)
			allowed := codebuddyToolAllowed(c.request, stringAt(input, "tool_name"), input["tool_input"])
			response := map[string]any{"continue": true}
			if !allowed {
				response["decision"] = "block"
				response["reason"] = "Only the configured LemonSSH tools are allowed."
			}
			_ = c.respond(id, response)
		default:
			_ = c.write(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "error", "request_id": id, "error": "Unsupported control request"}})
		}
		return true
	case "control_notification":
		if stringAt(event, "channel") == "elicitation" {
			if notification, ok := event["data"].(map[string]any); ok {
				protocolID := stringAt(notification, "elicitationId")
				key := codebuddyElicitationID(run, protocolID)
				c.mu.Lock()
				pending := c.pending[key]
				delete(c.pending, key)
				c.mu.Unlock()
				if pending != nil {
					pending.timer.Stop()
					_ = c.respond(pending.controlID, map[string]any{"action": "cancel"})
				}
				copy := make(map[string]any, len(notification))
				for k, v := range notification {
					copy[k] = v
				}
				copy["elicitationId"] = key
				s.emitEvent(run.requestID, map[string]any{"type": "elicitation-complete", "notification": copy})
			}
		}
		return true
	case "control_cancel_request":
		c.mu.Lock()
		for key, pending := range c.pending {
			if pending.controlID == stringAt(event, "request_id") {
				delete(c.pending, key)
				pending.timer.Stop()
				c.mu.Unlock()
				s.emitCodebuddyComplete(run, key)
				return true
			}
		}
		c.mu.Unlock()
		return true
	case "stream_event":
		if stringAt(event, "event", "type") == "message_start" {
			c.mu.Lock()
			c.streamedText, c.streamedReasoning = false, false
			c.mu.Unlock()
		}
		if text := stringAt(event, "event", "delta", "text"); text != "" {
			c.mu.Lock()
			c.streamedText = true
			c.mu.Unlock()
			s.emitText(run.requestID, run, text)
		}
		if text := stringAt(event, "event", "delta", "thinking"); text != "" {
			c.mu.Lock()
			c.streamedReasoning = true
			c.mu.Unlock()
			s.emitEvent(run.requestID, map[string]any{"type": "reasoning-delta", "delta": text})
		}
		return true
	case "assistant", "user":
		message, _ := event["message"].(map[string]any)
		blocks, _ := message["content"].([]any)
		if blocks == nil {
			return false
		}
		c.mu.Lock()
		streamedText, streamedReasoning := c.streamedText, c.streamedReasoning
		c.mu.Unlock()
		for _, raw := range blocks {
			block, _ := raw.(map[string]any)
			switch stringAt(block, "type") {
			case "text":
				if !streamedText && stringAt(event, "type") == "assistant" {
					s.emitText(run.requestID, run, stringAt(block, "text"))
				}
			case "thinking":
				if !streamedReasoning {
					s.emitEvent(run.requestID, map[string]any{"type": "reasoning-delta", "delta": block["thinking"]})
				}
			case "tool_use":
				s.emitEvent(run.requestID, map[string]any{"type": "tool-call", "toolName": block["name"], "toolCallId": block["id"], "input": block["input"]})
			case "tool_result":
				s.emitEvent(run.requestID, map[string]any{"type": "tool-result", "toolCallId": block["tool_use_id"], "output": block["content"]})
			}
		}
		return true
	case "error":
		run.mu.Lock()
		run.protocolError = firstString(event, []string{"error", "message"}, []string{"error"}, []string{"message"})
		if run.protocolError == "" {
			run.protocolError = "CodeBuddy execution failed"
		}
		run.mu.Unlock()
		s.closeCodebuddy(run)
		if run.cancel != nil {
			run.cancel()
		}
		return true
	case "result":
		run.mu.Lock()
		emitted := run.emittedText
		run.mu.Unlock()
		if !emitted && event["is_error"] != true {
			s.emitText(run.requestID, run, stringAt(event, "result"))
		}
		if event["is_error"] == true {
			run.mu.Lock()
			run.protocolError = "CodeBuddy execution failed: " + fmt.Sprint(event["errors"])
			run.mu.Unlock()
		}
		s.closeCodebuddy(run)
		return true
	}
	return false
}

func (s *ExternalAgentService) emitCodebuddyComplete(run *externalAgentRun, id string) {
	s.emitEvent(run.requestID, map[string]any{"type": "elicitation-complete", "notification": map[string]any{"elicitationId": id}})
}

func (s *ExternalAgentService) expireCodebuddyElicitation(run *externalAgentRun, id string, pending *codebuddyPending) {
	c := run.codebuddy
	c.mu.Lock()
	if c.pending[id] != pending {
		c.mu.Unlock()
		return
	}
	delete(c.pending, id)
	if pending.timer != nil {
		pending.timer.Stop()
	}
	c.mu.Unlock()
	_ = c.respond(pending.controlID, map[string]any{"action": "cancel"})
	s.emitCodebuddyComplete(run, id)
}

func (s *ExternalAgentService) RespondCodebuddyElicitation(elicitationID, action string, content map[string]any) ExternalAgentResult {
	if action != "accept" && action != "decline" && action != "cancel" {
		return ExternalAgentResult{Error: "Invalid elicitation action"}
	}
	s.mu.Lock()
	runs := make([]*externalAgentRun, 0, len(s.active))
	for _, run := range s.active {
		runs = append(runs, run)
	}
	s.mu.Unlock()
	for _, run := range runs {
		c := run.codebuddy
		if c == nil {
			continue
		}
		c.mu.Lock()
		pending := c.pending[elicitationID]
		if pending == nil || c.closed {
			c.mu.Unlock()
			continue
		}
		delete(c.pending, elicitationID)
		pending.timer.Stop()
		c.mu.Unlock()
		response := map[string]any{"action": action}
		if action == "accept" && content != nil {
			response["content"] = content
		}
		if err := c.respond(pending.controlID, response); err != nil {
			s.emitCodebuddyComplete(run, elicitationID)
			return ExternalAgentResult{Error: err.Error()}
		}
		s.emitCodebuddyComplete(run, elicitationID)
		return ExternalAgentResult{OK: true}
	}
	return ExternalAgentResult{Error: "CodeBuddy elicitation is no longer pending"}
}

func (s *ExternalAgentService) closeCodebuddy(run *externalAgentRun) {
	c := run.codebuddy
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	if c.initTimer != nil {
		c.initTimer.Stop()
	}
	pending := c.pending
	c.pending = make(map[string]*codebuddyPending)
	c.mu.Unlock()
	_ = c.stdin.Close()
	for id, entry := range pending {
		if entry.timer != nil {
			entry.timer.Stop()
		}
		s.emitCodebuddyComplete(run, id)
	}
}

func codebuddyToolAllowed(request ExternalAgentStreamRequest, name string, raw any) bool {
	if request.ToolIntegrationMode != "skills" {
		return strings.HasPrefix(name, "mcp__lemonssh__")
	}
	if name != "Bash" {
		return false
	}
	input, _ := raw.(map[string]any)
	if input["run_in_background"] == true {
		return false
	}
	return isLemonSSHCLICommand(stringAt(input, "command"), resolveExternalAgentToolPath(), request.ChatSessionID)
}
