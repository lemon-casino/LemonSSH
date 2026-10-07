package main

// Client-level tests for the codex app-server JSON-RPC channel. Every test
// runs against an in-process fake transport (io.Pipe + goroutine server), so
// no codex binary is required.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAppServerTransport is a codexAppServerTransport whose "process" is a
// pair of io.Pipes. Kill closes stdin (so client writes fail) and closes the
// killed channel, which unblocks Wait.
type fakeAppServerTransport struct {
	mu        sync.Mutex
	stdin     io.WriteCloser // client write end of the stdin pipe
	stdout    io.Reader      // client read end of the stdout pipe
	closeOut  func()         // simulates the process death closing stdout
	killOnce  sync.Once
	killed    chan struct{}
	killCount int
	waitErr   error
}

func (f *fakeAppServerTransport) Stdin() io.WriteCloser { return f.stdin }
func (f *fakeAppServerTransport) Stdout() io.Reader     { return f.stdout }
func (f *fakeAppServerTransport) Stderr() io.Reader     { return nil }

func (f *fakeAppServerTransport) Wait() error {
	<-f.killed
	return f.waitErr
}

func (f *fakeAppServerTransport) Kill() error {
	f.mu.Lock()
	f.killCount++
	f.mu.Unlock()
	f.killOnce.Do(func() {
		// Simulate process death: the client's stdin writes fail and the
		// stdout reader EOFs, exactly like a killed child process.
		_ = f.stdin.Close()
		if f.closeOut != nil {
			f.closeOut()
		}
		close(f.killed)
	})
	return nil
}

func (f *fakeAppServerTransport) killCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.killCount
}

// fakeAppServer drives the client side of the protocol. Requests from the
// client land in requests; responses from scripted server calls land in
// responses. handle, when set, runs for every client request and may answer
// synchronously (typical: reply to initialize/thread/start/turn/start).
type fakeAppServer struct {
	transport *fakeAppServerTransport
	client    *codexAppServerClient
	requests  chan fakeRequest
	responses chan fakeResponse
	writeMu   sync.Mutex
	stdout    *io.PipeWriter // server→client write end
	handle    func(req fakeRequest)
	wg        sync.WaitGroup
}

type fakeRequest struct {
	id     int64
	method string
	params map[string]any
}

type fakeResponse struct {
	id     int64
	result json.RawMessage
	errMsg string
}

func newFakeAppServer(t *testing.T, handle func(fakeRequest)) *fakeAppServer {
	t.Helper()
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	fake := &fakeAppServer{
		transport: &fakeAppServerTransport{
			stdin:    stdinWriter,
			stdout:   stdoutReader,
			closeOut: func() { _ = stdoutWriter.Close() },
			killed:   make(chan struct{}),
		},
		requests:  make(chan fakeRequest, 32),
		responses: make(chan fakeResponse, 32),
		handle:    handle,
		stdout:    stdoutWriter,
	}
	fake.wg.Add(1)
	go func() {
		defer fake.wg.Done()
		scanner := bufio.NewScanner(stdinReader)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var msg codexAppServerMessage
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}
			if msg.Method != "" {
				id, _ := codexRPCIDToInt64(msg.ID)
				req := fakeRequest{id: id, method: msg.Method, params: map[string]any{}}
				if len(msg.Params) > 0 {
					_ = json.Unmarshal(msg.Params, &req.params)
				}
				fake.requests <- req
				if fake.handle != nil {
					fake.handle(req)
				}
				continue
			}
			id, _ := codexRPCIDToInt64(msg.ID)
			response := fakeResponse{id: id, result: msg.Result}
			if msg.Error != nil {
				response.errMsg = msg.Error.Message
			}
			fake.responses <- response
		}
	}()
	fake.client = newCodexAppServerClient(fake.transport)
	t.Cleanup(func() { fake.close() })
	return fake
}

// writeLine lets tests push server→client lines (notifications, requests).
func (f *fakeAppServer) writeLine(value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f.writeMu.Lock()
	defer f.writeMu.Unlock()
	_, err = f.stdout.Write(append(encoded, '\n'))
	return err
}

func (f *fakeAppServer) notify(t *testing.T, method string, params map[string]any) {
	t.Helper()
	if err := f.writeLine(map[string]any{"method": method, "params": params}); err != nil {
		t.Fatalf("fake server notify: %v", err)
	}
}

func (f *fakeAppServer) serverRequest(t *testing.T, id int64, method string, params map[string]any) {
	t.Helper()
	if err := f.writeLine(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		t.Fatalf("fake server request: %v", err)
	}
}

