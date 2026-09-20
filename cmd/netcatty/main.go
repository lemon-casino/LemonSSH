// Command netcatty is the Wails v3 shell of the Netcatty application.
//
// This package is the only production Go code allowed to import Wails
// (P1-02). It adapts the shell-neutral use cases from internal/app to Wails
// services and loads the existing Vite build of the React frontend.
package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/binaricat/netcatty/internal/agent/drivers/fixture"
	agentruntime "github.com/binaricat/netcatty/internal/agent/runtime"
	"github.com/binaricat/netcatty/internal/agent/tools"
	"github.com/binaricat/netcatty/internal/app"
	"github.com/binaricat/netcatty/internal/app/terminaluse"
	"github.com/binaricat/netcatty/internal/capability"
	"github.com/binaricat/netcatty/internal/platform/applock"
	"github.com/binaricat/netcatty/internal/platform/applog"
	"github.com/binaricat/netcatty/internal/platform/credentials"
	"github.com/binaricat/netcatty/internal/platform/filesystem"
	"github.com/binaricat/netcatty/internal/platform/netpolicy"
	"github.com/binaricat/netcatty/internal/terminal/dataplane"
	"github.com/binaricat/netcatty/internal/terminal/sessionlog"
	"github.com/binaricat/netcatty/internal/terminal/ssh"
	"github.com/binaricat/netcatty/internal/terminal/sshpool"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed appicon.png
var appIcon []byte

// version is injected at build time via -ldflags once a release pipeline
// exists (P6-02). Until then it reports the skeleton version.
var version = "0.0.0-wails-skeleton"

// NetcattyService is the Wails-facing facade. It holds no business logic:
// every call delegates to internal/app use cases.
type NetcattyService struct {
	application *app.App
}

func newNetcattyService(application *app.App) *NetcattyService {
	return &NetcattyService{application: application}
}

// Health reports application liveness to the renderer.
func (s *NetcattyService) Health() (app.HealthStatus, error) {
	return s.application.Health(context.Background())
}

// Version reports build identity to the renderer.
func (s *NetcattyService) Version() (app.VersionInfo, error) {
	return s.application.Version(context.Background())
}

// ResolveWindowRole validates a window role requested by a renderer.
func (s *NetcattyService) ResolveWindowRole(role string) (app.WindowRoleInfo, error) {
	return s.application.ResolveWindowRole(context.Background(), role)
}

func mainWindowOptions() application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Name:             "main",
		Title:            fmt.Sprintf("LemonSSH %s", version),
		Width:            1280,
		Height:           800,
		MinWidth:         960,
		MinHeight:        600,
		Frameless:        true,
		EnableFileDrop:   true,
		BackgroundColour: application.NewRGB(20, 23, 28),
		URL:              "/index.html",
	}
}

