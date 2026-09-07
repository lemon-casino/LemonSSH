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

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/binaricat/netcatty/internal/app"
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

func main() {
	core := app.New("Netcatty", version)
	service := newNetcattyService(core)

	wailsApp := application.New(application.Options{
		Name:        "Netcatty",
		Description: "Netcatty Wails shell",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            fmt.Sprintf("Netcatty %s", version),
		Width:            1280,
		Height:           800,
		MinWidth:         960,
		MinHeight:        600,
		BackgroundColour: application.NewRGB(20, 23, 28),
		URL:              "/index.html",
	})

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}
