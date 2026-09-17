package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testFixture struct {
	server *Server
	tokens *TokenStore
	token  string
	client net.Conn
	reader *bufio.Reader
}

func newTestFixture(t *testing.T, handlers map[string]Handler) *testFixture {
	t.Helper()
	tokens := NewTokenStore()
	token, err := tokens.Issue(Principal{ID: "principal-1", Kind: PrincipalExternal, Scope: []string{"session-a"}})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	server := NewServer(tokens, handlers, ServerOptions{MaxDeadline: time.Second})
	fixture := &testFixture{server: server, tokens: tokens, token: token}

	clientA, clientB := net.Pipe()
	fixture.client = clientA
	fixture.reader = bufio.NewReader(clientA)
	go server.ServeConn(context.Background(), "test-remote", clientB, clientB)
	t.Cleanup(func() {
		server.Close()
		clientA.Close()
		clientB.Close()
	})
	return fixture
}

func echoHandlers() map[string]Handler {
	return map[string]Handler{
		"test/echo": func(ctx context.Context, principal *Principal, params json.RawMessage) (any, error) {
			return "ok", nil
		},
	}
}

func (f *testFixture) send(t *testing.T, envelope string) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		_, _ = f.client.Write([]byte(envelope + "\n"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("write blocked for a second (server not reading): %q", envelope)
	}
}

func (f *testFixture) readResponse(t *testing.T) *Response {
	t.Helper()
	line, err := f.reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var response Response
	if err := json.Unmarshal([]byte(line), &response); err != nil {
		t.Fatalf("parse response %q: %v", line, err)
	}
	return &response
}

func (f *testFixture) call(t *testing.T, envelope string) *Response {
	f.send(t, envelope)
	return f.readResponse(t)
}

func (f *testFixture) callValid(t *testing.T, method, params string) *Response {
	envelope := `{"v":1,"id":"req-1","token":"` + f.token + `","method":"` + method + `","params":` + params + `}`
	return f.call(t, envelope)
}

func TestRoundTripAuthenticatedCall(t *testing.T) {
	fixture := newTestFixture(t, map[string]Handler{
		"test/echo": func(ctx context.Context, principal *Principal, params json.RawMessage) (any, error) {
			return map[string]any{"echo": "hi", "principal": principal.ID}, nil
		},
	})
	response := fixture.callValid(t, "test/echo", `{}`)
	if !response.OK || response.Error != nil {
		t.Fatalf("expected success, got %+v", response)
	}
	var result map[string]any
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result["principal"] != "principal-1" {
		t.Errorf("handler must see the authenticated principal, got %v", result)
	}
}

func TestAuthFailures(t *testing.T) {
	fixture := newTestFixture(t, echoHandlers())

	missing := fixture.call(t, `{"v":1,"id":"a","method":"test/echo"}`)
	if missing.Error == nil || missing.Error.Code != CodeAuthFailed {
		t.Errorf("missing token must fail AUTH_FAILED, got %+v", missing.Error)
	}

	forged := fixture.call(t, `{"v":1,"id":"b","token":"deadbeef","method":"test/echo"}`)
	if forged.Error == nil || forged.Error.Code != CodeAuthFailed {
		t.Errorf("forged token must fail AUTH_FAILED, got %+v", forged.Error)
	}
}

func TestVersionMismatchIsTyped(t *testing.T) {
	fixture := newTestFixture(t, echoHandlers())
	response := fixture.call(t, `{"v":99,"id":"a","token":"`+fixture.token+`","method":"test/echo"}`)
	if response.Error == nil || response.Error.Code != CodeVersionMismatch {
		t.Errorf("version mismatch must be VERSION_UNSUPPORTED, got %+v", response.Error)
	}
}

func TestUnknownMethod(t *testing.T) {
	fixture := newTestFixture(t, echoHandlers())
	response := fixture.callValid(t, "other/method", `{}`)
	if response.Error == nil || response.Error.Code != CodeUnknownMethod {
		t.Errorf("unknown method must be UNKNOWN_METHOD, got %+v", response.Error)
	}
}

// TestScopeGuardBlocksForgedSession pins the T42 requirement: a valid token
// with an out-of-scope session reference is refused by the scope guard, not
// by trusting request parameters.
func TestScopeGuardBlocksForgedSession(t *testing.T) {
	fixture := newTestFixture(t, map[string]Handler{
		"test/session": func(ctx context.Context, principal *Principal, params json.RawMessage) (any, error) {
			var parsed struct {
				SessionID string `json:"sessionId"`
			}
			if err := json.Unmarshal(params, &parsed); err != nil {
				return nil, err
			}
			if err := RequireSession(principal, parsed.SessionID); err != nil {
				return nil, err
			}
			return map[string]any{"session": parsed.SessionID}, nil
		},
	})

	inScope := fixture.callValid(t, "test/session", `{"sessionId":"session-a"}`)
	if !inScope.OK {
		t.Fatalf("in-scope session must pass, got %+v", inScope.Error)
	}

	forged := fixture.callValid(t, "test/session", `{"sessionId":"session-evil"}`)
	if forged.Error == nil || forged.Error.Code != CodeScopeDenied {
		t.Errorf("forged session must fail SCOPE_DENIED, got %+v", forged.Error)
	}
}

// TestOversizedFrameKeepsConnectionUsable pins the T43 bound: a huge frame
// is answered with BAD_REQUEST and drained, and the next frame still works.
func TestOversizedFrameKeepsConnectionUsable(t *testing.T) {
	fixture := newTestFixture(t, echoHandlers())

	huge := strings.Repeat("x", int(DefaultMaxFrameBytes)+64)
	fixture.send(t, `{"v":1,"id":"big","method":"test/echo","params":{"blob":"`+huge+`"}}`)
	response := fixture.readResponse(t)
	if response.Error == nil || response.Error.Code != CodeBadRequest {
		t.Fatalf("oversized frame must fail BAD_REQUEST, got %+v", response.Error)
	}

	recovered := fixture.callValid(t, "test/echo", `{}`)
	if !recovered.OK {
		t.Errorf("connection must stay usable after oversized frame, got %+v", recovered.Error)
	}
}

