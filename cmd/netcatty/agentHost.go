package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
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
	approvals      capability.ApprovalGate
	state          *agentToolState
	vaultRouter    *AgentVaultRouter
	external       externalAgentAccess
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
	Approvals      capability.ApprovalGate
	PermissionMode string
	VaultRouter    *AgentVaultRouter
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
		approvals:      config.Approvals,
		state:          newAgentToolState(config.PermissionMode),
		vaultRouter:    config.VaultRouter,
		external:       externalAgentAccess{config: ExternalAgentConfig{Mode: "temporary", IdleTimeoutMinutes: 10, SessionIdleTimeoutMinutes: 30}, sessions: map[string]time.Time{}},
	}
}

// capabilityHandlers binds the catalog to the canonical native services and
// application-owned vault operations. Coverage tests require every served
// capability to have a handler on each declared RPC surface.
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
				"permissionMode": h.state.permissionMode(),
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
		if writer, ok := h.sftp.(SFTPWriter); ok {
			for _, id := range []string{"sftp.write", "sftp.mkdir", "sftp.delete", "sftp.rename", "sftp.chmod"} {
				handlers[id] = h.sftpMutationHandler(writer)
			}
		}
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
		// Headless reads and stop use native services. The desktop vault
		// router below supplies the canonical rule mutation/start operations.
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
	handlers["session.cancel"] = h.cancelHandler
	handlers["session.resume"] = h.cancelHandler
	if h.vaultRouter != nil {
		for _, def := range capability.Catalog {
			if def.Domain == "vault" || (def.Domain == "portforward" && def.ID != "portforward.tunnels.list") || def.ID == "session.close" {
				handlers[def.ID] = h.vaultRelayHandler
			}
		}
	}
	return handlers
}

func (h *AgentHost) cancelHandler(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
	chat, _ := params["chatSessionId"].(string)
	if chat == "" {
		return nil, fmt.Errorf("chatSessionId is required")
	}
	cancelled := true
	if value, ok := params["cancelled"].(bool); ok {
		cancelled = value
	}
	h.setChatCancelled(chat, cancelled)
	return map[string]any{"ok": true, "cancelled": cancelled}, nil
}

func (h *AgentHost) vaultRelayHandler(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
	chat, _ := params["chatSessionId"].(string)
	id, _ := params["sessionId"].(string)
	if def.ID == "session.close" {
		h.state.mu.Lock()
		owned := h.state.owned[chat][id]
		h.state.mu.Unlock()
		if !owned {
			return nil, &rpc.ScopeError{Code: rpc.CodeScopeDenied, Message: "only sessions opened by this chat can be closed"}
		}
	}
	op := strings.TrimPrefix(def.ID, "vault.")
	result, err := h.vaultRouter.Call(ctx, op, params)
	if err != nil {
		return nil, err
	}
	if result["ok"] == true {
		if def.ID == "vault.host.open" {
			opened, _ := result["sessionId"].(string)
			h.state.mu.Lock()
			if h.state.owned[chat] == nil {
				h.state.owned[chat] = map[string]bool{}
			}
			h.state.owned[chat][opened] = true
			h.state.mu.Unlock()
			if chat == externalAgentChat {
				h.touchExternalSession(opened, true)
			}
		} else if def.ID == "session.close" {
			h.state.mu.Lock()
			delete(h.state.owned[chat], id)
			h.state.mu.Unlock()
			if chat == externalAgentChat {
				h.external.mu.Lock()
				delete(h.external.sessions, id)
				h.armExternalTimerLocked()
				h.external.mu.Unlock()
			}
		}
	}
	return result, nil
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
	if err := h.requireScopedSession(chatSessionID, sessionID); err != nil {
		return nil, err
	}
	h.state.mu.Lock()
	timeout := h.state.timeout
	h.state.mu.Unlock()
	if ms, ok := params["timeoutMs"].(float64); ok {
		timeout = time.Duration(ms) * time.Millisecond
	}
	output, exitCode, known, err := h.jobs.Exec(ctx, h.state.nativeID(sessionID), chatSessionID, command, timeout)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok": true, "output": string(output), "stdout": string(output), "stderr": "",
		"exitCode": exitCode, "exitCodeKnown": known,
	}, nil
}

