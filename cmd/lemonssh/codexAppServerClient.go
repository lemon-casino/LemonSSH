package main

// codexAppServerClient speaks the newline-delimited JSON-RPC protocol of
// `codex app-server`. The wire format and every method used here were
// verified against codex-cli 0.130.0-alpha.5 (Windows x64) by running the
// real binary:
//   - initialize → {userAgent, codexHome, platformFamily, platformOs}, then
//     the `initialized` client notification;
//   - thread/start → {thread:{id,sessionId,status}, model, approvalPolicy,
//     sandbox, reasoningEffort, ...} plus a `thread/started` notification;
//   - thread/resume {threadId} (invalid ids fail with JSON-RPC -32600);
//   - turn/start {threadId, input:[{type:"text",text}], model?} →
//     {turn:{id,status}} and turn/started/item/started/item/completed/
//     item/agentMessage/delta/thread/tokenUsage/updated/turn/completed
//     notifications (agent deltas reassembled to the full reply text);
//   - turn/steer {threadId, expectedTurnId, input} → {turnId} (steer input
//     joined the running turn as a new userMessage item);
//   - turn/interrupt {threadId, turnId} → {};
//   - model/list → {data:[Model], nextCursor} with the live catalog;
//   - server→client requests item/commandExecution/requestApproval,
//     item/fileChange/requestApproval, item/permissions/requestApproval,
//     item/tool/requestUserInput (+ legacy execCommandApproval /
//     applyPatchApproval) expect {decision} / {answers} / {permissions,scope}
//     results — shapes taken from `codex app-server generate-ts` of the same
//     binary.
//
// The transport is an interface so tests can drive the client against a fake
// in-process app-server without the codex binary.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	// codexAppServerMaxMessageBytes bounds one JSON-RPC line. Item payloads
	// (file diffs, command output) can be large; 8 MiB matches the exec-mode
	// scanner headroom philosophy.
	codexAppServerMaxMessageBytes = 8 << 20
	// codexAppServerStderrBytes caps the diagnostic stderr tail kept for
	// error messages.
	codexAppServerStderrBytes = 16 * 1024
)

// codexAppServerTransport is the process surface the client drives. Tests
// inject fakes; production wraps exec.Cmd via streamingCommand (which handles
// Windows .cmd shims).
type codexAppServerTransport interface {
	Stdin() io.WriteCloser
	Stdout() io.Reader
	Stderr() io.Reader
	Wait() error
	Kill() error
}

type codexAppServerProcessTransport struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.Reader
	stderr  io.Reader
}

func (t *codexAppServerProcessTransport) Stdin() io.WriteCloser { return t.stdin }
func (t *codexAppServerProcessTransport) Stdout() io.Reader     { return t.stdout }
func (t *codexAppServerProcessTransport) Stderr() io.Reader     { return t.stderr }
func (t *codexAppServerProcessTransport) Wait() error           { return t.command.Wait() }

// Kill closes stdin first so a wedged server unblocks on its own reader, then
// hard-kills the process.
func (t *codexAppServerProcessTransport) Kill() error {
	if t.stdin != nil {
		_ = t.stdin.Close()
	}
	if t.command.Process != nil {
		return t.command.Process.Kill()
	}
	return nil
}

