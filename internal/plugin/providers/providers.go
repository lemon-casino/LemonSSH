// Package providers owns the terminal/extension provider registry: enabled
// plugins declare providers over the lemonssh-wasm-abi v1 dispatch channel
// ("providers.list"), the host validates every declaration against the
// fail-closed permission broker before it ever reaches a caller, and the
// registry answers enumeration, invocation, cancellation and session-event
// queries for the Wails PluginService.
//
// Security model (docs/plugin-platform/terminal-providers.md):
//   - A declaration is only accepted for a provider kind whose
//     provider|<kind>|read permission the plugin's stored manifest declares
//     AND the broker has granted. Grants derive from the manifest only
//     (host.Host.Grant), so a plugin can never claim a kind it did not
//     declare, and disabling/uninstalling revokes the grant immediately.
//   - The privileged terminal.interceptor.* kinds are not registrable here;
//     they belong to the separate, explicit-grant intercept fast path.
//   - Provider invocations carry an immutable session metadata snapshot —
//     never raw terminal objects, output streams or secrets.
package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/binaricat/lemonssh/internal/plugin/permissions"
	pluginstore "github.com/binaricat/lemonssh/internal/plugin/store"
	"github.com/binaricat/lemonssh/internal/plugin/wasm"
)

// Dispatch methods of the provider protocol over the lemonssh-wasm-abi v1
// channel. The names mirror the canonical "view.data" / "command.execute"
// style used by the renderer bridge.
const (
	// MethodList asks a plugin for its provider declarations. The payload is
	// empty; the result is {"providers": [...]}.
	MethodList = "providers.list"
	// MethodInvoke runs one provider operation. The payload is
	// {providerId, kind, operation, requestId, session, payload, deadlineMs};
	// the result is the operation-specific JSON value.
	MethodInvoke = "provider.invoke"
	// MethodSessionEvent notifies a plugin about one terminal session
	// lifecycle event; the payload is the event verbatim, the result is
	// ignored.
	MethodSessionEvent = "provider.sessionEvent"
)

// PermissionKind is the manifest permission kind that gates provider
// declarations (internal/plugin/manifest validPermissionKinds).
const PermissionKind = "provider"

// PermissionMode is the only mode that grants provider registration.
const PermissionMode = "read"

// Provider kinds accepted by this registry. The terminal.interceptor.*
// kinds are deliberately absent: hot interception is a separate privileged
// fast path with explicit terminal.intercept.* grants, never the JSON-RPC
// provider path.
const (
	KindTerminalCompletion = "terminal.completion"
	KindTerminalDecoration = "terminal.decoration"
	KindTerminalLink       = "terminal.link"
	KindTerminalHover      = "terminal.hover"
	KindTerminalMatcher    = "terminal.matcher"
	KindTerminalSemantic   = "terminal.semantic"
	KindTerminalPrompt     = "terminal.prompt"
	KindTerminalBackground = "terminal.background"
	KindTerminalTheme      = "terminal.theme"
	KindConnection         = "connection"
	KindAuthentication     = "authentication"
	KindImporter           = "importer"
	KindSync               = "sync"
)

// TerminalKinds are the provider kinds served by the terminal provider bridge.
func TerminalKinds() map[string]bool {
	return map[string]bool{
		KindTerminalCompletion: true, KindTerminalDecoration: true,
		KindTerminalLink: true, KindTerminalHover: true, KindTerminalMatcher: true,
		KindTerminalSemantic: true, KindTerminalPrompt: true,
		KindTerminalBackground: true, KindTerminalTheme: true,
	}
}

// ExtensionKinds are the provider kinds served by the extension provider
// bridge (connection/authentication/importer/sync).
func ExtensionKinds() map[string]bool {
	return map[string]bool{KindConnection: true, KindAuthentication: true, KindImporter: true, KindSync: true}
}

// SupportedKinds is every kind this registry accepts.
func SupportedKinds() map[string]bool {
	merged := TerminalKinds()
	for kind := range ExtensionKinds() {
		merged[kind] = true
	}
	return merged
}

// Result statuses (mirrors the contract ProviderResult).
const (
	StatusOK        = "ok"
	StatusCancelled = "cancelled"
	StatusFailed    = "failed"
)

// Wire error codes (packages/plugin-sdk PLUGIN_ERROR_WIRE_CODES).
const (
	CodeCancelled        = -32001
	CodeUnknown          = -32002
	CodeDeadlineExceeded = -32004
	CodeInternal         = -32013
)

