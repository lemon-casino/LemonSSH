package main

import (
	"context"

	"github.com/lemon-casino/lemonssh/internal/app/terminaluse"
	"github.com/lemon-casino/lemonssh/internal/platform/filesystem"
	"github.com/lemon-casino/lemonssh/internal/terminal/dataplane"
	"github.com/lemon-casino/lemonssh/internal/terminal/serialport"
	"github.com/lemon-casino/lemonssh/internal/terminal/ssh"
	"github.com/lemon-casino/lemonssh/internal/terminal/ymodem"
	gossh "golang.org/x/crypto/ssh"
)

// Shell-facing DTOs. The canonical definitions (and JSON contracts) live in
// internal/app/terminaluse; these aliases keep the Wails API names stable.
type (
	TelnetStartRequest          = terminaluse.TelnetStartRequest
	SSHConnectRequest           = terminaluse.SSHConnectRequest
	MoshStartRequest            = terminaluse.MoshStartRequest
	SerialStartRequest          = terminaluse.SerialStartRequest
	LocalStartRequest           = terminaluse.LocalStartRequest
	TerminalExitStatus          = terminaluse.TerminalExitStatus
	HelperSessionState          = terminaluse.HelperSessionState
	TerminalPwdOptions          = terminaluse.TerminalPwdOptions
	TerminalPwdResult           = terminaluse.TerminalPwdResult
	TerminalRemoteInfo          = terminaluse.TerminalRemoteInfo
	MonitoringResult            = terminaluse.MonitoringResult
	DockerStatsOptions          = terminaluse.DockerStatsOptions
	ProcessSignalOptions        = terminaluse.ProcessSignalOptions
	TmuxSessionRequest          = terminaluse.TmuxSessionRequest
	TmuxTargetRequest           = terminaluse.TmuxTargetRequest
	TmuxActionRequest           = terminaluse.TmuxActionRequest
	SystemServiceActionRequest  = terminaluse.SystemServiceActionRequest
	DockerInspectRequest        = terminaluse.DockerInspectRequest
	DockerActionRequest         = terminaluse.DockerActionRequest
	DockerImageActionRequest    = terminaluse.DockerImageActionRequest
	AutocompleteDirectoryEntry  = terminaluse.AutocompleteDirectoryEntry
	AutocompleteDirectoryResult = terminaluse.AutocompleteDirectoryResult
	DiscoveredShell             = terminaluse.DiscoveredShell
	PathValidation              = terminaluse.PathValidation
	ProxyProbeRequest           = terminaluse.ProxyProbeRequest
	ProxyProbeResult            = terminaluse.ProxyProbeResult
)

// TerminalService is the Wails-facing facade over internal/app/terminaluse.
// It owns only shell wiring: renderer event callbacks and service registration.
// All session rules (SSH dial → auth → PTY → data plane streaming → route
// bootstrap, telnet/serial/mosh/et starts, exit tracking) live in the shared
// terminaluse.Service, so a future capability dispatch entry point can call
// the same instance.
type TerminalService struct {
	core *terminaluse.Service
}

// NewTerminalService wires the route controller, transport and known-hosts
// store together. The urgent channel (Ctrl-C et al.) is handled in-process by
// writing the payload to the session's stdin.
func NewTerminalService(controller *dataplane.RouteController, dp *dataplane.Server, knownHosts *ssh.KnownHosts) *TerminalService {
	return &TerminalService{core: terminaluse.New(controller, dp, knownHosts)}
}

func (s *TerminalService) setChallengeEmitter(emit func(ssh.KeyboardChallenge)) {
	s.core.SetChallengeEmitter(emit)
}

func (s *TerminalService) setEventEmitter(emit func(name string, payload any)) {
	s.core.SetEventEmitter(emit)
}

func (s *TerminalService) setOutputObserver(observe func(sessionID string, data []byte)) {
	s.core.SetOutputObserver(observe)
}

func (s *TerminalService) setTempService(temp *filesystem.TempService) {
	s.core.SetTempService(temp)
}

// RegisterPluginSession hosts a plugin-protocol connection as a first class
// terminal session (the PluginConnectionSessionSink seam consumed by
// PluginService.SetPluginSessionSink; see internal/app/terminaluse/
// plugin_session.go).
func (s *TerminalService) RegisterPluginSession(sessionID, label string, hooks terminaluse.PluginSessionHooks) error {
	_, err := s.core.RegisterPluginSession(sessionID, label, hooks)
	return err
}

