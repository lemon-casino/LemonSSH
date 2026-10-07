package main

// Plugin-protocol connection sessions, importer providers and authentication
// challenges: the second half of the extension provider data plane. See
// pluginExtensionService.go for the shared dispatch plumbing and the
// security model.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lemon-casino/lemonssh/internal/app/terminaluse"
	"github.com/lemon-casino/lemonssh/internal/plugin/providers"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ---------------------------------------------------------------------------
// Connection providers
// ---------------------------------------------------------------------------

// ProviderDiagnostic mirrors the contract ProviderValidationIssue.
type ProviderDiagnostic struct {
	Path     string `json:"path,omitempty"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// PluginConnectionStartRequest is the renderer's start payload
// (LemonSSHBridge.startPluginConnection). SessionLog is accepted for
// compatibility; session logs flow through the ordinary output observer.
type PluginConnectionStartRequest struct {
	RequestID                string         `json:"requestId,omitempty"`
	SessionID                string         `json:"sessionId"`
	Protocol                 string         `json:"protocol,omitempty"`
	HostLabel                string         `json:"hostLabel,omitempty"`
	Hostname                 string         `json:"hostname,omitempty"`
	ProviderID               string         `json:"providerId"`
	Configuration            map[string]any `json:"configuration,omitempty"`
	Columns                  int            `json:"columns"`
	Rows                     int            `json:"rows"`
	Credential               map[string]any `json:"credential,omitempty"`
	AuthenticationProviderID string         `json:"authenticationProviderId,omitempty"`
	DeadlineMs               int            `json:"deadlineMs,omitempty"`
}

// PluginConnectionStartResult is the renderer's start answer.
type PluginConnectionStartResult struct {
	SessionID   string               `json:"sessionId"`
	ProviderID  string               `json:"providerId"`
	Status      string               `json:"status"`
	Diagnostics []ProviderDiagnostic `json:"diagnostics"`
}

// connectionOpenResult mirrors the contract ConnectionOpenResult.
type connectionOpenResult struct {
	ConnectionID string               `json:"connectionId"`
	Status       string               `json:"status"`
	Diagnostics  []ProviderDiagnostic `json:"diagnostics,omitempty"`
}

// connectionOutputResult is the wire shape of the readOutput operation.
type connectionOutputResult struct {
	Encoding string `json:"encoding"`
	Data     string `json:"data"`
	Closed   bool   `json:"closed"`
	Reason   string `json:"reason"`
}

// StartPluginConnection opens a plugin-protocol connection and hosts it as a
// first class terminal session: the renderer attaches through the ordinary
// data-plane bootstrap, and Write/Resize/Signal/Close route back into the
// plugin via provider.invoke operations.
func (s *PluginService) StartPluginConnection(request PluginConnectionStartRequest) (*PluginConnectionStartResult, error) {
	if strings.TrimSpace(request.SessionID) == "" {
		return nil, errors.New("sessionId is required")
	}
	contribution, err := s.resolveExtensionProvider(providers.KindConnection, request.ProviderID)
	if err != nil {
		return nil, err
	}
	sink := s.ext.sessions
	if sink == nil {
		return nil, errors.New("terminal session sink is unavailable for plugin connections")
	}
	s.ext.mu.Lock()
	if len(s.ext.connections) >= maxActiveConnections {
		s.ext.mu.Unlock()
		return nil, errors.New("too many active plugin connections")
	}
	if _, exists := s.ext.connections[request.SessionID]; exists {
		s.ext.mu.Unlock()
		return nil, fmt.Errorf("plugin connection session %q already exists", request.SessionID)
	}
	s.ext.mu.Unlock()

	requestID := orMintRequestID(request.RequestID)
	credential, err := s.ext.resolveConnectionCredential(contribution.PluginID, request.Credential)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"configuration": request.Configuration,
		"operationId":   requestID,
		"columns":       request.Columns,
		"rows":          request.Rows,
	}
	if credential != nil {
		payload["credential"] = credential
	}
	if request.AuthenticationProviderID != "" {
		payload["authenticationProviderId"] = request.AuthenticationProviderID
	}
	result, err := s.ext.invokeExtension(context.Background(), extensionInvocation{
		pluginID:   contribution.PluginID,
		providerID: contribution.Provider.ID,
		kind:       providers.KindConnection,
		operation:  opConnOpen,
		requestID:  requestID,
		deadline:   clampDispatchDeadline(request.DeadlineMs),
		payload:    payload,
	})
	if err != nil {
		return nil, err
	}
	var opened connectionOpenResult
	if err := json.Unmarshal(result, &opened); err != nil || opened.ConnectionID == "" {
		return nil, errors.New("plugin connection open returned no connectionId")
	}
	if opened.Status == "" {
		opened.Status = "connecting"
	}

	conn := &pluginConnection{
		sessionID:  request.SessionID,
		requestID:  requestID,
		pluginID:   contribution.PluginID,
		providerID: contribution.Provider.ID,
		start:      request,
		done:       make(chan struct{}),
	}
	conn.setConnectionTarget(opened.ConnectionID)
	connCtx, cancel := context.WithCancel(context.Background())
	conn.cancel = cancel

	label := request.HostLabel
	if label == "" {
		label = request.Hostname
	}
	if label == "" {
		label = request.ProviderID
	}
	hooks := terminaluse.PluginSessionHooks{
		Write: func(data []byte) (int, error) {
			return s.ext.writeConnectionInput(conn, data)
		},
		Resize: func(cols, rows uint16) error {
			return s.ext.controlConnectionOperation(conn, opConnResize, map[string]any{
				"connectionId": conn.connectionTarget(),
				"columns":      int(cols),
				"rows":         int(rows),
			})
		},
		Signal: func(signal string) error {
			return s.ext.controlConnectionOperation(conn, opConnSignal, map[string]any{
				"connectionId": conn.connectionTarget(),
				"signal":       signal,
			})
		},
		Close: func(reason string) {
			s.ext.teardownConnection(conn, reason)
		},
	}
	if err := sink.RegisterPluginSession(request.SessionID, label, hooks); err != nil {
		cancel()
		// The plugin already opened a connection — close it best-effort.
		s.ext.dispatchCloseOperation(conn)
		return nil, fmt.Errorf("register plugin connection session: %w", err)
	}
	s.ext.mu.Lock()
	s.ext.connections[request.SessionID] = conn
	s.ext.mu.Unlock()
	go s.ext.runConnectionOutputLoop(connCtx, conn)

	diagnostics := opened.Diagnostics
	if diagnostics == nil {
		diagnostics = []ProviderDiagnostic{}
	}
	return &PluginConnectionStartResult{
		SessionID:   request.SessionID,
		ProviderID:  contribution.Provider.ID,
		Status:      opened.Status,
		Diagnostics: diagnostics,
	}, nil
}

// resolveConnectionCredential resolves SecretRef/CredentialRef connection
// credentials. Secrets resolve against the owning plugin of the referenced
// provider id (any extension kind).
func (h *pluginExtensionHost) resolveConnectionCredential(defaultPluginID string, credential map[string]any) (map[string]any, error) {
	if len(credential) == 0 {
		return nil, nil
	}
	switch credential["kind"] {
	case "credential":
		return credential, nil
	case "secret":
		key, _ := credential["key"].(string)
		refID, _ := credential["id"].(string)
		if key == "" {
			return nil, errors.New("secret credential ref requires a key")
		}
		pluginID := h.ownerOfProvider(refID)
		if pluginID == "" {
			pluginID = defaultPluginID
		}
		value, err := h.loadSyncSecret(pluginID, key)
		if err != nil {
			return nil, fmt.Errorf("resolve connection secret %q: %w", key, err)
		}
		return map[string]any{"kind": "secret", "id": refID, "key": key, "value": value}, nil
	default:
		return nil, fmt.Errorf("unsupported connection credential kind %q", fmt.Sprint(credential["kind"]))
	}
}

// ownerOfProvider scans every extension kind for the provider id.
func (h *pluginExtensionHost) ownerOfProvider(providerID string) string {
	for kind := range providers.ExtensionKinds() {
		contributions := h.service.providers.List(context.Background(), kind, "")
		for index := range contributions {
			if contributions[index].Provider.ID == providerID {
				return contributions[index].PluginID
			}
		}
	}
	return ""
}

// writeConnectionInput forwards renderer stdin to the plugin connection.
func (h *pluginExtensionHost) writeConnectionInput(conn *pluginConnection, data []byte) (int, error) {
	if len(data) > maxSyncChunkBytes {
		return 0, errors.New("plugin connection input chunk exceeds the dispatch bound")
	}
	if conn.isClosed() {
		return 0, errors.New("plugin connection is closed")
	}
	err := h.controlConnectionOperation(conn, opConnWriteInput, map[string]any{
		"connectionId": conn.connectionTarget(),
		"encoding":     "base64",
		"data":         base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

// controlConnectionOperation dispatches one connection control operation
// against the connection's owning plugin.
func (h *pluginExtensionHost) controlConnectionOperation(conn *pluginConnection, operation string, payload map[string]any) error {
	if conn.isClosed() {
		return errors.New("plugin connection is closed")
	}
	if payload != nil {
		if _, has := payload["connectionId"]; has {
			payload["connectionId"] = conn.connectionTarget()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), clampDispatchDeadline(0))
	defer cancel()
	_, err := h.invokeExtension(ctx, extensionInvocation{
		pluginID:   conn.pluginID,
		providerID: conn.providerID,
		kind:       providers.KindConnection,
		operation:  operation,
		requestID:  conn.sessionID,
		deadline:   clampDispatchDeadline(0),
		payload:    payload,
	})
	return err
}

func (h *pluginExtensionHost) dispatchCloseOperation(conn *pluginConnection) {
	ctx, cancel := context.WithTimeout(context.Background(), closeDispatchDeadline)
	defer cancel()
	_, _ = h.invokeExtension(ctx, extensionInvocation{
		pluginID:   conn.pluginID,
		providerID: conn.providerID,
		kind:       providers.KindConnection,
		operation:  opConnClose,
		requestID:  conn.sessionID,
		deadline:   closeDispatchDeadline,
		payload:    map[string]any{"connectionId": conn.connectionTarget()},
	})
}

// teardownConnection is the renderer-initiated close path (terminal session
// close hook or a request cancel): stop the poller, tell the plugin, drop
// host state. It never waits for conn.done — the output loop closes it on
// its own return once it observes the cancelled context, and this function
// may run ON the loop's call stack (terminaluse close → hook). Losers of
// beginClose return immediately so the re-entrant hook chain
// (PluginSessionClosed → finishSessionClose → hooks.Close) can never block.
func (h *pluginExtensionHost) teardownConnection(conn *pluginConnection, reason string) {
	if !conn.beginClose() {
		return
	}
	if reason == "" {
		reason = "closed"
	}
	if conn.cancel != nil {
		conn.cancel()
	}
	h.mu.Lock()
	if current, ok := h.connections[conn.sessionID]; ok && current == conn {
		delete(h.connections, conn.sessionID)
	}
	h.mu.Unlock()
	// Renderer-initiated closes dispatch the plugin close and emit no event
	// (the renderer already knows); a cancelled start never opened a
	// plugin-side connection the plugin cannot release idempotently.
	h.dispatchCloseOperation(conn)
}

// runConnectionOutputLoop polls the plugin connection for output. Plugin
// output is published through the terminal session's ordinary data plane
// (zmodem detection, flow control, observers, route publishing); the
// plugin:connection-data event is gated behind SetPluginConnectionDataEvents
// for non-terminal consumers.
func (h *pluginExtensionHost) runConnectionOutputLoop(ctx context.Context, conn *pluginConnection) {
	defer close(conn.done)
	for {
		if ctx.Err() != nil {
			return
		}
		callCtx, cancel := context.WithTimeout(ctx, clampDispatchDeadline(0))
		result, err := h.invokeExtension(callCtx, extensionInvocation{
			pluginID:   conn.pluginID,
			providerID: conn.providerID,
			kind:       providers.KindConnection,
			operation:  opConnReadOutput,
			requestID:  conn.sessionID,
			deadline:   clampDispatchDeadline(0),
			payload: map[string]any{
				"connectionId": conn.connectionTarget(),
				"maxBytes":     maxConnChunkBytes,
			},
		})
		cancel()
		if err != nil {
			if errors.Is(err, errExtensionCancelled) || ctx.Err() != nil {
				return
			}
			// Transport failure: the connection is broken; end the session
			// through the ordinary close path (exit tracking + terminal:exit).
			reason := "error"
			h.finishConnectionFromPlugin(conn, reason)
			return
		}
		var output connectionOutputResult
		if jsonErr := json.Unmarshal(result, &output); jsonErr != nil {
			h.finishConnectionFromPlugin(conn, "error")
			return
		}
		filled := 0
		if output.Data != "" {
			if raw, decodeErr := base64.StdEncoding.DecodeString(output.Data); decodeErr == nil && len(raw) > 0 {
				filled = len(raw)
				delivered := false
				if h.sessions != nil {
					delivered = h.sessions.PublishPluginOutput(conn.sessionID, raw)
				}
				if h.dataEvents.Load() {
					h.emitEvent(eventConnectionData, map[string]any{
						"sessionId": conn.sessionID,
						"data":      output.Data,
					})
				}
				if !delivered {
					// The session is gone (renderer closed it); stop polling.
					h.finishConnectionFromPlugin(conn, "closed")
					return
				}
			}
		}
		if output.Closed {
			h.finishConnectionFromPlugin(conn, output.Reason)
			return
		}
		if filled < maxConnChunkBytes {
			select {
			case <-ctx.Done():
				return
			case <-time.After(idleConnPollInterval):
			}
		}
	}
}

// finishConnectionFromPlugin ends the connection from the plugin side or on
// a broken transport. It runs ON the output-loop goroutine, so it must never
// wait for conn.done (only the loop itself closes it, on return), and it
// must not re-enter the terminal close hook while holding the close state:
// PluginSessionClosed triggers finishSessionClose → hooks.Close →
// teardownConnection, which observes closed=true via beginClose and returns
// immediately. Exit tracking (terminal:exit, session removal) runs in the
// ordinary async close path and does not depend on this call returning.
func (h *pluginExtensionHost) finishConnectionFromPlugin(conn *pluginConnection, reason string) {
	if !conn.beginClose() {
		// A renderer-initiated teardown already owns this close; the loop
		// just stops.
		return
	}
	if reason == "" {
		reason = "closed"
	}
	if conn.cancel != nil {
		conn.cancel()
	}
	h.mu.Lock()
	if current, ok := h.connections[conn.sessionID]; ok && current == conn {
		delete(h.connections, conn.sessionID)
	}
	h.mu.Unlock()
	if h.sessions != nil {
		h.sessions.PluginSessionClosed(conn.sessionID, reason)
	}
	h.emitEvent(eventConnectionClosed, map[string]any{
		"sessionId": conn.sessionID,
		"reason":    reason,
	})
}

// WritePluginConnection forwards renderer stdin bytes (base64) to the plugin
// connection (bridge writePluginConnection).
func (s *PluginService) WritePluginConnection(sessionID string, data string) error {
	s.ext.mu.Lock()
	conn, ok := s.ext.connections[sessionID]
	s.ext.mu.Unlock()
	if !ok {
		return fmt.Errorf("plugin connection session %q not found", sessionID)
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return errors.New("plugin connection data must be base64")
	}
	_, err = s.ext.writeConnectionInput(conn, raw)
	return err
}

// ControlPluginConnection runs resize/signal/reconnect/close/getStatus
// (bridge controlPluginConnection).
func (s *PluginService) ControlPluginConnection(sessionID, operation string, payload map[string]any) (map[string]any, error) {
	s.ext.mu.Lock()
	conn, ok := s.ext.connections[sessionID]
	s.ext.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("plugin connection session %q not found", sessionID)
	}
	switch operation {
	case "resize":
		columns, rows := dimsFromPayload(payload)
		err := s.ext.controlConnectionOperation(conn, opConnResize, map[string]any{
			"connectionId": conn.connectionTarget(),
			"columns":      columns,
			"rows":         rows,
		})
		return nil, err
	case "signal":
		signal, _ := payload["signal"].(string)
		err := s.ext.controlConnectionOperation(conn, opConnSignal, map[string]any{
			"connectionId": conn.connectionTarget(),
			"signal":       signal,
		})
		return nil, err
	case "getStatus":
		ctx, cancel := context.WithTimeout(context.Background(), clampDispatchDeadline(0))
		defer cancel()
		result, err := s.ext.invokeExtension(ctx, extensionInvocation{
			pluginID:   conn.pluginID,
			providerID: conn.providerID,
			kind:       providers.KindConnection,
			operation:  opConnStatus,
			requestID:  sessionID,
			deadline:   clampDispatchDeadline(0),
			payload:    map[string]any{"connectionId": conn.connectionTarget()},
		})
		if err != nil {
			return nil, err
		}
		var decoded map[string]any
		if err := json.Unmarshal(result, &decoded); err != nil || decoded == nil {
			return map[string]any{"status": "unknown"}, nil
		}
		return decoded, nil
	case "reconnect":
		return s.ext.reconnectConnection(conn)
	case "close":
		if s.ext.sessions != nil {
			// The ordinary close path invokes the plugin close hook exactly
			// once and tears the session down with exit tracking.
			_ = s.ext.sessions.CloseSession(sessionID)
			return nil, nil
		}
		s.ext.teardownConnection(conn, "closed")
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported plugin connection operation %q", operation)
	}
}

func dimsFromPayload(payload map[string]any) (int, int) {
	columns, rows := 0, 0
	if payload != nil {
		if value, ok := payload["columns"].(float64); ok {
			columns = int(value)
		}
		if value, ok := payload["rows"].(float64); ok {
			rows = int(value)
		}
	}
	return columns, rows
}

// reconnectConnection re-runs the open flow with the stored configuration
// and swaps the poller onto the new plugin connectionId. The correlation
// swap is guarded (conn.mu) so the output loop never observes a torn state.
func (h *pluginExtensionHost) reconnectConnection(conn *pluginConnection) (map[string]any, error) {
	if conn.isClosed() {
		return nil, errors.New("plugin connection is closed")
	}
	request := conn.start
	requestID := orMintRequestID("")
	credential, err := h.resolveConnectionCredential(conn.pluginID, request.Credential)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"configuration": request.Configuration,
		"operationId":   requestID,
		"columns":       request.Columns,
		"rows":          request.Rows,
	}
	if credential != nil {
		payload["credential"] = credential
	}
	if request.AuthenticationProviderID != "" {
		payload["authenticationProviderId"] = request.AuthenticationProviderID
	}
	result, err := h.invokeExtension(context.Background(), extensionInvocation{
		pluginID:   conn.pluginID,
		providerID: conn.providerID,
		kind:       providers.KindConnection,
		operation:  opConnOpen,
		requestID:  requestID,
		deadline:   clampDispatchDeadline(request.DeadlineMs),
		payload:    payload,
	})
	if err != nil {
		return nil, err
	}
	var opened connectionOpenResult
	if err := json.Unmarshal(result, &opened); err != nil || opened.ConnectionID == "" {
		return nil, errors.New("plugin connection reconnect returned no connectionId")
	}
	conn.setConnectionTarget(opened.ConnectionID)
	conn.setConnectionRequestID(requestID)
	if opened.Status == "" {
		opened.Status = "connecting"
	}
	return map[string]any{
		"connectionId": opened.ConnectionID,
		"status":       opened.Status,
	}, nil
}

// ---------------------------------------------------------------------------
// Importer providers
// ---------------------------------------------------------------------------

// SelectPluginImporterFile opens the native file picker, keeps the selected
// path staged behind an opaque token, and returns a bounded detect sample.
func (s *PluginService) SelectPluginImporterFile() (map[string]any, error) {
	app := application.Get()
	if app == nil {
		return nil, errors.New("native file dialog is unavailable")
	}
	path, err := app.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		CanChooseFiles:       true,
		CanChooseDirectories: false,
		Title:                "Import hosts",
	}).PromptForSingleSelection()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil // cancelled
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open selected file: %w", err)
	}
	defer file.Close()
	sample := make([]byte, maxImporterSampleBytes)
	read, err := file.Read(sample)
	if err != nil && read == 0 {
		return nil, fmt.Errorf("read selected file: %w", err)
	}
	sample = sample[:read]
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxImporterInputBytes {
		return nil, fmt.Errorf("selected file exceeds the importer input bound (%d bytes)", maxImporterInputBytes)
	}
	token := mintPluginRequestID()
	name := path
	if index := strings.LastIndexAny(path, `\/`); index >= 0 && index+1 < len(path) {
		name = path[index+1:]
	}
	s.ext.mu.Lock()
	s.ext.sweepImporterFilesLocked()
	if len(s.ext.importFiles) >= maxStagedImporterFiles {
		s.ext.mu.Unlock()
		return nil, errors.New("too many staged importer files; release one first")
	}
	s.ext.importFiles[token] = &stagedImporterFile{
		token:  token,
		path:   path,
		name:   name,
		sample: sample,
	}
	s.ext.mu.Unlock()
	return map[string]any{
		"selectionToken": token,
		"fileName":       name,
		"sample":         sample,
	}, nil
}

// ReleasePluginImporterFile drops a staged importer file (and its sample).
func (s *PluginService) ReleasePluginImporterFile(selectionToken string) (bool, error) {
	if selectionToken == "" {
		return false, nil
	}
	s.ext.mu.Lock()
	defer s.ext.mu.Unlock()
	if _, ok := s.ext.importFiles[selectionToken]; !ok {
		return false, nil
	}
	delete(s.ext.importFiles, selectionToken)
	return true, nil
}

func (h *pluginExtensionHost) sweepImporterFilesLocked() {
	// Capacity guard: make room for one more staged file by evicting an
	// arbitrary entry (staging is a short-lived selection, not a queue).
	for token := range h.importFiles {
		delete(h.importFiles, token)
		return
	}
}

type PluginImporterDetectRequest struct {
	RequestID  string `json:"requestId,omitempty"`
	ProviderID string `json:"providerId"`
	Sample     string `json:"sample"` // base64
	FileName   string `json:"fileName,omitempty"`
	MediaType  string `json:"mediaType,omitempty"`
	DeadlineMs int    `json:"deadlineMs,omitempty"`
}

// DetectPluginImporter asks the importer provider to identify the format of
// the given sample (contract ImporterDetectPayload).
func (s *PluginService) DetectPluginImporter(request PluginImporterDetectRequest) (map[string]any, error) {
	contribution, err := s.resolveExtensionProvider(providers.KindImporter, request.ProviderID)
	if err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(request.Sample)
	if err != nil {
		return nil, errors.New("importer sample must be base64")
	}
	if len(raw) > maxImporterSampleBytes {
		return nil, errors.New("importer sample exceeds the detect bound")
	}
	requestID := orMintRequestID(request.RequestID)
	payload := map[string]any{
		"sample": map[string]any{
			"encoding": "base64",
			"data":     request.Sample,
		},
	}
	if request.FileName != "" {
		payload["fileName"] = request.FileName
	}
	if request.MediaType != "" {
		payload["mediaType"] = request.MediaType
	}
	result, err := s.ext.invokeExtension(context.Background(), extensionInvocation{
		pluginID:   contribution.PluginID,
		providerID: contribution.Provider.ID,
		kind:       providers.KindImporter,
		operation:  opImportDetect,
		requestID:  requestID,
		deadline:   clampDispatchDeadline(request.DeadlineMs),
		payload:    payload,
	})
	if err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := json.Unmarshal(result, &decoded); err != nil || decoded == nil {
		return nil, errors.New("plugin importer detect returned invalid JSON")
	}
	return decoded, nil
}

type PluginImporterParseRequest struct {
	RequestID      string         `json:"requestId,omitempty"`
	ProviderID     string         `json:"providerId"`
	SelectionToken string         `json:"selectionToken"`
	MediaType      string         `json:"mediaType,omitempty"`
	Options        map[string]any `json:"options,omitempty"`
	DeadlineMs     int            `json:"deadlineMs,omitempty"`
}

// ParsePluginImporterFile streams the staged file to the importer provider
// (parseBegin → parseChunk* → parseFinish), then pulls the parsed records
// (parseRecords until done). Progress records are mirrored to the renderer
// through the plugin:importer-progress event; drafts and warnings/errors are
// returned to the caller.
func (s *PluginService) ParsePluginImporterFile(request PluginImporterParseRequest) (map[string]any, error) {
	contribution, err := s.resolveExtensionProvider(providers.KindImporter, request.ProviderID)
	if err != nil {
		return nil, err
	}
	s.ext.mu.Lock()
	staged, ok := s.ext.importFiles[request.SelectionToken]
	s.ext.mu.Unlock()
	if !ok {
		return nil, errors.New("importer selection token is unknown or was released")
	}
	requestID := orMintRequestID(request.RequestID)
	invoke := func(operation string, payload map[string]any, deadline int) (json.RawMessage, error) {
		return s.ext.invokeExtension(context.Background(), extensionInvocation{
			pluginID:   contribution.PluginID,
			providerID: contribution.Provider.ID,
			kind:       providers.KindImporter,
			operation:  operation,
			requestID:  requestID,
			deadline:   clampDispatchDeadline(deadline),
			payload:    payload,
		})
	}
	abort := func() {
		_, _ = invoke(opImportParseAbort, map[string]any{
			"operationId": requestID,
		}, int(closeDispatchDeadline/time.Millisecond))
	}

	file, err := os.Open(staged.path)
	if err != nil {
		return nil, fmt.Errorf("open staged importer file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxImporterInputBytes {
		return nil, fmt.Errorf("staged file exceeds the importer input bound (%d bytes)", maxImporterInputBytes)
	}

	beginPayload := map[string]any{
		"operationId": requestID,
		"byteLength":  info.Size(),
	}
	if staged.name != "" {
		beginPayload["fileName"] = staged.name
	}
	if request.MediaType != "" {
		beginPayload["mediaType"] = request.MediaType
	} else if staged.media != "" {
		beginPayload["mediaType"] = staged.media
	}
	if request.Options != nil {
		beginPayload["options"] = request.Options
	}
	beginResult, err := invoke(opImportParseBegin, beginPayload, request.DeadlineMs)
	if err != nil {
		abort()
		return nil, err
	}
	var begin struct {
		WindowBytes int `json:"windowBytes"`
	}
	_ = json.Unmarshal(beginResult, &begin)
	windowBytes := begin.WindowBytes
	if windowBytes <= 0 || windowBytes > maxImportChunkData {
		windowBytes = maxImportChunkData

	}

	buffer := make([]byte, windowBytes)
	sequence := 0
	for {
		read, readErr := file.Read(buffer)
		if read > 0 {
			chunk := buffer[:read]
			if _, err := invoke(opImportParseChunk, map[string]any{
				"operationId": requestID,
				"sequence":    sequence,
				"encoding":    "base64",
				"data":        base64.StdEncoding.EncodeToString(chunk),
			}, request.DeadlineMs); err != nil {
				abort()
				return nil, err
			}
			sequence++
		}
		if readErr != nil || read == 0 {
			break
		}
	}

	finishResult, err := invoke(opImportParseFinish, map[string]any{
		"operationId": requestID,
	}, request.DeadlineMs)
	if err != nil {
		abort()
		return nil, err
	}
	var parsed struct {
		Parsed   int `json:"parsed"`
		Warnings int `json:"warnings"`
		Errors   int `json:"errors"`
	}
	if err := json.Unmarshal(finishResult, &parsed); err != nil {
		abort()
		return nil, errors.New("plugin importer parseFinish returned invalid JSON")
	}

	records := []any{}
	total := 0
	for {
		if total >= maxImporterRecords {
			abort()
			return nil, errors.New("plugin importer produced too many records")
		}
		recordResult, err := invoke(opImportParseRecord, map[string]any{
			"operationId": requestID,
			"maxRecords":  500,
		}, request.DeadlineMs)
		if err != nil {
			abort()
			return nil, err
		}
		var batch struct {
			Records []any `json:"records"`
			Done    bool  `json:"done"`
		}
		if err := json.Unmarshal(recordResult, &batch); err != nil {
			abort()
			return nil, errors.New("plugin importer parseRecords returned invalid JSON")
		}
		for _, record := range batch.Records {
			if isProgressRecord(record) {
				s.ext.emitImporterProgress(requestID, contribution.Provider.ID, record)
				continue
			}
			records = append(records, record)
			total++
			if total >= maxImporterRecords {
				break
			}
		}
		if batch.Done || len(batch.Records) == 0 {
			break
		}
	}
	abort() // best-effort state release; ignored when the plugin already finished

	return map[string]any{
		"providerId": contribution.Provider.ID,
		"result": map[string]any{
			"parsed":   parsed.Parsed,
			"warnings": parsed.Warnings,
			"errors":   parsed.Errors,
		},
		"records": records,
	}, nil
}

func (h *pluginExtensionHost) emitImporterProgress(requestID, providerID string, record any) {
	h.emitEvent(eventImporterProgress, map[string]any{
		"requestId":  requestID,
		"providerId": providerID,
		"progress":   record,
	})
}

// isProgressRecord narrows the wire record shape {type:"progress",...}.
func isProgressRecord(record any) bool {
	asMap, ok := record.(map[string]any)
	if !ok {
		return false
	}
	value, _ := asMap["type"].(string)
	return value == "progress"
}

// ---------------------------------------------------------------------------
// Authentication providers (challenge round trip)
// ---------------------------------------------------------------------------

// registerChallengeResult inspects an authentication begin/respond result and
// mirrors challenge outcomes to the renderer through the
// plugin:authentication-challenge event.
func (h *pluginExtensionHost) registerChallengeResult(pluginID, providerID, requestID string, result any) {
	asMap, ok := result.(map[string]any)
	if !ok {
		return
	}
	status, _ := asMap["status"].(string)
	switch status {
	case "challenge":
		challenge, present := asMap["challenge"]
		if !present {
			return
		}
		h.mu.Lock()
		if len(h.challenges) >= maxPendingChallenges {
			h.mu.Unlock()
			return
		}
		h.challenges[requestID] = &pendingChallenge{
			pluginID:   pluginID,
			providerID: providerID,
			requestID:  requestID,
		}
		h.mu.Unlock()
		h.emitEvent(eventAuthenticationChal, map[string]any{
			"requestId":          requestID,
			"challengeRequestId": requestID,
			"challenge":          challenge,
		})
	case "cancelled", "failed":
		h.emitChallengeCancelled(pluginID, providerID, requestID)
	case "authenticated":
		// The pending challenge (if any) is done.
		h.mu.Lock()
		delete(h.challenges, requestID)
		h.mu.Unlock()
	default:
		// Unknown or missing status: fail closed. The pending entry stays so
		// the renderer can still answer or cancel it through
		// RespondPluginAuthenticationChallenge / CancelPluginExtensionRequest;
		// no event is fabricated for a malformed plugin answer.
	}
}

func (h *pluginExtensionHost) emitChallengeCancelled(pluginID, providerID, requestID string) {
	h.mu.Lock()
	pending, ok := h.challenges[requestID]
	if ok {
		delete(h.challenges, requestID)
	}
	h.mu.Unlock()
	if ok && pending != nil {
		h.emitEvent(eventAuthenticationChal, map[string]any{
			"requestId":          requestID,
			"challengeRequestId": requestID,
			"cancelled":          true,
		})
	}
}

// RespondPluginAuthenticationChallenge forwards the renderer's answer to the
// pending authentication operation (contract AuthenticationResponsePayload).
// A new challenge in the answer re-registers the pending entry.
func (s *PluginService) RespondPluginAuthenticationChallenge(request struct {
	RequestID          string `json:"requestId"`
	ChallengeRequestID string `json:"challengeRequestId"`
	ChallengeID        string `json:"challengeId"`
	Response           any    `json:"response,omitempty"`
	Cancelled          bool   `json:"cancelled,omitempty"`
}) error {
	if request.ChallengeRequestID == "" {
		return errors.New("challengeRequestId is required")
	}
	s.ext.mu.Lock()
	pending, ok := s.ext.challenges[request.ChallengeRequestID]
	s.ext.mu.Unlock()
	if !ok || pending == nil {
		return fmt.Errorf("no pending authentication challenge %q", request.ChallengeRequestID)
	}
	payload := map[string]any{
		"operationId": request.ChallengeRequestID,
		"challengeId": request.ChallengeID,
	}
	if request.Cancelled {
		payload["cancelled"] = true
	} else {
		payload["response"] = request.Response
	}
	result, err := s.ext.invokeExtension(context.Background(), extensionInvocation{
		pluginID:   pending.pluginID,
		providerID: pending.providerID,
		kind:       providers.KindAuthentication,
		operation:  opAuthRespond,
		requestID:  request.ChallengeRequestID,
		deadline:   clampDispatchDeadline(0),
		payload:    payload,
	})
	if err != nil {
		return err
	}
	var decoded any
	if err := json.Unmarshal(result, &decoded); err != nil {
		return errors.New("plugin authentication response returned invalid JSON")
	}
	s.ext.registerChallengeResult(pending.pluginID, pending.providerID, request.ChallengeRequestID, decoded)
	return nil
}