const (
	maxDeclarationsPerPlugin = 256
	maxCapabilities          = 32
	maxCapabilityLength      = 128
	maxLabelLength           = 256
	maxSchemaBytes           = 16 << 10
	maxRequestIDLength       = 128
	maxOperationLength       = 128
	maxProviderIDLength      = 128
	maxEventTypeLength       = 64
	cancelledTTL             = 10 * time.Minute
)

var (
	identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)
	// ErrInvalidDeclaration marks a provider declaration the host rejects.
	ErrInvalidDeclaration = errors.New("invalid provider declaration")
)

// Declaration is one validated provider contribution (contract
// ProviderContribution, localized labels resolved to plain strings).
type Declaration struct {
	ID                  string          `json:"id"`
	Label               string          `json:"label"`
	Description         string          `json:"description,omitempty"`
	Kind                string          `json:"kind"`
	Capabilities        []string        `json:"capabilities,omitempty"`
	ConfigurationSchema json.RawMessage `json:"configurationSchema,omitempty"`
}

// Contribution wraps a declaration with the declaring plugin's identity.
type Contribution struct {
	PluginID          string      `json:"pluginId"`
	PluginVersion     string      `json:"pluginVersion"`
	PluginDisplayName string      `json:"pluginDisplayName"`
	Provider          Declaration `json:"provider"`
}

// TerminalSessionSnapshot is the immutable session metadata providers receive
// (renderer LemonsshTerminalSessionSnapshot).
type TerminalSessionSnapshot struct {
	SessionID       string `json:"sessionId"`
	HostID          string `json:"hostId,omitempty"`
	WorkspaceID     string `json:"workspaceId,omitempty"`
	Protocol        string `json:"protocol"`
	Status          string `json:"status"`
	CWD             string `json:"cwd,omitempty"`
	Title           string `json:"title,omitempty"`
	ShellType       string `json:"shellType,omitempty"`
	Cols            int    `json:"cols,omitempty"`
	Rows            int    `json:"rows,omitempty"`
	AlternateScreen bool   `json:"alternateScreen,omitempty"`
}

// SessionEvent is one terminal session lifecycle notification.
type SessionEvent struct {
	Type     string                  `json:"type"`
	Session  TerminalSessionSnapshot `json:"session"`
	ExitCode *int                    `json:"exitCode,omitempty"`
}

// TerminalRequest is one provider invocation fan-out (renderer
// LemonsshTerminalProviderRequest).
type TerminalRequest struct {
	RequestID            string                  `json:"requestId"`
	Kind                 string                  `json:"kind"`
	Operation            string                  `json:"operation"`
	Session              TerminalSessionSnapshot `json:"session"`
	Payload              json.RawMessage         `json:"payload,omitempty"`
	Locale               string                  `json:"locale,omitempty"`
	PreferredProviderIDs []string                `json:"preferredProviderIds,omitempty"`
	DeadlineMs           int                     `json:"deadlineMs,omitempty"`
}

// ProviderError is the failed-result payload (numeric wire code, contract
// RpcErrorObject shape).
type ProviderError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// TerminalResult is one provider's answer (renderer
// LemonsshTerminalProviderResult).
type TerminalResult struct {
	PluginID      string          `json:"pluginId"`
	PluginVersion string          `json:"pluginVersion"`
	ProviderID    string          `json:"providerId"`
	Kind          string          `json:"kind"`
	RequestID     string          `json:"requestId"`
	Status        string          `json:"status"`
	Result        json.RawMessage `json:"result,omitempty"`
	Error         *ProviderError  `json:"error,omitempty"`
}

// SessionDelivery reports one plugin's session-event delivery outcome.
type SessionDelivery struct {
	PluginID  string `json:"pluginId"`
	Delivered bool   `json:"delivered"`
}

// DispatchFunc sends one WASM dispatch request. The registry never touches
// the wasm runtime directly so tests can drive it with a fake.
type DispatchFunc func(ctx context.Context, pluginID, method, payloadJSON string) (*wasm.DispatchResult, error)

type cacheEntry struct {
	version      string
	grantKey     string
	locale       string
	declarations []Declaration
}

type activeRequest struct {
	cancel context.CancelFunc
}

// Registry resolves provider declarations of enabled plugins.
type Registry struct {
	store    *pluginstore.Store
	broker   *permissions.Broker
	dispatch DispatchFunc

	mu        sync.Mutex
	cache     map[string]cacheEntry
	active    map[string]*activeRequest
	cancelled map[string]time.Time
}

