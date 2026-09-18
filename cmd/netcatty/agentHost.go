package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/binaricat/netcatty/internal/app/terminaluse"
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
	// Download copies a remote file to a local path, returning bytes moved.
	Download(sessionID, remotePath, localPath string) (int64, error)
	// Upload copies a local file to a remote path, returning bytes moved.
	Upload(sessionID, localPath, remotePath string) (int64, error)
}

// AgentHost serves the authenticated local RPC surface the native CLI/MCP
// binaries connect to (W13, P7-01/P7-02). Every call crosses the
// capability policy layer via the W05 dispatcher — the host holds no
// parallel authorization.
type AgentHost struct {
	listener       net.Listener
	jobs           *terminaluse.JobQueue
	tokens         *rpc.TokenStore
	server         *rpc.Server
	token          string
	discoveryPath  string
	permissionMode string
	version        appVersion
	sessions       func() []SessionEntry
	sftp           SFTPReader
	vault          *VaultReader
	attachments    *AttachmentRegistry
	forwards       *ForwardService
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
	Jobs           *terminaluse.JobQueue
	Vault          *VaultReader
	Attachments    *AttachmentRegistry
	Forwards       *ForwardService
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
		jobs:           config.Jobs,
		vault:          config.Vault,
		attachments:    config.Attachments,
		forwards:       config.Forwards,
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
		// Transfers: write-capable, long-running; confirm mode demands
		// approval first (fail-closed without a gate), auto mode runs them
		// through the shared sftpuse path.
		handlers["sftp.download"] = h.sftpDownloadHandler
		handlers["sftp.upload"] = h.sftpUploadHandler
	}
	if h.jobs != nil {
		// Terminal write operations route through the job queue. In
		// confirm mode they demand approval first (fail-closed without a
		// gate); auto mode reaches the handlers directly.
		handlers["terminal.execute"] = h.terminalExecHandler
		handlers["terminal.start"] = h.terminalStartHandler
		handlers["terminal.poll"] = h.terminalPollHandler
		handlers["terminal.stop"] = h.terminalStopHandler
	} else {
		// Without a job queue the write still advertises but fails closed:
		// dispatch demands approval first, and with no approval gate wired
		// the request never reaches this handler.
		handlers["terminal.execute"] = func(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
			return nil, errors.New("terminal.execute reached its handler without an approval gate")
		}
	}
	if h.attachments != nil {
		handlers["attachment.list"] = h.attachmentListHandler
		handlers["attachment.read"] = h.attachmentReadHandler
	}
	if h.forwards != nil {
		// Reads list persisted rules (from the vault store) and live
		// tunnels (from the shared forwarduse). Start stays HANDLER_MISSING
		// until the canonical host connect command exists; stop goes
		// through forwarduse StopByRuleId (confirm mode demands approval).
		handlers["portforward.rules.list"] = func(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
			rules, err := h.vault.PortForwardingRules()
			if err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "rules": rules}, nil
		}
		handlers["portforward.tunnels.list"] = func(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
			return map[string]any{"ok": true, "tunnels": h.forwards.List()}, nil
		}
		handlers["portforward.stop"] = func(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
			ruleID, _ := params["ruleId"].(string)
			if ruleID == "" {
				return nil, fmt.Errorf("ruleId is required")
			}
			return h.forwards.StopByRuleId(ruleID), nil
		}
	}
	if h.vault != nil {
		// Vault reads serve metadata only: secret fields are redacted at
		// the reader boundary (capability contract).
		handlers["vault.host.list"] = func(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
			hosts, err := h.vault.readList(vaultStorageKeys["hosts"])
			if err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "hosts": hosts}, nil
		}
		handlers["vault.host.get"] = h.vaultGetHandler("hosts", "hostId", "host")
		handlers["vault.host.notes.get"] = func(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
			hostID, _ := params["hostId"].(string)
			host, err := h.vault.findByID(vaultStorageKeys["hosts"], hostID)
			if err != nil {
				return nil, err
			}
			if host == nil {
				return nil, fmt.Errorf("host %q not found", hostID)
			}
			notes, _ := host.(map[string]any)["notes"]
			return map[string]any{"ok": true, "notes": notes}, nil
		}
		handlers["vault.note.list"] = func(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
			notes, err := h.vault.readList(vaultStorageKeys["notes"])
			if err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "notes": notes}, nil
		}
		handlers["vault.note.get"] = h.vaultGetHandler("notes", "noteId", "note")
		handlers["vault.identity.list"] = func(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
			identities, err := h.vault.readList(vaultStorageKeys["identities"])
			if err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "identities": identities}, nil
		}
		handlers["vault.proxyProfile.list"] = func(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
			profiles, err := h.vault.readList(vaultStorageKeys["proxyProfiles"])
			if err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "proxyProfiles": profiles}, nil
		}
		handlers["vault.group.list"] = func(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
			groups, err := h.vault.readList(vaultStorageKeys["groups"])
			if err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "groups": groups}, nil
		}
		handlers["vault.snippets.list"] = func(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
			all, err := h.vault.readList(vaultStorageKeys["snippets"])
			if err != nil {
				return nil, err
			}
			plain := make([]any, 0, len(all))
			for _, item := range all {
				if obj, ok := item.(map[string]any); ok && !scriptIs(obj, true) {
					plain = append(plain, obj)
				}
			}
			return map[string]any{"ok": true, "snippets": plain}, nil
		}
		handlers["vault.snippets.get"] = h.vaultGetHandler("snippets", "snippetId", "snippet")
		handlers["vault.scripts.list"] = func(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
			all, err := h.vault.readList(vaultStorageKeys["snippets"])
			if err != nil {
				return nil, err
			}
			scripts := make([]any, 0, len(all))
			for _, item := range all {
				if obj, ok := item.(map[string]any); ok && scriptIs(obj, true) {
					scripts = append(scripts, obj)
				}
			}
			return map[string]any{"ok": true, "scripts": scripts}, nil
		}
		handlers["vault.scripts.get"] = h.vaultGetHandler("snippets", "scriptId", "script")
	}
	return handlers
}

