package main

import (
	"context"

	"github.com/binaricat/netcatty/internal/app/terminaluse"
	"github.com/binaricat/netcatty/internal/platform/filesystem"
	"github.com/binaricat/netcatty/internal/terminal/dataplane"
	"github.com/binaricat/netcatty/internal/terminal/serialport"
	"github.com/binaricat/netcatty/internal/terminal/ssh"
	"github.com/binaricat/netcatty/internal/terminal/ymodem"
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