// NewRegistry builds a registry over the plugin inventory, the fail-closed
// broker and a dispatch transport.
func NewRegistry(store *pluginstore.Store, broker *permissions.Broker, dispatch DispatchFunc) *Registry {
	return &Registry{
		store:     store,
		broker:    broker,
		dispatch:  dispatch,
		cache:     make(map[string]cacheEntry),
		active:    make(map[string]*activeRequest),
		cancelled: make(map[string]time.Time),
	}
}

// permissionKey derives the canonical broker resource key for one provider
// kind — the same ["kind","resource"]:mode shape host.Host uses.
func permissionKey(kind string) string {
	encoded, _ := json.Marshal([]string{PermissionKind, kind})
	return string(encoded) + ":" + PermissionMode
}

// grantedKinds returns the provider kinds the plugin currently holds a
// manifest-derived read grant for. Every check is fail-closed.
func (r *Registry) grantedKinds(pluginID string, declared []string) []string {
	if r.broker == nil {
		return nil
	}
	granted := make([]string, 0, len(declared))
	for _, kind := range declared {
		if r.broker.Check(pluginID, permissionKey(kind), PermissionMode) == nil {
			granted = append(granted, kind)
		}
	}
	sort.Strings(granted)
	return granted
}

// manifestProviderKinds decodes the stored manifest snapshot and returns the
// provider kinds it declares (kind "provider", mode "read").
func manifestProviderKinds(record *pluginstore.PackageRecord) []string {
	var manifest struct {
		Permissions []struct {
			Kind     string `json:"kind"`
			Resource string `json:"resource"`
			Mode     string `json:"mode"`
		} `json:"permissions"`
	}
	if record == nil || len(record.Manifest) == 0 {
		return nil
	}
	if err := json.Unmarshal(record.Manifest, &manifest); err != nil {
		return nil
	}
	seen := map[string]bool{}
	kinds := []string{}
	for _, permission := range manifest.Permissions {
		if permission.Kind == PermissionKind && permission.Mode == PermissionMode && permission.Resource != "" && !seen[permission.Resource] {
			seen[permission.Resource] = true
			kinds = append(kinds, permission.Resource)
		}
	}
	sort.Strings(kinds)
	return kinds
}

func (r *Registry) invalidateLocked(pluginID string) {
	delete(r.cache, pluginID)
}

// Invalidate drops the cached declaration snapshot of one plugin (call after
// restart/reinstall so a changed entrypoint is re-queried).
func (r *Registry) Invalidate(pluginID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.invalidateLocked(pluginID)
}

// listDeclarations returns the accepted declarations of one enabled plugin.
// A nil result means "contributes nothing" — disabled, ungranted, no dispatch
// ABI or a broken module never fails the whole enumeration.
func (r *Registry) listDeclarations(ctx context.Context, record *pluginstore.PackageRecord, locale string) []Declaration {
	if record == nil {
		return nil
	}
	pluginID := record.PluginID
	if record.State != pluginstore.StateEnabled {
		r.mu.Lock()
		r.invalidateLocked(pluginID)
		r.mu.Unlock()
		return nil
	}
	granted := r.grantedKinds(pluginID, manifestProviderKinds(record))
	if len(granted) == 0 {
		r.mu.Lock()
		r.invalidateLocked(pluginID)
		r.mu.Unlock()
		return nil
	}
	grantKey := strings.Join(granted, ",")
	r.mu.Lock()
	cached, hit := r.cache[pluginID]
	r.mu.Unlock()
	if hit && cached.version == record.Version && cached.grantKey == grantKey && cached.locale == locale {
		return cached.declarations
	}
	declarations := r.dispatchDeclarations(ctx, pluginID, granted, locale)
	r.mu.Lock()
	r.cache[pluginID] = cacheEntry{version: record.Version, grantKey: grantKey, locale: locale, declarations: declarations}
	r.mu.Unlock()
	return declarations
}

// rawDeclaration mirrors the plugin-side providers.list entry with localized
// text fields still encoded.
type rawDeclaration struct {
	ID                  string          `json:"id"`
	Label               json.RawMessage `json:"label,omitempty"`
	Description         json.RawMessage `json:"description,omitempty"`
	Kind                string          `json:"kind"`
	Capabilities        []string        `json:"capabilities,omitempty"`
	ConfigurationSchema json.RawMessage `json:"configurationSchema,omitempty"`
}

