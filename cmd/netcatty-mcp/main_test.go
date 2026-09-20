package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"testing"

	"github.com/binaricat/netcatty/internal/capability"
	"github.com/binaricat/netcatty/internal/rpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRelayUsesPublicMethodAndReportsHostErrorsWithoutPanic(t *testing.T) {
	tokens := rpc.NewTokenStore()
	token, _ := tokens.Issue(rpc.Principal{ID: "test", Kind: rpc.PrincipalExternal})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	host := rpc.NewServer(tokens, map[string]rpc.Handler{
		"public/sftp/writeFile": func(ctx context.Context, p *rpc.Principal, raw json.RawMessage) (any, error) {
			var params map[string]any
			_ = json.Unmarshal(raw, &params)
			if params["chatSessionId"] != "__external_mcp__" {
				return nil, fmt.Errorf("forged chat accepted")
			}
			return nil, &capability.DispatchError{Code: capability.CodeUserDenied, Message: "denied by test"}
		},
		"public/vault/notes/create": func(context.Context, *rpc.Principal, json.RawMessage) (any, error) {
			return map[string]any{"ok": false, "error": "vault rejected input"}, nil
		},
	}, rpc.ServerOptions{})
	defer host.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				host.ServeConn(context.Background(), conn.RemoteAddr().String(), conn, conn)
			}()
		}
	}()
	path := filepath.Join(t.TempDir(), "discovery.json")
	if err := rpc.WriteDiscovery(path, rpc.Discovery{Port: listener.Addr().(*net.TCPAddr).Port, Token: token}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"sftp.write", "vault.note.create"} {
		var spec capability.ToolSurface
		for _, tool := range capability.Default().ListMcpTools() {
			if tool.CapabilityID == id {
				spec = tool
			}
		}
		request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(`{"chatSessionId":"forged","path":"/x","content":"x"}`)}}
		result, err := relayCall(context.Background(), &relay{}, path, &spec, request)
		if err != nil || !result.IsError || len(result.Content) != 1 {
			t.Fatalf("%s: %v %v", id, result, err)
		}
	}
}