// PublishPluginOutput forwards plugin connection output into the session's
// ordinary data plane (flow control, observers, route publishing).
func (s *TerminalService) PublishPluginOutput(sessionID string, data []byte) bool {
	return s.core.PublishPluginOutput(sessionID, data)
}

// PluginSessionClosed ends a plugin-protocol session from the plugin side
// with ordinary exit tracking.
func (s *TerminalService) PluginSessionClosed(sessionID, reason string) {
	s.core.PluginSessionClosed(sessionID, reason)
}

// CloseSession closes any terminal session through the ordinary close path
// (plugin connections route the close through their plugin hook).
func (s *TerminalService) CloseSession(sessionID string) error {
	return s.core.Close(sessionID)
}

// TransportFor exposes a live session's authenticated SSH client for the SFTP
// subsystem seam; see terminaluse.Service.TransportFor.
func (s *TerminalService) TransportFor(sessionID string) (*gossh.Client, func() bool, error) {
	return s.core.TransportFor(sessionID)
}

func (s *TerminalService) Connect(request SSHConnectRequest) (string, error) {
	return s.core.Connect(request)
}

func (s *TerminalService) StartMosh(request MoshStartRequest) (string, error) {
	return s.core.StartMosh(request)
}

func (s *TerminalService) StartEt(request MoshStartRequest) (string, error) {
	return s.core.StartEt(request)
}

func (s *TerminalService) StartLocal(shell, cwd string, cols, rows uint16) (string, error) {
	return s.core.StartLocal(shell, cwd, cols, rows)
}

func (s *TerminalService) StartLocalWithOptions(request LocalStartRequest) (string, error) {
	return s.core.StartLocalWithOptions(request)
}

func (s *TerminalService) StartTelnet(request TelnetStartRequest) (string, error) {
	return s.core.StartTelnet(request)
}

func (s *TerminalService) StartSerial(request SerialStartRequest) (string, error) {
	return s.core.StartSerial(request)
}

func (s *TerminalService) ListSerialPorts() ([]serialport.Info, error) {
	return s.core.ListSerialPorts()
}

func (s *TerminalService) Bootstrap(sessionID string) (dataplane.RouteBootstrap, error) {
	return s.core.Bootstrap(sessionID)
}

func (s *TerminalService) Reconnect(sessionID string) (dataplane.RouteBootstrap, error) {
	return s.core.Reconnect(sessionID)
}

func (s *TerminalService) ListenAddr() string { return s.core.ListenAddr() }

// RunnerFor exposes the transport-specific command runner for the agent
// job queue (W13). Not a Wails method.
func (s *TerminalService) RunnerFor(sessionID string) (terminaluse.CommandRunner, error) {
	return s.core.RunnerFor(sessionID)
}

func (s *TerminalService) Write(sessionID string, data []byte) (int, error) {
	return s.core.Write(sessionID, data)
}

// TerminalEncodingResult reports the charset applied to a session.
type TerminalEncodingResult struct {
	OK       bool   `json:"ok"`
	Encoding string `json:"encoding"`
}

// SetSessionEncoding pins the terminal input charset for one session. The
// renderer decodes output with the same charset on the data plane; the Go
// side encodes keystrokes before they reach the PTY/telnet/serial stream.
func (s *TerminalService) SetSessionEncoding(sessionID, encoding string) (TerminalEncodingResult, error) {
	if err := s.core.SetSessionEncoding(sessionID, encoding); err != nil {
		return TerminalEncodingResult{OK: false, Encoding: encoding}, err
	}
	return TerminalEncodingResult{OK: true, Encoding: s.core.SessionEncoding(sessionID)}, nil
}

// GetSessionEncoding reports the charset pinned for one session ("" = UTF-8)
// so any attaching display (popups, re-attach) can decode output correctly.
func (s *TerminalService) GetSessionEncoding(sessionID string) string {
	return s.core.SessionEncoding(sessionID)
}

func (s *TerminalService) Resize(sessionID string, cols, rows uint16) error {
	return s.core.Resize(sessionID, cols, rows)
}

func (s *TerminalService) Signal(sessionID, signal string) error {
	return s.core.Signal(sessionID, signal)
}