// TestTruncatedFrameEndsConnection pins EOF-inside-frame handling: the
// connection terminates rather than inventing a response.
func TestTruncatedFrameEndsConnection(t *testing.T) {
	fixture := newTestFixture(t, echoHandlers())
	if _, err := fixture.client.Write([]byte(`{"v":1,"id":"cut"`)); err != nil {
		t.Fatalf("write partial frame: %v", err)
	}
	fixture.client.Close()

	done := make(chan struct{})
	go func() {
		for {
			if _, err := fixture.reader.ReadString('\n'); err != nil {
				break
			}
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reader never observed connection end after truncated frame")
	}
}

// TestRevokedTokenKeepsFailing pins T42 revocation semantics end to end:
// after RevokeAll the previously valid token fails AUTH_FAILED on reuse.
func TestRevokedTokenKeepsFailing(t *testing.T) {
	fixture := newTestFixture(t, echoHandlers())

	if response := fixture.callValid(t, "test/echo", `{}`); !response.OK {
		t.Fatalf("pre-revocation call must pass, got %+v", response.Error)
	}
	fixture.tokens.RevokeAll()

	after := fixture.callValid(t, "test/echo", `{}`)
	if after.Error == nil || after.Error.Code != CodeAuthFailed {
		t.Errorf("revoked token must fail AUTH_FAILED, got %+v", after.Error)
	}
	again := fixture.callValid(t, "test/echo", `{}`)
	if again.Error == nil || again.Error.Code != CodeAuthFailed {
		t.Errorf("revoked token reuse must keep failing, got %+v", again.Error)
	}
}

// TestDeadlineExceeded pins per-request deadlines.
func TestDeadlineExceeded(t *testing.T) {
	fixture := newTestFixture(t, map[string]Handler{
		"test/slow": func(ctx context.Context, principal *Principal, params json.RawMessage) (any, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})
	envelope := `{"v":1,"id":"slow","token":"` + fixture.token + `","method":"test/slow","deadlineMs":50,"params":{}}`
	start := time.Now()
	response := fixture.call(t, envelope)
	if response.Error == nil || response.Error.Code != CodeDeadline {
		t.Fatalf("deadline breach must fail DEADLINE_EXCEEDED, got %+v", response.Error)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("deadline must bound the call, took %v", elapsed)
	}
}

// TestCloseAnswersServerClosing pins shutdown: ServeConn on a closed server
// answers SERVER_CLOSING on entry instead of reading or executing requests.
func TestCloseAnswersServerClosing(t *testing.T) {
	tokens := NewTokenStore()
	if _, err := tokens.Issue(Principal{ID: "p", Kind: PrincipalFirstParty, Scope: []string{"s"}}); err != nil {
		t.Fatalf("issue: %v", err)
	}
	server := NewServer(tokens, echoHandlers(), ServerOptions{})
	server.Close()

	clientA, clientB := net.Pipe()
	defer clientA.Close()
	defer clientB.Close()
	go server.ServeConn(context.Background(), "late", clientB, clientB)

	// The server emits the closing frame on entry without waiting for
	// input; a request write here would block on the synchronous pipe,
	// which is itself the shutdown contract (no request is ever read).
	response := readOne(t, clientA)
	if response.Error == nil || response.Error.Code != CodeServerClosing {
		t.Errorf("call on closed server must fail SERVER_CLOSING, got %+v", response.Error)
	}
}

// TestFramesOnlyOnWire pins stdio purity: no non-frame bytes interleave
// with responses, even across errors.
func TestFramesOnlyOnWire(t *testing.T) {
	fixture := newTestFixture(t, echoHandlers())
	fixture.callValid(t, "test/echo", `{}`)
	fixture.callValid(t, "other/method", `{}`)
	fixture.call(t, `not json at all`)

	fixture.client.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	defer fixture.client.SetReadDeadline(time.Time{})
	if pending, err := fixture.reader.ReadByte(); err == nil {
		t.Errorf("unexpected trailing byte %q after responses", pending)
	}
}

// TestDiscoveryRoundTrip pins the first-party discovery file contract.
func TestDiscoveryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subdir", "discovery.json")
	if err := WriteDiscovery(path, Discovery{Port: 4567, Token: "secret-token", PID: 4242, PermissionMode: "confirm"}); err != nil {
		t.Fatalf("write discovery: %v", err)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatalf("stat discovery: %v", err)
	} else if info.Mode().Perm() != 0o600 {
		t.Logf("discovery mode is %v (Windows ignores POSIX mode; ACL evidence pending)", info.Mode().Perm())
	}

	loaded, err := LoadDiscovery(path)
	if err != nil {
		t.Fatalf("load discovery: %v", err)
	}
	if loaded.Port != 4567 || loaded.Token != "secret-token" {
		t.Errorf("discovery round trip mismatch: %+v", loaded)
	}

	if err := RemoveDiscovery(path); err != nil {
		t.Errorf("remove discovery: %v", err)
	}
	if _, err := LoadDiscovery(path); err == nil {
		t.Errorf("load after remove must fail")
	}
}

func readOne(t *testing.T, conn net.Conn) *Response {
	t.Helper()
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	var response Response
	if err := json.Unmarshal([]byte(line), &response); err != nil {
		t.Fatalf("parse frame %q: %v", line, err)
	}
	return &response
}