func (h *AgentHost) terminalStartHandler(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
	sessionID, _ := params["sessionId"].(string)
	chatSessionID, _ := params["chatSessionId"].(string)
	command, _ := params["command"].(string)
	if err := h.requireScopedSession(chatSessionID, sessionID); err != nil {
		return nil, err
	}
	jobID, err := h.jobs.Start(h.state.nativeID(sessionID), chatSessionID, command, 0)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil || h.state.isCancelled(chatSessionID) {
		_, _ = h.jobs.Stop(jobID, chatSessionID)
		return nil, context.Canceled
	}
	return map[string]any{"ok": true, "jobId": jobID, "sessionId": sessionID, "status": string(terminaluse.JobRunning)}, nil
}

func (h *AgentHost) terminalPollHandler(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
	chatSessionID, _ := params["chatSessionId"].(string)
	jobID, _ := params["jobId"].(string)
	offset := 0
	if v, ok := params["offset"].(float64); ok {
		offset = int(v)
	}
	return h.jobs.PollText(jobID, chatSessionID, offset)
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
	chat, _ := params["chatSessionId"].(string)
	all := h.state.visible(chat, h.sessions())
	if principal, _ := ctx.Value(agentPrincipalContextKey{}).(*rpc.Principal); principal != nil && len(principal.Scope) > 0 {
		filtered := []AgentSession{}
		for _, session := range all {
			if principal.AllowsSession(session.SessionID) {
				filtered = append(filtered, session)
			}
		}
		all = filtered
	}
	if sessionID != "" {
		for _, session := range all {
			if session.SessionID == sessionID {
				return map[string]any{"ok": true, "session": session}, nil
			}
		}
		return nil, rpc.RequireSession(newHostPrincipal(), sessionID)
	}
	return map[string]any{"ok": true, "sessions": all, "hosts": all}, nil
}

func (h *AgentHost) sftpListHandler(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
	sessionID, path := hostSessionAndPath(params)
	if err := h.checkSession(sessionID); err != nil {
		return nil, err
	}
	entries, err := h.sftpForContext(ctx).List(h.state.nativeID(sessionID), path)
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
	content, err := h.sftpForContext(ctx).Read(h.state.nativeID(sessionID), path)
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
	info, err := h.sftpForContext(ctx).Stat(h.state.nativeID(sessionID), path)
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
	home, err := h.sftpForContext(ctx).HomeDir(h.state.nativeID(sessionID))
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
	bytesMoved, err := h.sftpForContext(ctx).Download(h.state.nativeID(sessionID), remotePath, localPath)
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
	bytesMoved, err := h.sftpForContext(ctx).Upload(h.state.nativeID(sessionID), localPath, remotePath)
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
	h.state.mu.Lock()
	defer h.state.mu.Unlock()
	if _, ok := h.state.live[sessionID]; ok {
		return nil
	}
	for _, sessions := range h.state.scopes {
		for _, session := range sessions {
			if session.SessionID == sessionID {
				return nil
			}
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
			go func() {
				defer conn.Close()
				h.server.ServeConn(context.Background(), conn.RemoteAddr().String(), conn, conn)
			}()
		}
	}()
	return nil
}

// methodTable serves every declared alias with its own surface policy.
func (h *AgentHost) methodTable() map[string]rpc.Handler {
	registered := h.capabilityHandlers()
	table := make(map[string]rpc.Handler)
	for _, def := range capability.Default().List(capability.ListOptions{Status: capability.StatusImplemented}) {
		if registered[def.ID] == nil {
			continue
		}
		for _, surface := range []capability.Surface{capability.SurfaceBuiltin, capability.SurfaceGlobal, capability.SurfacePublic} {
			method := def.Surfaces[surface].RPCMethod
			if method == "" {
				continue
			}
			table[method] = func(ctx context.Context, principal *rpc.Principal, rawParams json.RawMessage) (any, error) {
				params := map[string]any{}
				if len(rawParams) > 0 {
					if err := json.Unmarshal(rawParams, &params); err != nil || params == nil {
						return nil, &rpc.ScopeError{Code: rpc.CodeBadRequest, Message: "tool arguments must be a JSON object"}
					}
				}
				chat, _ := params["chatSessionId"].(string)
				if principal.Kind == rpc.PrincipalExternal || chat == "" {
					chat = "__external_mcp__"
				}
				return h.dispatch(ctx, method, params, chat, principal)
			}
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
	_, _ = h.setExternalEnabled(false)
	h.external.mu.Lock()
	if h.external.timer != nil {
		h.external.timer.Stop()
	}
	h.external.generation++
	h.external.sessions = map[string]time.Time{}
	h.external.mu.Unlock()
	h.state.mu.Lock()
	for _, call := range h.state.active {
		call.cancel()
	}
	h.state.mu.Unlock()
	if h.jobs != nil {
		h.jobs.CancelAll()
	}
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