func (s *TerminalService) Close(sessionID string) error {
	return s.core.Close(sessionID)
}

func (s *TerminalService) CancelZmodem(sessionID string) error {
	return s.core.CancelZmodem(sessionID)
}

func (s *TerminalService) SendZmodem(sessionID, filePath, remoteName, command string) error {
	return s.core.SendZmodem(sessionID, filePath, remoteName, command)
}

func (s *TerminalService) ReceiveZmodem(sessionID, destinationDir string) error {
	return s.core.ReceiveZmodem(sessionID, destinationDir)
}

func (s *TerminalService) SendSerialYmodem(sessionID, filePath string) (ymodem.SendResult, error) {
	return s.core.SendSerialYmodem(sessionID, filePath)
}

func (s *TerminalService) ReceiveSerialYmodem(sessionID, destinationDir string) ([]ymodem.ReceiveResult, error) {
	return s.core.ReceiveSerialYmodem(sessionID, destinationDir)
}

func (s *TerminalService) RespondKeyboardInteractive(requestID string, responses []string, cancelled bool) error {
	return s.core.RespondKeyboardInteractive(requestID, responses, cancelled)
}

func (s *TerminalService) setPassphraseEmitter(emit func(ssh.PassphraseRequest)) {
	s.core.SetPassphraseEmitter(emit)
}

func (s *TerminalService) setHostKeyVerificationEmitter(emit func(ssh.HostKeyVerificationRequest)) {
	s.core.SetHostKeyVerificationEmitter(emit)
}

// RespondPassphrase completes or cancels a pending encrypted-key passphrase
// prompt raised during SSH auth.
func (s *TerminalService) RespondPassphrase(requestID, passphrase string, cancelled bool) error {
	return s.core.RespondPassphrase(requestID, passphrase, cancelled)
}

// RespondHostKeyVerification completes or rejects a pending changed host-key
// confirmation. accept+addToKnownHosts rotates the pinned key; accept alone
// allows exactly this connection.
func (s *TerminalService) RespondHostKeyVerification(requestID string, accept, addToKnownHosts bool) error {
	return s.core.RespondHostKeyVerification(requestID, accept, addToKnownHosts)
}

// ExecCommandRequest is the shell-facing one-shot SSH exec payload.
type ExecCommandRequest = terminaluse.ExecRequest

// ExecCommandResult reports stdout/stderr and the exit status of a one-shot
// SSH exec.
type ExecCommandResult = terminaluse.ExecResult

// ExecCommand runs one command over a fresh authenticated SSH connection
// (ssh-copy-id style key export, remote checks) and closes the transport.
func (s *TerminalService) ExecCommand(request ExecCommandRequest) ExecCommandResult {
	return s.core.Exec(request)
}

func (s *TerminalService) GetTelnetEchoMode(sessionID string) (map[string]any, error) {
	return s.core.GetTelnetEchoMode(sessionID)
}

func (s *TerminalService) GetExitStatus(id string) *TerminalExitStatus {
	return s.core.GetExitStatus(id)
}

func (s *TerminalService) GetSessionRemoteInfo(sessionID string) TerminalRemoteInfo {
	return s.core.GetSessionRemoteInfo(sessionID)
}

func (s *TerminalService) GetSessionPwd(sessionID string, options TerminalPwdOptions) TerminalPwdResult {
	return s.core.GetSessionPwd(sessionID, options)
}

func (s *TerminalService) GetHelperSessionState(sessionID string) (HelperSessionState, error) {
	return s.core.GetHelperSessionState(sessionID)
}

func (s *TerminalService) RestartHelper(sessionID string) (HelperSessionState, error) {
	return s.core.RestartHelper(sessionID)
}

func (s *TerminalService) GetDefaultShell() string { return terminaluse.DefaultShell() }

func (s *TerminalService) ValidatePath(path, kind string) PathValidation {
	return terminaluse.ValidatePath(path, kind)
}

func (s *TerminalService) DiscoverShells() []DiscoveredShell {
	return terminaluse.DiscoverShells()
}

func (s *TerminalService) GetServerStats(ctx context.Context, sessionID string) MonitoringResult {
	return s.core.GetServerStats(ctx, sessionID)
}

func (s *TerminalService) ProbeSystemCapabilities(ctx context.Context, sessionID string) MonitoringResult {
	return s.core.ProbeSystemCapabilities(ctx, sessionID)
}