// launchCodexAppServerTransport starts `codex app-server` over stdio.
func launchCodexAppServerTransport(executable string, args []string, env []string, dir string) (codexAppServerTransport, error) {
	command := streamingCommand(context.Background(), executable, args)
	command.Env = env
	if dir != "" {
		command.Dir = dir
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	return &codexAppServerProcessTransport{command: command, stdin: stdin, stdout: stdout, stderr: stderr}, nil
}

// codexAppServerMessage is one JSON-RPC envelope in either direction. Params
// and Result stay raw so handlers decode only what they need.
type codexAppServerMessage struct {
	ID     any                     `json:"id,omitempty"`
	Method string                  `json:"method,omitempty"`
	Params json.RawMessage         `json:"params,omitempty"`
	Result json.RawMessage         `json:"result,omitempty"`
	Error  *codexAppServerRPCError `json:"error,omitempty"`
}

type codexAppServerRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// codexRPCIDToInt64 normalizes protocol request ids (number per RequestId in
// the generated protocol types) into the int64 the client keys pending calls
// by.
func codexRPCIDToInt64(value any) (int64, bool) {
	switch id := value.(type) {
	case float64:
		return int64(id), true
	case json.Number:
		parsed, err := id.Int64()
		return parsed, err == nil
	case int64:
		return id, true
	case int:
		return int64(id), true
	case string:
		var parsed int64
		if _, err := fmt.Sscanf(strings.TrimSpace(id), "%d", &parsed); err == nil {
			return parsed, true
		}
		return 0, false
	default:
		return 0, false
	}
}

// codexAppServerClient frames requests/responses/notifications over one app
// server process. Callers install notify/request hooks before Start; the
// request hook must eventually respond (respond/respondError) or the server
// wedges.
type codexAppServerClient struct {
	transport codexAppServerTransport
	notify    func(method string, params json.RawMessage)
	request   func(id int64, method string, params json.RawMessage)
	onExit    func(err error)

	mu      sync.Mutex
	stdinMu sync.Mutex
	nextID  int64
	pending map[int64]chan *codexAppServerMessage
	stderr  strings.Builder
	exited  chan struct{}
	exitErr error

	exitOnce  sync.Once
	closeOnce sync.Once
}

func newCodexAppServerClient(transport codexAppServerTransport) *codexAppServerClient {
	return &codexAppServerClient{
		transport: transport,
		pending:   make(map[int64]chan *codexAppServerMessage),
		exited:    make(chan struct{}),
	}
}

// Start launches the read loop and the exit watcher.
func (c *codexAppServerClient) Start() {
	go c.readLoop()
	if c.transport.Stderr() != nil {
		go c.drainStderr()
	}
	go c.waitLoop()
}

func (c *codexAppServerClient) readLoop() {
	scanner := bufio.NewScanner(c.transport.Stdout())
	scanner.Buffer(make([]byte, 64*1024), codexAppServerMaxMessageBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		c.handleLine(line)
	}
	c.markExited(fmt.Errorf("codex app-server stdout closed"))
}

func (c *codexAppServerClient) drainStderr() {
	scanner := bufio.NewScanner(c.transport.Stderr())
	scanner.Buffer(make([]byte, 16*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		c.mu.Lock()
		if c.stderr.Len() < codexAppServerStderrBytes {
			c.stderr.WriteString(line + "\n")
		}
		c.mu.Unlock()
	}
}

func (c *codexAppServerClient) waitLoop() {
	c.markExited(c.transport.Wait())
}

func (c *codexAppServerClient) handleLine(line string) {
	var msg codexAppServerMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		// Human-readable noise on stdout is dropped, never surfaced as text.
		return
	}
	if msg.Method != "" {
		if msg.ID != nil {
			id, ok := codexRPCIDToInt64(msg.ID)
			if !ok || c.request == nil {
				if ok {
					_ = c.RespondError(id, -32601, "LemonSSH does not support app-server method "+msg.Method)
				}
				return
			}
			c.request(id, msg.Method, msg.Params)
			return
		}
		if c.notify != nil {
			c.notify(msg.Method, msg.Params)
		}
		return
	}
	if msg.ID == nil {
		return
	}
	id, ok := codexRPCIDToInt64(msg.ID)
	if !ok {
		return
	}
	c.mu.Lock()
	waiter := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if waiter != nil {
		waiter <- &msg
	}
}

// markExited records the first exit cause, fails every in-flight call and
// notifies the owner. Idempotent.
func (c *codexAppServerClient) markExited(err error) {
	c.exitOnce.Do(func() {
		c.mu.Lock()
		if c.exitErr == nil {
			c.exitErr = err
		}
		pending := c.pending
		c.pending = make(map[int64]chan *codexAppServerMessage)
		c.mu.Unlock()
		close(c.exited)
		for _, waiter := range pending {
			waiter <- nil
		}
		if c.onExit != nil {
			c.onExit(err)
		}
	})
}

func (c *codexAppServerClient) IsExited() bool {
	select {
	case <-c.exited:
		return true
	default:
		return false
	}
}

func (c *codexAppServerClient) ExitError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exitErr
}