func (f *fakeAppServer) reply(id int64, result any) {
	_ = f.writeLine(map[string]any{"id": id, "result": result})
}

func (f *fakeAppServer) close() {
	f.client.Close()
	f.wg.Wait()
}

func waitForChannel[T any](t *testing.T, ch chan T, timeout time.Duration) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for channel value")
		var zero T
		return zero
	}
}

func TestCodexRPCIDToInt64Variants(t *testing.T) {
	if id, ok := codexRPCIDToInt64(float64(7)); !ok || id != 7 {
		t.Fatalf("float64 id: %d %v", id, ok)
	}
	if id, ok := codexRPCIDToInt64(json.Number("42")); !ok || id != 42 {
		t.Fatalf("json.Number id: %d %v", id, ok)
	}
	if _, ok := codexRPCIDToInt64("not-a-number"); ok {
		t.Fatalf("non-numeric string id should not parse")
	}
	if id, ok := codexRPCIDToInt64("12"); !ok || id != 12 {
		t.Fatalf("numeric string id: %d %v", id, ok)
	}
}

func TestCodexAppServerClientCallEncodesRequestAndDecodesResult(t *testing.T) {
	var fake *fakeAppServer
	fake = newFakeAppServer(t, func(req fakeRequest) {
		if req.method == "initialize" {
			fake.reply(req.id, map[string]any{"userAgent": "lemonssh-test/1", "codexHome": "C:\\x", "platformFamily": "windows", "platformOs": "windows"})
		}
	})
	fake.client.Start()

	result, err := fake.client.Call("initialize", map[string]any{"clientInfo": map[string]any{"name": "lemonssh-test", "version": "1"}}, 2*time.Second)
	if err != nil {
		t.Fatalf("initialize call: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("result decode: %v", err)
	}
	if payload["platformOs"] != "windows" {
		t.Fatalf("unexpected initialize result: %v", payload)
	}

	request := waitForChannel(t, fake.requests, time.Second)
	if request.method != "initialize" || request.id != 0 {
		t.Fatalf("unexpected request on the wire: %+v", request)
	}
	if request.params["clientInfo"].(map[string]any)["name"] != "lemonssh-test" {
		t.Fatalf("clientInfo was not encoded: %v", request.params)
	}
}

func TestCodexAppServerClientCallSurfacesProtocolError(t *testing.T) {
	var fake *fakeAppServer
	fake = newFakeAppServer(t, func(req fakeRequest) {
		if req.method == "thread/resume" {
			_ = fakeWriteError(fake, req.id, -32600, "failed to load configuration: bad toml")
		}
	})
	fake.client.Start()

	_, err := fake.client.Call("thread/resume", map[string]any{"threadId": "x"}, 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "-32600") || !strings.Contains(err.Error(), "bad toml") {
		t.Fatalf("expected protocol error with code and message, got %v", err)
	}
}

// fakeWriteError mirrors fakeAppServer.reply for handler closures that only
// receive the request.
func fakeWriteError(fake *fakeAppServer, id int64, code int, message string) error {
	return fake.writeLine(map[string]any{"id": id, "error": map[string]any{"code": code, "message": message}})
}

func TestCodexAppServerClientNotificationDispatch(t *testing.T) {
	fake := newFakeAppServer(t, nil)
	notifications := make(chan string, 8)
	fake.client.notify = func(method string, params json.RawMessage) {
		notifications <- method + ":" + string(params)
	}
	fake.client.Start()
	fake.notify(t, "thread/started", map[string]any{"thread": map[string]any{"id": "t-1"}})

	received := waitForChannel(t, notifications, time.Second)
	if !strings.HasPrefix(received, "thread/started:") || !strings.Contains(received, "t-1") {
		t.Fatalf("notification not dispatched: %q", received)
	}
}

func TestCodexAppServerClientServerRequestDispatchAndRespond(t *testing.T) {
	fake := newFakeAppServer(t, nil)
	fake.client.request = func(id int64, method string, params json.RawMessage) {
		go func() {
			_ = fake.client.Respond(id, map[string]any{"decision": "accept"})
		}()
	}
	fake.client.Start()
	fake.serverRequest(t, 11, "item/commandExecution/requestApproval", map[string]any{"threadId": "t-1", "itemId": "i-1"})

	response := waitForChannel(t, fake.responses, 2*time.Second)
	if response.id != 11 || !strings.Contains(string(response.result), "accept") {
		t.Fatalf("server request response mismatch: %+v", response)
	}
}

func TestCodexAppServerClientUnsupportedServerRequestFailsTyped(t *testing.T) {
	fake := newFakeAppServer(t, nil)
	fake.client.Start()
	fake.serverRequest(t, 12, "account/chatgptAuthTokens/refresh", map[string]any{})

	response := waitForChannel(t, fake.responses, 2*time.Second)
	if response.id != 12 || response.errMsg == "" {
		t.Fatalf("expected typed error response, got %+v", response)
	}
}

func TestCodexAppServerClientCallTimeoutCleansPending(t *testing.T) {
	originalTimeout := codexAppServerRequestTimeout
	codexAppServerRequestTimeout = 100 * time.Millisecond
	t.Cleanup(func() { codexAppServerRequestTimeout = originalTimeout })

	fake := newFakeAppServer(t, nil) // never replies
	fake.client.Start()

	start := time.Now()
	_, err := fake.client.Call("model/list", map[string]any{}, 100*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout did not fire promptly: %s", elapsed)
	}
	fake.client.mu.Lock()
	pending := len(fake.client.pending)
	fake.client.mu.Unlock()
	if pending != 0 {
		t.Fatalf("pending map leaked %d entries", pending)
	}
}

func TestCodexAppServerClientExitFailsPendingAndFiresOnExitOnce(t *testing.T) {
	fake := newFakeAppServer(t, nil)
	exits := make(chan error, 4)
	fake.client.onExit = func(err error) { exits <- err }
	fake.client.Start()

	callDone := make(chan error, 1)
	go func() {
		_, err := fake.client.Call("model/list", map[string]any{}, 10*time.Second)
		callDone <- err
	}()
	time.Sleep(50 * time.Millisecond)
	_ = fake.transport.Kill()

	select {
	case err := <-callDone:
		if err == nil || !strings.Contains(err.Error(), "codex app-server") {
			t.Fatalf("pending call should fail with exit error, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("pending call never failed after exit")
	}
	select {
	case <-exits:
	case <-time.After(time.Second):
		t.Fatalf("onExit never fired")
	}
	// onExit must fire exactly once even though both Wait() and readLoop EOF
	// observe the death.
	select {
	case extra := <-exits:
		t.Fatalf("onExit fired more than once: %v", extra)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestCodexAppServerClientCloseKillsTransportOnce(t *testing.T) {
	var fake *fakeAppServer
	fake = newFakeAppServer(t, func(req fakeRequest) {
		if req.method == "initialize" {
			fake.reply(req.id, map[string]any{"ok": true})
		}
	})
	fake.client.Start()
	if _, err := fake.client.Call("initialize", map[string]any{"clientInfo": map[string]any{}}, time.Second); err != nil {
		t.Fatalf("handshake call: %v", err)
	}
	fake.client.Close()
	fake.client.Close()
	if calls := fake.transport.killCalls(); calls != 1 {
		t.Fatalf("Kill should be called exactly once, got %d", calls)
	}
	if !fake.client.IsExited() {
		t.Fatalf("client should report exited after Close")
	}
	_, err := fake.client.Call("model/list", map[string]any{}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("calls after close must fail fast, got %v", err)
	}
}

func TestCodexAppServerClientStderrCaptured(t *testing.T) {
	stderrReader, stderrWriter := io.Pipe()
	fake := newFakeAppServer(t, nil)
	// Replace the nil stderr with a live pipe and start the drain manually.
	client := fake.client
	client.transport = &fakeAppServerTransportWithStderr{inner: fake.transport, stderr: stderrReader}
	client.Start()
	_, _ = stderrWriter.Write([]byte("ERROR codex_app_server: bad config line\n"))
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(client.StderrText(), "bad config line") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("stderr tail was not captured: %q", client.StderrText())
}

type fakeAppServerTransportWithStderr struct {
	inner  codexAppServerTransport
	stderr io.Reader
}

func (f *fakeAppServerTransportWithStderr) Stdin() io.WriteCloser { return f.inner.Stdin() }
func (f *fakeAppServerTransportWithStderr) Stdout() io.Reader     { return f.inner.Stdout() }
func (f *fakeAppServerTransportWithStderr) Stderr() io.Reader     { return f.stderr }
func (f *fakeAppServerTransportWithStderr) Wait() error           { return f.inner.Wait() }
func (f *fakeAppServerTransportWithStderr) Kill() error           { return f.inner.Kill() }

func TestLaunchCodexAppServerTransportRejectsMissingBinary(t *testing.T) {
	if _, err := launchCodexAppServerTransport("definitely-not-a-real-binary-xyz", []string{"app-server"}, nil, ""); err == nil && !errors.Is(err, context.Canceled) {
		// exec.LookPath must fail for a nonexistent binary on every platform.
		if err == nil {
			t.Fatalf("expected launch error for missing binary")
		}
	}
}