// dispatchDeclarations asks the plugin over the dispatch channel and keeps
// only the declarations that validate and are granted.
func (r *Registry) dispatchDeclarations(ctx context.Context, pluginID string, granted []string, locale string) []Declaration {
	if r.dispatch == nil {
		return nil
	}
	response, err := r.dispatch(ctx, pluginID, MethodList, "")
	if err != nil || response == nil || !response.OK || len(response.Result) == 0 {
		// Transport failures degrade to "no providers" so one broken plugin
		// cannot blank the registry for everyone else.
		return nil
	}
	var payload struct {
		Providers []rawDeclaration `json:"providers"`
	}
	if err := json.Unmarshal(response.Result, &payload); err != nil {
		return nil
	}
	if len(payload.Providers) > maxDeclarationsPerPlugin {
		payload.Providers = payload.Providers[:maxDeclarationsPerPlugin]
	}
	allowed := make(map[string]bool, len(granted))
	for _, kind := range granted {
		allowed[kind] = true
	}
	seen := make(map[string]bool, len(payload.Providers))
	accepted := make([]Declaration, 0, len(payload.Providers))
	for _, raw := range payload.Providers {
		if !allowed[raw.Kind] {
			continue // never accepted: ungranted (⇒ undeclared) kind
		}
		declaration := Declaration{
			ID:                  raw.ID,
			Label:               resolveLocalizedText(raw.Label, locale),
			Description:         resolveLocalizedText(raw.Description, locale),
			Kind:                raw.Kind,
			Capabilities:        raw.Capabilities,
			ConfigurationSchema: raw.ConfigurationSchema,
		}
		if err := ValidateDeclaration(declaration); err != nil {
			continue
		}
		if seen[declaration.ID] {
			continue
		}
		seen[declaration.ID] = true
		declaration.Capabilities = boundCapabilities(declaration.Capabilities)
		accepted = append(accepted, declaration)
	}
	return accepted
}

// resolveLocalizedText decodes the contract LocalizedText (string |
// {locale: string}): the exact locale wins, then "en", then the first key in
// sorted order.
func resolveLocalizedText(raw json.RawMessage, locale string) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var localized map[string]string
	if err := json.Unmarshal(raw, &localized); err != nil {
		return ""
	}
	if value, ok := localized[locale]; ok {
		return value
	}
	if value, ok := localized["en"]; ok {
		return value
	}
	keys := make([]string, 0, len(localized))
	for key := range localized {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		return localized[keys[0]]
	}
	return ""
}

// List enumerates the declared providers of every enabled plugin, optionally
// filtered to one kind (empty kind = all kinds). Unknown kinds list nothing.
func (r *Registry) List(ctx context.Context, kind, locale string) []Contribution {
	results := []Contribution{}
	if kind != "" && !SupportedKinds()[kind] {
		return results
	}
	for _, record := range r.store.List() {
		if record == nil {
			continue
		}
		for _, declaration := range r.listDeclarations(ctx, record, locale) {
			if kind != "" && declaration.Kind != kind {
				continue
			}
			results = append(results, Contribution{
				PluginID:          record.PluginID,
				PluginVersion:     record.Version,
				PluginDisplayName: manifestDisplayName(record),
				Provider:          declaration,
			})
		}
	}
	return results
}

func manifestDisplayName(record *pluginstore.PackageRecord) string {
	var manifest struct {
		DisplayName string `json:"displayName"`
	}
	if record != nil && len(record.Manifest) > 0 {
		if err := json.Unmarshal(record.Manifest, &manifest); err == nil && manifest.DisplayName != "" {
			return manifest.DisplayName
		}
	}
	if record != nil {
		return record.PluginID
	}
	return ""
}

// Provide fans one terminal provider request out to every matching provider.
// Each dispatch honors the request deadline (clamped to the host's 10 s
// dispatch cap) and can be aborted with Cancel.
func (r *Registry) Provide(ctx context.Context, request TerminalRequest) []TerminalResult {
	if err := validateTerminalRequest(request, TerminalKinds()); err != nil {
		return []TerminalResult{failedResult(request, CodeUnknown, err.Error())}
	}
	matching := r.matchingProviders(ctx, request.Kind, request.PreferredProviderIDs)
	results := make([]TerminalResult, 0, len(matching))
	cancelled := func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		_, hit := r.cancelled[request.RequestID]
		return hit
	}
	for _, contribution := range matching {
		if ctx.Err() != nil || cancelled() {
			results = append(results, TerminalResult{
				PluginID: contribution.PluginID, PluginVersion: contribution.PluginVersion,
				ProviderID: contribution.Provider.ID, Kind: request.Kind,
				RequestID: request.RequestID, Status: StatusCancelled,
			})
			continue
		}
		results = append(results, r.invokeProvider(ctx, contribution, request, cancelled))
	}
	r.mu.Lock()
	delete(r.cancelled, request.RequestID)
	r.mu.Unlock()
	return results
}