func (c *codexAppServerClient) StderrText() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.TrimSpace(c.stderr.String())
}

func (c *codexAppServerClient) writeLine(encoded []byte) error {
	c.stdinMu.Lock()
	defer c.stdinMu.Unlock()
	if _, err := c.transport.Stdin().Write(append(encoded, '\n')); err != nil {
		return err
	}
	return nil
}

// Call sends one request and waits for its response or the timeout. A dead
// process fails immediately; an exit during the wait fails the call.
func (c *codexAppServerClient) Call(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	if c.IsExited() {
		return nil, fmt.Errorf("codex app-server is not running")
	}
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	waiter := make(chan *codexAppServerMessage, 1)
	c.pending[id] = waiter
	c.mu.Unlock()

	discardWaiter := func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}
	encoded, err := json.Marshal(struct {
		ID     int64  `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params,omitempty"`
	}{ID: id, Method: method, Params: params})
	if err != nil {
		discardWaiter()
		return nil, err
	}
	if err := c.writeLine(encoded); err != nil {
		discardWaiter()
		return nil, fmt.Errorf("codex app-server write failed: %w", err)
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case msg := <-waiter:
		if msg == nil {
			cause := c.ExitError()
			if cause == nil {
				cause = fmt.Errorf("codex app-server exited")
			}
			return nil, fmt.Errorf("codex app-server %s failed: %w", method, cause)
		}
		if msg.Error != nil {
			return nil, fmt.Errorf("codex app-server %s failed (code %d): %s", method, msg.Error.Code, msg.Error.Message)
		}
		return msg.Result, nil
	case <-timer.C:
		discardWaiter()
		return nil, fmt.Errorf("codex app-server %s timed out after %s", method, timeout)
	}
}

// Respond resolves a server→client request with a result.
func (c *codexAppServerClient) Respond(id int64, result any) error {
	encoded, err := json.Marshal(struct {
		ID     int64 `json:"id"`
		Result any   `json:"result"`
	}{ID: id, Result: result})
	if err != nil {
		return err
	}
	return c.writeLine(encoded)
}

// RespondError resolves a server→client request with a JSON-RPC error, which
// the server treats as a denial/abort of the underlying operation.
func (c *codexAppServerClient) RespondError(id int64, code int, message string) error {
	encoded, err := json.Marshal(struct {
		ID    int64                  `json:"id"`
		Error codexAppServerRPCError `json:"error"`
	}{ID: id, Error: codexAppServerRPCError{Code: code, Message: message}})
	if err != nil {
		return err
	}
	return c.writeLine(encoded)
}

// Notify sends a client notification (no id, no response expected).
func (c *codexAppServerClient) Notify(method string, params any) error {
	encoded, err := json.Marshal(struct {
		Method string `json:"method"`
		Params any    `json:"params,omitempty"`
	}{Method: method, Params: params})
	if err != nil {
		return err
	}
	return c.writeLine(encoded)
}

// Close kills the process and waits briefly for the exit bookkeeping (pending
// failure + onExit) to run. Safe to call multiple times.
func (c *codexAppServerClient) Close() {
	c.closeOnce.Do(func() {
		_ = c.transport.Kill()
		// If the loops never ran, the exit bookkeeping must still happen;
		// markExited is idempotent, so a racing readLoop/waitLoop exit is a
		// no-op.
		c.markExited(fmt.Errorf("codex app-server closed"))
		select {
		case <-c.exited:
		case <-time.After(5 * time.Second):
		}
	})
}