// vaultGetHandler builds an id-lookup handler over one vault key.
func (h *AgentHost) vaultGetHandler(vaultKey, paramField, resultField string) capability.Handler {
	return func(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
		id, _ := params[paramField].(string)
		item, err := h.vault.findByID(vaultStorageKeys[vaultKey], id)
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, fmt.Errorf("%s %q not found", vaultKey, id)
		}
		return map[string]any{"ok": true, resultField: item}, nil
	}
}

func (h *AgentHost) terminalExecHandler(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
	sessionID, _ := params["sessionId"].(string)
	chatSessionID, _ := params["chatSessionId"].(string)
	command, _ := params["command"].(string)
	timeout := time.Duration(0)
	if ms, ok := params["timeoutMs"].(float64); ok {
		timeout = time.Duration(ms) * time.Millisecond
	}
	output, exitCode, known, err := h.jobs.Exec(ctx, sessionID, chatSessionID, command, timeout)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok": true, "output": string(output),
		"exitCode": exitCode, "exitCodeKnown": known,
	}, nil
}

func (h *AgentHost) terminalStartHandler(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
	sessionID, _ := params["sessionId"].(string)
	chatSessionID, _ := params["chatSessionId"].(string)
	command, _ := params["command"].(string)
	jobID, err := h.jobs.Start(sessionID, chatSessionID, command, 0)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "jobId": jobID, "status": string(terminaluse.JobRunning)}, nil
}

func (h *AgentHost) terminalPollHandler(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
	chatSessionID, _ := params["chatSessionId"].(string)
	jobID, _ := params["jobId"].(string)
	offset := 0
	if v, ok := params["offset"].(float64); ok {
		offset = int(v)
	}
	output, status, exitCode, known, err := h.jobs.Poll(jobID, chatSessionID, offset)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"ok": true, "jobId": jobID, "status": string(status),
		"output":        string(output),
		"exitCodeKnown": known,
	}
	if known {
		payload["exitCode"] = exitCode
	}
	return payload, nil
}