func (s *TerminalService) ListSystemProcesses(ctx context.Context, sessionID string) MonitoringResult {
	return s.core.ListSystemProcesses(ctx, sessionID)
}

func (s *TerminalService) ListTmuxSessions(ctx context.Context, sessionID string) MonitoringResult {
	return s.core.ListTmuxSessions(ctx, sessionID)
}

func (s *TerminalService) ListDockerContainers(ctx context.Context, sessionID string) MonitoringResult {
	return s.core.ListDockerContainers(ctx, sessionID)
}

func (s *TerminalService) ListDockerImages(ctx context.Context, sessionID string) MonitoringResult {
	return s.core.ListDockerImages(ctx, sessionID)
}

func (s *TerminalService) GetDockerStats(ctx context.Context, options DockerStatsOptions) MonitoringResult {
	return s.core.GetDockerStats(ctx, options)
}

func (s *TerminalService) SignalSystemProcess(ctx context.Context, options ProcessSignalOptions) MonitoringResult {
	return s.core.SignalSystemProcess(ctx, options)
}

func (s *TerminalService) SetupOsc7Tracking(ctx context.Context, sessionID, command string) MonitoringResult {
	return s.core.SetupOSC7Tracking(ctx, sessionID, command)
}

func (s *TerminalService) CreateTmuxSession(ctx context.Context, request TmuxSessionRequest) MonitoringResult {
	return s.core.CreateTmuxSession(ctx, request)
}

func (s *TerminalService) ListTmuxWindows(ctx context.Context, request TmuxTargetRequest) MonitoringResult {
	return s.core.ListTmuxWindows(ctx, request)
}

func (s *TerminalService) ListTmuxPanes(ctx context.Context, request TmuxTargetRequest) MonitoringResult {
	return s.core.ListTmuxPanes(ctx, request)
}

func (s *TerminalService) ListTmuxClients(ctx context.Context, request TmuxTargetRequest) MonitoringResult {
	return s.core.ListTmuxClients(ctx, request)
}

func (s *TerminalService) TmuxAction(ctx context.Context, request TmuxActionRequest) MonitoringResult {
	return s.core.TmuxAction(ctx, request)
}

func (s *TerminalService) ListAccelerators(ctx context.Context, sessionID string) MonitoringResult {
	return s.core.ListAccelerators(ctx, sessionID)
}

func (s *TerminalService) ListListeningPorts(ctx context.Context, sessionID string) MonitoringResult {
	return s.core.ListListeningPorts(ctx, sessionID)
}

func (s *TerminalService) ListSystemServices(ctx context.Context, sessionID string) MonitoringResult {
	return s.core.ListSystemServices(ctx, sessionID)
}

func (s *TerminalService) SystemServiceAction(ctx context.Context, request SystemServiceActionRequest) MonitoringResult {
	return s.core.SystemServiceAction(ctx, request)
}

func (s *TerminalService) DockerInspect(ctx context.Context, request DockerInspectRequest) MonitoringResult {
	return s.core.DockerInspect(ctx, request)
}

func (s *TerminalService) DockerImageInspect(ctx context.Context, request DockerInspectRequest) MonitoringResult {
	return s.core.DockerImageInspect(ctx, request)
}

func (s *TerminalService) DockerAction(ctx context.Context, request DockerActionRequest) MonitoringResult {
	return s.core.DockerAction(ctx, request)
}

func (s *TerminalService) DockerImageAction(ctx context.Context, request DockerImageActionRequest) MonitoringResult {
	return s.core.DockerImageAction(ctx, request)
}

func (s *TerminalService) ListAutocompleteDirectory(ctx context.Context, sessionID, directory string, foldersOnly bool, prefix string, limit int) AutocompleteDirectoryResult {
	return s.core.ListAutocompleteDirectory(ctx, sessionID, directory, foldersOnly, prefix, limit)
}

// TestProxy probes HTTP/SOCKS5/ProxyCommand reachability without opening a session.
func (s *TerminalService) TestProxy(request ProxyProbeRequest) ProxyProbeResult {
	return terminaluse.ProbeProxy(request)
}

// --- Session introspection & attach primitives (History panel, distro probe,
// --- busy-close confirmation, popup attach). All resolve native-or-alias ids.