func (r *Registry) matchingProviders(ctx context.Context, kind string, preferred []string) []Contribution {
	all := r.List(ctx, kind, "")
	if len(preferred) == 0 {
		return all
	}
	want := make(map[string]bool, len(preferred))
	for _, id := range preferred {
		want[id] = true
	}
	filtered := make([]Contribution, 0, len(all))
	for _, contribution := range all {
		if want[contribution.Provider.ID] {
			filtered = append(filtered, contribution)
		}
	}
	return filtered
}

// invokeProvider dispatches one provider.invoke and maps every failure mode
// onto the structured result envelope.
func (r *Registry) invokeProvider(ctx context.Context, contribution Contribution, request TerminalRequest, cancelled func() bool) TerminalResult {
	base := TerminalResult{
		PluginID:      contribution.PluginID,
		PluginVersion: contribution.PluginVersion,
		ProviderID:    contribution.Provider.ID,
		Kind:          request.Kind,
		RequestID:     request.RequestID,
	}
	deadline := time.Duration(request.DeadlineMs) * time.Millisecond
	if deadline <= 0 || deadline > wasm.DefaultDispatchLimit {
		deadline = wasm.DefaultDispatchLimit
	}
	callCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	r.mu.Lock()
	if _, hit := r.cancelled[request.RequestID]; hit {
		r.mu.Unlock()
		base.Status = StatusCancelled
		return base
	}
	r.active[request.RequestID] = &activeRequest{cancel: cancel}
	r.mu.Unlock()

	payload, err := json.Marshal(map[string]any{
		"providerId": contribution.Provider.ID,
		"kind":       request.Kind,
		"operation":  request.Operation,
		"requestId":  request.RequestID,
		"session":    request.Session,
		"payload":    request.Payload,
		"deadlineMs": request.DeadlineMs,
	})
	if err == nil {
		var response *wasm.DispatchResult
		response, err = r.dispatch(callCtx, contribution.PluginID, MethodInvoke, string(payload))
		if err == nil {
			r.mu.Lock()
			delete(r.active, request.RequestID)
			_, wasCancelled := r.cancelled[request.RequestID]
			r.mu.Unlock()
			return finalizeResult(base, response, wasCancelled || callCtx.Err() == context.Canceled)
		}
	}

	r.mu.Lock()
	delete(r.active, request.RequestID)
	_, wasCancelled := r.cancelled[request.RequestID]
	r.mu.Unlock()

	if wasCancelled || errors.Is(callCtx.Err(), context.Canceled) {
		base.Status = StatusCancelled
		return base
	}
	base.Status = StatusFailed
	code := CodeInternal
	if errors.Is(err, wasm.ErrDispatchTimeout) {
		code = CodeDeadlineExceeded
	}
	base.Error = &ProviderError{Code: code, Message: err.Error()}
	return base
}

func finalizeResult(base TerminalResult, response *wasm.DispatchResult, wasCancelled bool) TerminalResult {
	if wasCancelled {
		base.Status = StatusCancelled
		return base
	}
	if response == nil || !response.OK {
		base.Status = StatusFailed
		pluginCode := "unknown"
		message := "provider returned no result"
		var data json.RawMessage
		if response != nil && response.Error != nil {
			pluginCode = response.Error.Code
			message = response.Error.Message
			if len(response.Error.Data) > 0 {
				data = response.Error.Data
			}
		}
		if encoded, marshalErr := json.Marshal(map[string]string{"pluginCode": pluginCode}); marshalErr == nil && data == nil {
			data = encoded
		}
		base.Error = &ProviderError{Code: CodeUnknown, Message: message, Data: data}
		return base
	}
	base.Status = StatusOK
	base.Result = response.Result
	return base
}

// Cancel aborts one in-flight (or about-to-run) provider request. It reports
// whether the request was known.
func (r *Registry) Cancel(requestID string) bool {
	if requestID == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	active, wasActive := r.active[requestID]
	delete(r.active, requestID)
	if wasActive && active.cancel != nil {
		active.cancel()
	}
	now := time.Now()
	for id, at := range r.cancelled {
		if now.Sub(at) > cancelledTTL {
			delete(r.cancelled, id)
		}
	}
	r.cancelled[requestID] = now
	return wasActive
}