func (h *AgentHost) terminalStopHandler(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
	chatSessionID, _ := params["chatSessionId"].(string)
	jobID, _ := params["jobId"].(string)
	status, err := h.jobs.Stop(jobID, chatSessionID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "jobId": jobID, "status": string(status)}, nil
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

func (h *AgentHost) sftpDownloadHandler(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
	sessionID, _ := params["sessionId"].(string)
	remotePath, _ := params["remotePath"].(string)
	localPath, _ := params["localPath"].(string)
	if err := h.checkSession(sessionID); err != nil {
		return nil, err
	}
	if remotePath == "" || localPath == "" {
		return nil, fmt.Errorf("remotePath and localPath are required")
	}
	bytesMoved, err := h.sftp.Download(sessionID, remotePath, localPath)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "bytes": bytesMoved, "localPath": localPath}, nil
}

func (h *AgentHost) sftpUploadHandler(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
	sessionID, _ := params["sessionId"].(string)
	remotePath, _ := params["remotePath"].(string)
	localPath, _ := params["localPath"].(string)
	if err := h.checkSession(sessionID); err != nil {
		return nil, err
	}
	if localPath == "" || remotePath == "" {
		return nil, fmt.Errorf("localPath and remotePath are required")
	}
	bytesMoved, err := h.sftp.Upload(sessionID, localPath, remotePath)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "bytes": bytesMoved, "remotePath": remotePath}, nil
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

func permissionModeOf(mode string) capability.PermissionMode {
	switch capability.PermissionMode(mode) {
	case capability.ModeObserver, capability.ModeAuto:
		return capability.PermissionMode(mode)
	default:
		return capability.ModeConfirm
	}
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

	// Long-running transfers and jobs need more than the 30s default;
	// clients may still request shorter deadlines per call.
	h.server = rpc.NewServer(h.tokens, h.methodTable(), rpc.ServerOptions{
		DefaultDeadline: 30 * time.Second,
		MaxDeadline:     10 * time.Minute,
	})
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
// implemented capability with a served method (builtin, then global, then
// public) AND a registered host handler is dispatched on that surface's
// policy — builtin/global/public each carry their own binding semantics —
// everything else stays UNKNOWN_METHOD.
func (h *AgentHost) methodTable() map[string]rpc.Handler {
	registered := h.capabilityHandlers()
	newDispatcher := func(surface capability.Surface) *capability.Dispatcher {
		return &capability.Dispatcher{
			Registry:       capability.Default(),
			Surface:        surface,
			PermissionMode: permissionModeOf(h.permissionMode),
			Handlers:       registered,
		}
	}
	surfaces := map[capability.Surface]*capability.Dispatcher{
		capability.SurfaceBuiltin: newDispatcher(capability.SurfaceBuiltin),
		capability.SurfaceGlobal:  newDispatcher(capability.SurfaceGlobal),
		capability.SurfacePublic:  newDispatcher(capability.SurfacePublic),
	}

	table := make(map[string]rpc.Handler)
	for _, def := range capability.Default().List(capability.ListOptions{Status: capability.StatusImplemented}) {
		if _, handled := registered[def.ID]; !handled {
			continue
		}
		surface, method := servedMethod(def)
		if method == "" {
			continue
		}
		dispatcher := surfaces[surface]
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

// servedMethod resolves the method an agent calls for one capability and
// the surface that method belongs to (builtin, then global, then public).
func servedMethod(def *capability.Definition) (capability.Surface, string) {
	for _, surface := range []capability.Surface{capability.SurfaceBuiltin, capability.SurfaceGlobal, capability.SurfacePublic} {
		if binding, ok := def.Surfaces[surface]; ok && binding.RPCMethod != "" {
			return surface, binding.RPCMethod
		}
	}
	return "", ""
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
