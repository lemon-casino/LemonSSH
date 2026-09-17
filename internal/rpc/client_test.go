package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// startTestHost runs a real TCP server on 127.0.0.1 and writes its
// discovery file; returns the path for Client.Dial.
func startTestHost(t *testing.T) string {
	t.Helper()
	tokens := NewTokenStore()
	token, err := tokens.Issue(Principal{ID: "cli-principal", Kind: PrincipalFirstParty, Scope: []string{"sess-1"}})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := NewServer(tokens, map[string]Handler{
		"test/echo": func(ctx context.Context, principal *Principal, params json.RawMessage) (any, error) {
			return map[string]any{"seen": string(params), "scope": principal.Scope}, nil
		},
	}, ServerOptions{})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go server.ServeConn(context.Background(), conn.RemoteAddr().String(), conn, conn)
		}
	}()

	path := filepath.Join(t.TempDir(), "discovery.json")
	if err := WriteDiscovery(path, Discovery{Port: listener.Addr().(*net.TCPAddr).Port, Token: token, PID: os.Getpid()}); err != nil {
		t.Fatalf("write discovery: %v", err)
	}
	t.Cleanup(func() {
		server.Close()
		listener.Close()
	})
	return path
}

func TestClientRoundTripOverTCP(t *testing.T) {
	path := startTestHost(t)
	client, err := Dial(path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	result, err := client.Call(context.Background(), "test/echo", map[string]any{"sessionId": "sess-1", "command": "pwd"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var decoded struct {
		Seen  string   `json:"seen"`
		Scope []string `json:"scope"`
	}
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Scope == nil || len(decoded.Scope) != 1 || decoded.Scope[0] != "sess-1" {
		t.Errorf("handler must see the principal scope, got %+v", decoded)
	}
}

func TestClientHostErrorBecomesRPCError(t *testing.T) {
	tokens := NewTokenStore()
	token, _ := tokens.Issue(Principal{ID: "p", Kind: PrincipalFirstParty, Scope: []string{"s"}})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	server := NewServer(tokens, map[string]Handler{
		"test/deny": func(ctx context.Context, principal *Principal, params json.RawMessage) (any, error) {
			return nil, &ScopeError{Code: CodeScopeDenied, Message: "out of scope"}
		},
	}, ServerOptions{})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go server.ServeConn(context.Background(), "r", conn, conn)
		}
	}()

	path := filepath.Join(t.TempDir(), "discovery.json")
	if err := WriteDiscovery(path, Discovery{Port: listener.Addr().(*net.TCPAddr).Port, Token: token}); err != nil {
		t.Fatalf("write discovery: %v", err)
	}
	client, err := Dial(path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	_, err = client.Call(context.Background(), "test/deny", map[string]any{})
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("host refusal must surface as RPCError, got %v", err)
	}
	if rpcErr.Code != CodeScopeDenied {
		t.Errorf("host refusal code = %q, want %q", rpcErr.Code, CodeScopeDenied)
	}
}

func TestClientTypedUnavailable(t *testing.T) {
	if _, err := Dial(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("dial with missing discovery must fail")
	} else {
		var unavailable *UnavailableError
		if !errors.As(err, &unavailable) {
			t.Fatalf("missing discovery must be UnavailableError, got %v", err)
		}
		if !strings.Contains(err.Error(), "Start Netcatty first") {
			t.Errorf("unavailable message must be actionable, got %q", err.Error())
		}
	}

	// Port with no listener: refused connection is unavailable, not a crash.
	path := filepath.Join(t.TempDir(), "discovery.json")
	if err := WriteDiscovery(path, Discovery{Port: 1, Token: "x"}); err != nil {
		t.Fatalf("write discovery: %v", err)
	}
	if _, err := Dial(path); err == nil {
		t.Fatal("dial to dead port must fail")
	} else {
		var unavailable *UnavailableError
		if !errors.As(err, &unavailable) {
			t.Fatalf("refused connection must be UnavailableError, got %v", err)
		}
	}
}