// PublishSessionEvent delivers one terminal session lifecycle event to every
// plugin that currently contributes at least one granted provider. Delivery
// failures degrade to delivered:false for that plugin.
func (r *Registry) PublishSessionEvent(ctx context.Context, event SessionEvent) []SessionDelivery {
	if strings.TrimSpace(event.Type) == "" || len(event.Type) > maxEventTypeLength {
		return []SessionDelivery{}
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return []SessionDelivery{}
	}
	deliveries := []SessionDelivery{}
	for _, record := range r.store.List() {
		if record == nil {
			continue
		}
		if len(r.listDeclarations(ctx, record, "")) == 0 {
			continue
		}
		delivered := false
		if r.dispatch != nil {
			response, dispatchErr := r.dispatch(ctx, record.PluginID, MethodSessionEvent, string(payload))
			delivered = dispatchErr == nil && response != nil && response.OK
		}
		deliveries = append(deliveries, SessionDelivery{PluginID: record.PluginID, Delivered: delivered})
	}
	return deliveries
}

// ValidateDeclaration enforces the host-side declaration contract.
func ValidateDeclaration(declaration Declaration) error {
	if !identifierPattern.MatchString(declaration.ID) {
		return fmt.Errorf("%w: id %q", ErrInvalidDeclaration, declaration.ID)
	}
	if !SupportedKinds()[declaration.Kind] {
		return fmt.Errorf("%w: unsupported kind %q", ErrInvalidDeclaration, declaration.Kind)
	}
	if strings.TrimSpace(declaration.Label) == "" || len(declaration.Label) > maxLabelLength {
		return fmt.Errorf("%w: label must be 1..%d characters", ErrInvalidDeclaration, maxLabelLength)
	}
	if len(declaration.Description) > maxLabelLength {
		return fmt.Errorf("%w: description exceeds %d characters", ErrInvalidDeclaration, maxLabelLength)
	}
	if len(declaration.Capabilities) > maxCapabilities {
		return fmt.Errorf("%w: more than %d capabilities", ErrInvalidDeclaration, maxCapabilities)
	}
	for _, capability := range declaration.Capabilities {
		if capability == "" || len(capability) > maxCapabilityLength {
			return fmt.Errorf("%w: capability %q", ErrInvalidDeclaration, capability)
		}
	}
	if len(declaration.ConfigurationSchema) > maxSchemaBytes {
		return fmt.Errorf("%w: configurationSchema exceeds %d bytes", ErrInvalidDeclaration, maxSchemaBytes)
	}
	return nil
}

func boundCapabilities(capabilities []string) []string {
	if len(capabilities) > maxCapabilities {
		capabilities = capabilities[:maxCapabilities]
	}
	for index, capability := range capabilities {
		if len(capability) > maxCapabilityLength {
			capabilities[index] = capability[:maxCapabilityLength]
		}
	}
	return capabilities
}

func validateTerminalRequest(request TerminalRequest, allowed map[string]bool) error {
	if request.RequestID == "" || len(request.RequestID) > maxRequestIDLength {
		return fmt.Errorf("provider request id must be 1..%d characters", maxRequestIDLength)
	}
	if !allowed[request.Kind] {
		return fmt.Errorf("unsupported provider kind %q", request.Kind)
	}
	if strings.TrimSpace(request.Operation) == "" || len(request.Operation) > maxOperationLength {
		return fmt.Errorf("provider operation must be 1..%d characters", maxOperationLength)
	}
	if strings.TrimSpace(request.Session.SessionID) == "" {
		return fmt.Errorf("provider request requires a session id")
	}
	if len(request.PreferredProviderIDs) > maxDeclarationsPerPlugin {
		return fmt.Errorf("too many preferred provider ids")
	}
	for _, id := range request.PreferredProviderIDs {
		if id == "" || len(id) > maxProviderIDLength {
			return fmt.Errorf("invalid preferred provider id")
		}
	}
	return nil
}

func failedResult(request TerminalRequest, code int, message string) TerminalResult {
	return TerminalResult{
		RequestID: request.RequestID,
		Kind:      request.Kind,
		Status:    StatusFailed,
		Error:     &ProviderError{Code: code, Message: message},
	}
}