func main() {
	// Runtime log: next to the exe when writable, profile dir as fallback.
	if exePath, exeErr := os.Executable(); exeErr == nil {
		if err := applog.Init(filepath.Join(filepath.Dir(exePath), "logs")); err != nil {
			_ = applog.Init(filepath.Join(baseProfileDir(), "logs"))
		}
	} else {
		_ = applog.Init(filepath.Join(baseProfileDir(), "logs"))
	}
	log.SetOutput(applog.Writer())
	defer applog.Close()
	applog.Infof("LemonSSH %s starting", version)

	deepLinkService := newDeepLinkService()
	for _, rawURL := range deepLinkURLsFromArgs(os.Args) {
		_ = deepLinkService.Enqueue(rawURL)
	}
	// Acquire the single-instance lock FIRST: bbolt blocks on the profile
	// store's file lock while another instance runs, which would otherwise
	// hang a second launch before Wails could forward it to the first.
	wailsApp := application.New(application.Options{
		Name:        "LemonSSH",
		Description: "LemonSSH",
		Icon:        appIcon,
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "app.lemonssh.desktop",
			ExitCode: 0,
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				for _, rawURL := range deepLinkURLsFromArgs(data.Args) {
					_ = deepLinkService.Enqueue(rawURL)
				}
				app := application.Get()
				if app == nil {
					return
				}
				win, ok := app.Window.GetByName("main")
				if !ok {
					return
				}
				restoreMainWindow(appRestoreWindow{win})
			},
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})
	core := app.New("LemonSSH", version)
	service := newNetcattyService(core)

	profileStore, err := openProfileStore()
	if err != nil {
		log.Fatalf("open profile store: %v", err)
	}
	defer profileStore.Close()
	profileService := newProfileService(profileStore)
	credentialProvider := credentials.NewOSProvider()
	credentialService := newCredentialService(credentialProvider)
	ptyService := newPTYService()
	appLockService := newAppLockServiceWithDeps(
		applock.New(credentialProvider),
		profileStore,
	)
	pluginService, err := newPluginServiceAt(filepath.Join(filepath.Dir(profileStore.Path()), "plugins", "inventory.json"))
	if err != nil {
		log.Fatalf("open plugin inventory: %v", err)
	}
	managedTemp, err := filesystem.NewTempService(filepath.Join(baseProfileDir(), "temp"))
	if err != nil {
		log.Fatalf("open application temp directory: %v", err)
	}
	sweepTempOrphans(managedTemp)
	filesystemService := newFilesystemService()
	filesystemService.setTempService(managedTemp)
	transferService := newTransferService()
	transferService.setTempService(managedTemp)
	scriptService := newScriptService()
	shortcutService := newNativeShortcutService(func() {
		if win, ok := wailsApp.Window.GetByName("main"); ok {
			if win.IsVisible() {
				win.Hide()
			} else {
				restoreMainWindow(appRestoreWindow{win})
			}
		}
	})
	syncService := newSyncService()
	syncService.setSessionDependencies(profileStore, baseProfileDir(), credentialProvider)
	diagnosticLogService := newDiagnosticLogService()

	// Terminal data plane (loopback WebSocket) + SSH terminal service.
	routeController := dataplane.NewRouteController()
	dpServer := dataplane.NewServer(routeController, "127.0.0.1:0")
	if err := dpServer.Start(); err != nil {
		log.Fatalf("start terminal data plane: %v", err)
	}
	defer dpServer.Stop()
	knownHosts := ssh.NewKnownHosts(filepath.Join(filepath.Dir(profileStore.Path()), "known_hosts"))
	sshPool := sshpool.New(ssh.Dial)
	defer sshPool.Shutdown()
	terminalSvc := NewTerminalService(routeController, dpServer, knownHosts)
	terminalSvc.setChallengeEmitter(func(challenge ssh.KeyboardChallenge) {
		wailsApp.Event.Emit("ssh:keyboard-interactive", challenge)
	})
	terminalSvc.setEventEmitter(func(name string, payload any) {
		wailsApp.Event.Emit(name, payload)
	})
	sftpService := NewSFTPService(sshPool, knownHosts)
	sftpService.setTempService(managedTemp)
	sftpService.setTerminalService(terminalSvc)
	transferService.setSFTPService(sftpService)
	forwardService := NewForwardService(sshPool, knownHosts)
	scriptService.setWriter(func(sessionID string, data []byte) error {
		_, err := terminalSvc.Write(sessionID, data)
		return err
	})
	scriptService.setSessionCloser(terminalSvc.Close)
	sessionLogManager := sessionlog.NewManager(filepath.Join(baseProfileDir(), "session-logs"))
	defer sessionLogManager.CloseAll()
	scriptService.setSessionLog(
		func(sessionID, filePath string) (string, error) {
			return sessionLogManager.Start(sessionID, filePath)
		},
		func(sessionID string) error {
			return sessionLogManager.Stop(sessionID)
		},
	)
	scriptService.setDialogEmitter(func(name string, payload any) {
		wailsApp.Event.Emit(name, payload)
	})
	scriptService.setRunsListener(scriptService.broadcastRuns)
	terminalSvc.setOutputObserver(func(sessionID string, data []byte) {
		scriptService.ObserveOutput(sessionID, data)
		sessionLogManager.Append(sessionID, data)
	})

	// Agent turn runtime (W12). The fixture driver is wired only behind
	// the dev flag; release builds run driver-less so AI calls fail with
	// UNAVAILABLE instead of fixture output.
	turnManager := agentruntime.NewTurnManager()
	devDriver := os.Getenv("NETCATTY_AI_DEV_DRIVER") == "1"
	if devDriver {
		turnManager.SetDriver(fixture.New())
	}
	attachmentRegistry := newAttachmentRegistry()
	// Tool output store (W14): spill goes to the dedicated temp manager.
	outputStore := tools.NewOutputStore(tools.StoreOptions{
		Spill: tempSpillSink{temp: managedTemp},
	})
	// Approval gate (W13): confirm-mode writes prompt the renderer via the
	// agent:interaction event; decisions come back through
	// AgentRespondInteraction.
	interactionRouter := newInteractionRouter(func(name string, payload any) {
		wailsApp.Event.Emit(name, payload)
	})
	agentService := newAgentService(turnManager, devDriver, attachmentRegistry, outputStore, interactionRouter)

	// Agent host (W13): the authenticated loopback RPC surface the native
	// CLI/MCP binaries connect to via the discovery file.
	agentHost := newAgentHost(AgentHostConfig{
		Version: appVersion{
			Name:    "LemonSSH",
			Version: version,
			GOOS:    runtime.GOOS,
			GOARCH:  runtime.GOARCH,
		},
		Jobs:        terminaluse.NewJobQueue(terminalSvc.RunnerFor),
		SFTP:        agentSFTP{service: sftpService},
		Vault:       newVaultReader(profileStore),
		Attachments: attachmentRegistry,
		Forwards:    forwardService,
		Approvals:   interactionRouter,
		VaultRouter: newAgentVaultRouter(func(name string, payload any) { wailsApp.Event.Emit(name, payload) }),
	})
	agentService.host = agentHost

	// Live provider (W15): an explicit provider config takes precedence
	// over the dev fixture; without either, starts fail UNAVAILABLE. The
	// provider dispatcher reuses this host's handler table so model tool
	// calls ride the same handlers as RPC callers.
	providerNetPolicy := netpolicy.New()
	if endpoint := os.Getenv("NETCATTY_AI_ENDPOINT"); endpoint != "" {
		providerNetPolicy.AddProviderEndpoint(endpoint)
	}
	// Renderer provider traffic (settings model discovery + connection probe)
	// shares this policy: the same allowlist authority decides the live
	// provider driver and the renderer-initiated fetches.
	providerFetchService := newProviderFetchService(providerNetPolicy)
	providerFetchService.credentials = credentialProvider
	providerFetchService.emit = func(name string, payload any) { wailsApp.Event.Emit(name, payload) }
	defer providerFetchService.closeStreams()
	providerDispatcher := &capability.Dispatcher{
		Registry:       capability.Default(),
		Surface:        capability.SurfaceBuiltin,
		PermissionMode: capability.ModeAuto,
		Handlers:       agentHost.capabilityHandlers(),
		Approval:       interactionRouter,
	}
	if providerConfig, hasProvider, configErr := loadProviderConfig(); configErr != nil {
		log.Printf("provider config rejected: %v", configErr)
	} else if hasProvider {
		driver, driverErr := buildProviderDriver(providerConfig, providerNetPolicy,
			agentHost.sessions, func() []string { return nil }, providerDispatcher)
		if driverErr != nil {
			log.Printf("provider driver unavailable: %v", driverErr)
		} else {
			driver.dispatchTool = func(ctx context.Context, method string, params map[string]any, chat string) (any, error) {
				return agentHost.dispatch(ctx, method, params, chat, nil)
			}
			turnManager.SetDriver(driver)
			devDriver = true // a live provider makes the Go runtime authoritative
			log.Printf("live provider driver wired: model=%s", providerConfig.Model)
		}
	}
	agentDiscoveryPath := filepath.Join(baseProfileDir(), "agent-rpc-discovery.json")
	if err := agentHost.Start(agentDiscoveryPath); err != nil {
		log.Printf("agent host start failed: %v", err)
	} else {
		defer agentHost.Stop() // also removes the discovery file
	}

	wailsApp.RegisterService(application.NewService(service))
	wailsApp.RegisterService(application.NewService(profileService))
	wailsApp.RegisterService(application.NewService(credentialService))
	wailsApp.RegisterService(application.NewService(ptyService))
	wailsApp.RegisterService(application.NewService(appLockService))
	wailsApp.RegisterService(application.NewService(deepLinkService))
	wailsApp.RegisterService(application.NewService(pluginService))
	wailsApp.RegisterService(application.NewService(terminalSvc))
	wailsApp.RegisterService(application.NewService(sftpService))
	wailsApp.RegisterService(application.NewService(forwardService))
	wailsApp.RegisterService(application.NewService(filesystemService))
	wailsApp.RegisterService(application.NewService(transferService))
	wailsApp.RegisterService(application.NewService(scriptService))
	wailsApp.RegisterService(application.NewService(agentService))
	wailsApp.RegisterService(application.NewService(providerFetchService))
	wailsApp.RegisterService(application.NewService(shortcutService))
	wailsApp.RegisterService(application.NewService(syncService))
	wailsApp.RegisterService(application.NewService(diagnosticLogService))

	mainWindow := wailsApp.Window.NewWithOptions(mainWindowOptions())
	setTaskbarIcon(mainWindow)
	registerFileDrops(mainWindow)
	settingsWindowService := newSettingsWindowService(wailsApp)
	popupWindowService := newPopupWindowService(wailsApp)
	wailsApp.RegisterService(application.NewService(settingsWindowService))
	wailsApp.RegisterService(application.NewService(popupWindowService))
	// Preload the hidden settings window off the boot path: a second
	// WebView during startup delays first paint noticeably.
	mainWindow.RegisterHook(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		time.AfterFunc(2*time.Second, settingsWindowService.Preload)
	})

	// System Tray (P4-03). The context menu follows the Appearance language:
	// the renderer reports locale changes through TrayService.SetLanguage.
	tray := wailsApp.SystemTray.New()
	tray.SetIcon(appIcon)
	tray.SetTooltip("LemonSSH")
	trayService := newTrayService(wailsApp, tray, TrayActions{
		ShowMain: func() {
			if win, ok := wailsApp.Window.GetByName("main"); ok {
				restoreMainWindow(appRestoreWindow{win})
			}
		},
		OpenSettings: func() {
			_, _ = settingsWindowService.Open()
		},
	})
	tray.SetMenu(trayService.initialMenu())
	wailsApp.RegisterService(application.NewService(trayService))

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}
