package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"

	"github.com/binaricat/netcatty/internal/capability"
	"github.com/binaricat/netcatty/internal/rpc"
	"github.com/binaricat/netcatty/internal/terminal/sftp"
)

// SessionEntry is one terminal session exposed to agent context queries.
type SessionEntry struct {
	ID     string `json:"id"`
	Label  string `json:"label,omitempty"`
	Kind   string `json:"kind,omitempty"`
	HostID string `json:"hostId,omitempty"`
}

// SFTPReader is the read-only SFTP surface the host exposes to agents,
// satisfied by the shared sftpuse.Service — no second SFTP stack (W04).
type SFTPReader interface {
	List(sessionID, dir string) ([]sftp.Entry, error)
	Stat(sessionID, target string) (sftp.FileInfo, error)
	Read(sessionID, remotePath string) (string, error)
	HomeDir(sessionID string) (string, error)
}

// AgentHost serves the authenticated local RPC surface the native CLI/MCP
// binaries connect to (W13, P7-01/P7-02). Every call crosses the
// capability policy layer via the W05 dispatcher — the host holds no
// parallel authorization.
type AgentHost struct {
	listener       net.Listener
	tokens         *rpc.TokenStore
	server         *rpc.Server
	token          string
	discoveryPath  string
	permissionMode string
	version        appVersion
	sessions       func() []SessionEntry
	sftp           SFTPReader
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
	SFTP           SFTPReader
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
		sftp:           config.SFTP,
	}
}

// capabilityHandlers registers host handlers by capability ID. This map is
// the W13 domain-by-domain expansion point: an unregistered capability
// fails closed with HANDLER_MISSING at dispatch.
func (h *AgentHost) capabilityHandlers() map[string]capability.Handler {
	handlers := map[string]capability.Handler{
		"meta.status": func(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
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
		// session.environment and session.get share netcatty/getContext;
		// without a session id the handler lists the scoped sessions,
		// with one it returns that session's metadata.
		"session.environment": h.sessionContextHandler,
		"session.get":         h.sessionContextHandler,
	}
	if h.sftp != nil {
		handlers["sftp.list"] = h.sftpListHandler
		handlers["sftp.read"] = h.sftpReadHandler
		handlers["sftp.stat"] = h.sftpStatHandler
		handlers["sftp.home"] = h.sftpHomeHandler
	}
	// Terminal writes advertise but fail closed until the real job queue
	// lands: dispatch demands approval first, and with no approval gate
	// wired the request never reaches this handler. Reaching it at all is
	// a dispatch invariant violation.
	handlers["terminal.execute"] = func(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
		return nil, errors.New("terminal.execute reached its handler without an approval gate")
	}
	return handlers
}

func (h *AgentHost) sessionContextHandler(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
	sessionID, _ := params["sessionId"].(string)
	all := h.sessions()
	if sessionID != "" {
		for _, session := range all {
			if session.ID == sessionID {
				return map[string]any{"ok": true, "session": session}, nil
			}
		}
		return nil, rpc.RequireSession(newHostPrincipal(), sessionID)
	}
	return map[string]any{"ok": true, "sessions": all}, nil
}

func (h *AgentHost) sftpListHandler(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
	sessionID, path := hostSessionAndPath(params)
	if err := h.checkSession(sessionID); err != nil {
		return nil, err
	}
	entries, err := h.sftp.List(sessionID, path)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "entries": entries}, nil
}

func (h *AgentHost) sftpReadHandler(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
	sessionID, path := hostSessionAndPath(params)
	if err := h.checkSession(sessionID); err != nil {
		return nil, err
	}
	content, err := h.sftp.Read(sessionID, path)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "content": content}, nil
}

func (h *AgentHost) sftpStatHandler(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
	sessionID, path := hostSessionAndPath(params)
	if err := h.checkSession(sessionID); err != nil {
		return nil, err
	}
	info, err := h.sftp.Stat(sessionID, path)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "file": info}, nil
}

func (h *AgentHost) sftpHomeHandler(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
	sessionID, _ := params["sessionId"].(string)
	if err := h.checkSession(sessionID); err != nil {
		return nil, err
	}
	home, err := h.sftp.HomeDir(sessionID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "home": home}, nil
}

// checkSession validates that the requested session exists in the
// host-reported list (the host is the scope owner).
func (h *AgentHost) checkSession(sessionID string) error {
	if sessionID == "" {
		return rpc.RequireSession(newHostPrincipal(), "")
	}
	for _, session := range h.sessions() {
		if session.ID == sessionID {
			return nil
		}
	}
	return rpc.RequireSession(newHostPrincipal(), sessionID)
}

// hostSessionAndPath extracts the two common SFTP request parameters.
func hostSessionAndPath(params map[string]any) (sessionID, path string) {
	sessionID, _ = params["sessionId"].(string)
	path, _ = params["path"].(string)
	return sessionID, path
}

// newHostPrincipal is the host-list-scoped principal used by session
// checks; the real per-principal narrowing lands with token scopes in a
// later slice.
func newHostPrincipal() *rpc.Principal {
	return &rpc.Principal{ID: "host", Kind: rpc.PrincipalFirstParty, Scope: []string{"*"}}
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
		// The host resolves session scope per call from its own list.
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

	h.server = rpc.NewServer(h.tokens, h.methodTable(), rpc.ServerOptions{})
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

// methodTable adapts capability handlers into RPC method handlers: every
// implemented capability with a builtin method AND a registered host
// handler is served through the W05 dispatcher (policy + approval +
// fail-closed), everything else stays UNKNOWN_METHOD.
func (h *AgentHost) methodTable() map[string]rpc.Handler {
	dispatcher := &capability.Dispatcher{
		Registry: capability.Default(),
		Surface:  capability.SurfaceBuiltin,
		Handlers: h.capabilityHandlers(),
	}
	registered := h.capabilityHandlers()
	table := make(map[string]rpc.Handler)
	for _, def := range capability.Default().List(capability.ListOptions{Status: capability.StatusImplemented}) {
		if _, handled := registered[def.ID]; !handled {
			continue
		}
		binding, ok := def.Surfaces[capability.SurfaceBuiltin]
		if !ok || binding.RPCMethod == "" {
			continue
		}
		method := binding.RPCMethod
		table[method] = func(ctx context.Context, principal *rpc.Principal, rawParams json.RawMessage) (any, error) {
			var params map[string]any
			if len(rawParams) > 0 {
				if err := json.Unmarshal(rawParams, &params); err != nil {
					params = nil
				}
			}
			return dispatcher.Dispatch(ctx, method, params)
		}
	}
	return table
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
