package main

import (
	"context"
	"encoding/json"
	"net"
	"os"

	"github.com/binaricat/netcatty/internal/rpc"
)

// SessionEntry is one terminal session exposed to agent context queries.
type SessionEntry struct {
	ID     string `json:"id"`
	Label  string `json:"label,omitempty"`
	Kind   string `json:"kind,omitempty"`
	HostID string `json:"hostId,omitempty"`
}

// AgentHost serves the authenticated local RPC surface the native CLI/MCP
// binaries connect to (W13, P7-01/P7-02). Handlers are registered per
// capability method and every call crosses the capability policy layer, so
// the host holds no parallel authorization.
type AgentHost struct {
	listener      net.Listener
	tokens        *rpc.TokenStore
	server        *rpc.Server
	token         string
	discoveryPath string

	permissionMode string
	version        appVersion
	sessions       func() []SessionEntry
}

type appVersion struct {
	Name    string
	Version string
	GOOS    string
	GOARCH  string
}

// AgentHostConfig wires the host's data sources.
type AgentHostConfig struct {
	Version        appVersion
	Sessions       func() []SessionEntry
	PermissionMode string
}

func newAgentHost(config AgentHostConfig) *AgentHost {
	if config.Sessions == nil {
		config.Sessions = func() []SessionEntry { return nil }
	}
	if config.PermissionMode == "" {
		config.PermissionMode = "confirm"
	}
	return &AgentHost{
		permissionMode: config.PermissionMode,
		version:        config.Version,
		sessions:       config.Sessions,
	}
}

// handlers builds the capability method table. Registration is the W13
// domain-by-domain expansion point: one handler per catalog method, each
// crossing the same policy layer.
func (h *AgentHost) handlers() map[string]rpc.Handler {
	return map[string]rpc.Handler{
		"netcatty/getStatus": func(ctx context.Context, principal *rpc.Principal, params json.RawMessage) (any, error) {
			return map[string]any{
				"ok":             true,
				"name":           h.version.Name,
				"version":        h.version.Version,
				"goos":           h.version.GOOS,
				"goarch":         h.version.GOARCH,
				"pid":            os.Getpid(),
				"permissionMode": h.permissionMode,
			}, nil
		},
		"netcatty/getContext": func(ctx context.Context, principal *rpc.Principal, params json.RawMessage) (any, error) {
			all := h.sessions()
			inScope := make([]SessionEntry, 0, len(all))
			for _, session := range all {
				// An empty principal scope grants everything (host-decided);
				// a populated scope filters to the granted sessions.
				if len(principal.Scope) == 0 || principal.AllowsSession(session.ID) {
					inScope = append(inScope, session)
				}
			}
			return map[string]any{
				"ok":       true,
				"sessions": inScope,
			}, nil
		},
	}
}

// Start opens the loopback listener, issues the first-party token, writes
// the discovery file and serves connections until Stop.
func (h *AgentHost) Start(discoveryPath string) error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	h.listener = listener
	h.tokens = rpc.NewTokenStore()
	token, err := h.tokens.Issue(rpc.Principal{
		ID:   "first-party-cli",
		Kind: rpc.PrincipalFirstParty,
		// The host resolves session scope per call from its own list; a
		// nil scope grants the host-reported sessions.
		Scope: nil,
	})
	if err != nil {
		listener.Close()
		return err
	}
	h.token = token

	discovery := rpc.Discovery{
		Port:           listener.Addr().(*net.TCPAddr).Port,
		Token:          token,
		PID:            os.Getpid(),
		PermissionMode: h.permissionMode,
	}
	if err := rpc.WriteDiscovery(discoveryPath, discovery); err != nil {
		listener.Close()
		return err
	}
	h.discoveryPath = discoveryPath

	h.server = rpc.NewServer(h.tokens, h.handlers(), rpc.ServerOptions{})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go h.server.ServeConn(context.Background(), conn.RemoteAddr().String(), conn, conn)
		}
	}()
	return nil
}

// Stop revokes every token, removes the discovery file and closes the
// listener; in-flight ServeConn loops observe the server-closed state on
// their next frame. Stale launchers then fail with the typed unavailable
// message instead of dialing a dead port.
func (h *AgentHost) Stop() {
	if h.discoveryPath != "" {
		_ = rpc.RemoveDiscovery(h.discoveryPath)
		h.discoveryPath = ""
	}
	if h.server != nil {
		h.server.Close()
	}
	if h.listener != nil {
		h.listener.Close()
	}
	h.token = ""
}