// GetSessionDistroInfo runs the /etc/os-release probe on the session's
// existing transport (no new SSH connection).
func (s *TerminalService) GetSessionDistroInfo(sessionID string) terminaluse.SessionDistroResult {
	return s.core.GetSessionDistroInfo(sessionID)
}

// ReadRemoteHistory tails the remote shell's history file(s) over one exec
// channel on the existing transport.
func (s *TerminalService) ReadRemoteHistory(sessionID string, limit int) terminaluse.RemoteHistoryResult {
	return s.core.ReadRemoteHistory(sessionID, limit)
}

// PtyGetChildProcesses lists the direct children of a local session's shell
// for the busy-close confirmation.
func (s *TerminalService) PtyGetChildProcesses(sessionID string) []terminaluse.ChildProcessInfo {
	return s.core.PtyGetChildProcesses(sessionID)
}

// AcquireSessionFlowPauseLease pauses renderer-bound output for an attach popup.
func (s *TerminalService) AcquireSessionFlowPauseLease(sessionID string) terminaluse.FlowPauseLease {
	return s.core.AcquireSessionFlowPauseLease(sessionID)
}

// WaitSessionFlowPauseLease waits for pre-pause output to drain.
func (s *TerminalService) WaitSessionFlowPauseLease(sessionID, leaseID string) terminaluse.ActionResult {
	return s.core.WaitSessionFlowPauseLease(sessionID, leaseID)
}

// ReleaseSessionFlowPauseLease drops one lease and flushes buffered output.
func (s *TerminalService) ReleaseSessionFlowPauseLease(sessionID, leaseID string, options *terminaluse.FlowPauseReleaseOptions) terminaluse.ActionResult {
	return s.core.ReleaseSessionFlowPauseLease(sessionID, leaseID, options)
}

// SetSessionFlowPaused pauses/resumes output without a lease.
func (s *TerminalService) SetSessionFlowPaused(sessionID string, paused bool) error {
	return s.core.SetSessionFlowPaused(sessionID, paused)
}

// SetSessionFlowPausedAndWait pauses and waits for the queue to drain.
func (s *TerminalService) SetSessionFlowPausedAndWait(sessionID string, paused bool) terminaluse.ActionResult {
	return s.core.SetSessionFlowPausedAndWait(sessionID, paused)
}

// RequestSessionSnapshot asks the home renderer to serialize its scrollback.
func (s *TerminalService) RequestSessionSnapshot(sessionID, authorization string) terminaluse.SnapshotResult {
	return s.core.RequestSessionSnapshot(sessionID, authorization)
}

// RespondSessionSnapshot completes a pending snapshot request (home renderer).
func (s *TerminalService) RespondSessionSnapshot(
	requestID string,
	snapshot string,
	kittyState *terminaluse.KittyKeyboardModeState,
	kittyEnabled *bool,
	passwordPromptActive *bool,
	cwd *string,
	title *string,
) error {
	return s.core.RespondSessionSnapshot(requestID, snapshot, kittyState, kittyEnabled, passwordPromptActive, cwd, title)
}

// ApplySessionSnapshot pushes the popup display state to the home renderer.
func (s *TerminalService) ApplySessionSnapshot(sessionID, snapshot string, context terminaluse.SnapshotContext, authorization string) terminaluse.ActionResult {
	return s.core.ApplySessionSnapshot(sessionID, snapshot, context, authorization)
}

// RespondApplySnapshot completes a pending apply-snapshot push (home renderer).
func (s *TerminalService) RespondApplySnapshot(requestID string, accepted bool) error {
	return s.core.RespondApplySnapshot(requestID, accepted)
}

// MarkAttachPopupClosePrepared records that the popup began its close.
func (s *TerminalService) MarkAttachPopupClosePrepared(sessionID, authorization string) terminaluse.ActionResult {
	return s.core.MarkAttachPopupClosePrepared(sessionID, authorization)
}

// RebindSessionOutput rotates the data-plane route for the attach popup.
func (s *TerminalService) RebindSessionOutput(sessionID, authorization string) terminaluse.RebindResult {
	return s.core.RebindSessionOutput(sessionID, authorization)
}

// RestoreSessionOutput rotates the route back to the home renderer.
func (s *TerminalService) RestoreSessionOutput(sessionID, authorization string) terminaluse.ActionResult {
	return s.core.RestoreSessionOutput(sessionID, authorization)
}
