package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	os.Exit(runApplication())
}

func runApplication() int {
	autorunConfig, err := parseAutorunConfig(os.LookupEnv)
	if err != nil {
		writeAutorunStartupFailure(os.Stdout, "config", "invalid-config")
		return autorunExitFailure
	}
	config := DefaultProbeConfig()
	config.Autorun = autorunConfig
	probe, err := NewProbeService(config)
	if err != nil {
		if autorunConfig.Enabled {
			writeAutorunStartupFailure(os.Stdout, "startup", "service-start-failed")
		} else {
			log.Printf("terminal data-plane startup: %v", err)
			return 1
		}
		return autorunExitFailure
	}
	shutdownProbe := func() {
		if closeErr := probe.shutdown(); closeErr != nil {
			log.Printf("terminal data-plane shutdown: %v", closeErr)
		}
	}

	app := application.New(application.Options{
		Name:        "Netcatty Terminal Data-Plane Probe",
		Description: "Disposable Wails v3 terminal binary transport probe",
		Services: []application.Service{
			application.NewService(probe),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		OnShutdown: shutdownProbe,
	})
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "terminal-data-plane",
		Title:            "Terminal Data Plane Probe",
		Width:            1280,
		Height:           820,
		MinWidth:         720,
		MinHeight:        560,
		BackgroundColour: application.NewRGB(18, 20, 23),
		URL:              "/",
	})
	probe.attachApp(app, window)
	window.Show()
	runErr := app.Run()
	probe.autorun.signalStopped()
	if runErr != nil {
		log.Printf("Wails application stopped with an error: %v", runErr)
	}
	recordAutorunRunFailure(probe.autorun, runErr)
	shutdownProbe()
	probe.autorun.waitForQuitLoop()
	return applicationExitStatus(autorunConfig.Enabled, probe.autorun.outcomeSnapshot(), runErr != nil)
}
