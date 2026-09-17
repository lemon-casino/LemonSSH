// Command netcatty-mcp is the native MCP stdio server (W07, P7-02, AI-02).
// It projects the capability catalog's MCP surface (internal/capability)
// as tools and relays tools/call to the running Netcatty host over the
// authenticated local RPC (internal/rpc). It holds no policy copy: every
// authorization decision belongs to the host. When the app is not running,
// calls fail with a typed unavailable message instead of crashing the
// server.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/binaricat/netcatty/internal/capability"
	"github.com/binaricat/netcatty/internal/rpc"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const serverVersion = "0.1.0"

type relay struct {
	mu     sync.Mutex
	client *rpc.Client
}

// host returns a connected client, dialing lazily and re-dialing after a
// lost connection.
func (r *relay) host(discoveryPath string) (*rpc.Client, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client != nil {
		return r.client, nil
	}
	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		return nil, err
	}
	r.client = client
	return r.client, nil
}

func (r *relay) drop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client != nil {
		_ = r.client.Close()
		r.client = nil
	}
}

func main() {
	discoveryPath := os.Getenv("NETCATTY_TOOL_CLI_DISCOVERY_FILE")
	registry := capability.Default()
	tools := registry.ListMcpTools()

	sort.Slice(tools, func(i, j int) bool { return tools[i].ToolName < tools[j].ToolName })
	server := mcp.NewServer(&mcp.Implementation{Name: "netcatty", Version: serverVersion}, nil)
	r := &relay{}
	for _, tool := range tools {
		def := tool
		server.AddTool(&mcp.Tool{
			Name:        *def.MCPTool,
			Description: def.Description,
			InputSchema: json.RawMessage(buildInputSchema(def.InputShape)),
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return relayCall(ctx, r, discoveryPath, &def, req)
		})
	}

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "netcatty-mcp: server failed: %v\n", err)
		os.Exit(1)
	}
}

// relayCall relays one tool call to the host. Host-reported failures become
// isError content carrying the host code and message; an unreachable host
// becomes typed unavailable — both observable, neither silent.
func relayCall(ctx context.Context, r *relay, discoveryPath string, tool *capability.ToolSurface, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if tool.RPCMethod == nil {
		return errorResult(fmt.Sprintf("Error: tool %q has no host rpc binding", tool.ToolName)), nil
	}

	var (
		result  json.RawMessage
		callErr error
	)
	client, dialErr := r.host(discoveryPath)
	if dialErr == nil {
		result, callErr = client.Call(ctx, *tool.RPCMethod, req.Params.Arguments)
		if callErr != nil {
			r.drop()
		}
	}

	if dialErr != nil || callErr != nil {
		message := dialErr.Error()
		var rpcErr *rpc.RPCError
		var unavailable *rpc.UnavailableError
		switch {
		case dialErr != nil:
			// message already set
		case errors.As(callErr, &unavailable):
			message = unavailable.Message
		case errors.As(callErr, &rpcErr):
			message = fmt.Sprintf("Error: %s", rpcErr.Message)
		default:
			message = "Error: Operation failed"
		}
		return errorResult(message), nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(result)}},
	}, nil
}

func errorResult(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
		IsError: true,
	}
}

// buildInputSchema renders the catalog input shape as a 2020-12 JSON
// Schema object with required derived from non-optional fields.
func buildInputSchema(fields map[string]capability.ToolInputField) string {
	schema := map[string]any{"type": "object"}
	if len(fields) == 0 {
		raw, _ := json.Marshal(schema)
		return string(raw)
	}

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
	schema["properties"] = properties
	if len(required) > 0 {
		sort.Strings(required)
		schema["required"] = required
	}
	raw, _ := json.Marshal(schema)
	return string(raw)
}
