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
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/icons"

	"github.com/binaricat/netcatty/internal/app"
	"github.com/binaricat/netcatty/internal/platform/credentials"
	"github.com/binaricat/netcatty/internal/terminal/dataplane"
	"github.com/binaricat/netcatty/internal/terminal/ssh"
	"github.com/binaricat/netcatty/internal/terminal/sshpool"
)

//go:embed all:frontend/dist
var assets embed.FS

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
		BackgroundColour: application.NewRGB(20, 23, 28),
		URL:              "/index.html",
	}
}

func main() {
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
	migrationService := newProfileMigrationService(credentialProvider, filepath.Dir(profileStore.Path()))
	ptyService := newPTYService()
		upgradeService := newUpgradeService(filepath.Dir(profileStore.Path()))
		appLockService := newAppLockService()
		deepLinkService := newDeepLinkService()
		pluginService := newPluginService()

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
		sftpService := NewSFTPService(sshPool, knownHosts)
		forwardService := NewForwardService(sshPool, knownHosts)

	wailsApp := application.New(application.Options{
		Name:        "LemonSSH",
		Description: "LemonSSH",
		Services: []application.Service{
			application.NewService(service),
			application.NewService(profileService),
			application.NewService(credentialService),
			application.NewService(migrationService),
			application.NewService(ptyService),
				application.NewService(upgradeService),
				application.NewService(appLockService),
				application.NewService(deepLinkService),
				application.NewService(pluginService),
			application.NewService(terminalSvc),
				application.NewService(sftpService),
				application.NewService(forwardService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

		wailsApp.Window.NewWithOptions(mainWindowOptions())
		settingsWindowService := newSettingsWindowService(wailsApp)
		wailsApp.RegisterService(application.NewService(settingsWindowService))

		// System Tray (P4-03)
	tray := wailsApp.SystemTray.New()
	tray.SetIcon(icons.SystrayLight)
		tray.SetTooltip("LemonSSH")
		trayMenu := wailsApp.NewMenu()
		trayMenu.Add("Show LemonSSH").OnClick(func(*application.Context) {
			if win, ok := wailsApp.Window.GetByName("main"); ok {
				win.Show()
				win.Focus()
			}
		})
		trayMenu.Add("Settings").OnClick(func(*application.Context) {
			_, _ = settingsWindowService.Open()
		})
		trayMenu.Add("Quit").OnClick(func(*application.Context) { wailsApp.Quit() })
		tray.SetMenu(trayMenu)

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}
