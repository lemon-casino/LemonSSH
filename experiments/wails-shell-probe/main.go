package main

import (
	"embed"
	"log"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/icons"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[ProbeEvent]("probe:event")
}

func main() {
	probe := NewProbeService()
	app := application.New(application.Options{
		Name:        "Netcatty Wails Shell Probe",
		Description: "Disposable Wails v3 shell feasibility probe",
		Services: []application.Service{
			application.NewService(probe),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               "com.netcatty.wails-shell-probe",
			OnSecondInstanceLaunch: probe.handleSecondInstance,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Windows: application.WindowsOptions{DisableQuitOnLastWindowClosed: true},
		Linux:   application.LinuxOptions{DisableQuitOnLastWindowClosed: true},
		WarningHandler: func(message string) {
			probe.record("warning", message)
		},
		ErrorHandler: func(err error) {
			probe.record("error", err.Error())
		},
	})
	probe.attach(app)

	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(event *application.ApplicationEvent) {
		probe.handleDeepLink(event.Context().URL(), "application-event")
	})

	if _, err := probe.openWindow("main"); err != nil {
		log.Fatal(err)
	}

	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(icons.SystrayMacTemplate)
	} else {
		tray.SetIcon(icons.SystrayLight)
	}
	tray.SetTooltip("Netcatty Wails shell probe")
	showMain := func() {
		if _, err := probe.openWindow("main"); err != nil {
			probe.record("window-open-error", err.Error())
		}
	}
	tray.OnClick(showMain)
	menu := app.NewMenu()
	menu.Add("Show probe").OnClick(func(*application.Context) { showMain() })
	menu.Add("Quit").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)

	if err := app.GlobalShortcut.Register("CmdOrCtrl+Shift+F12", func() {
		showMain()
		probe.record("global-shortcut", "CmdOrCtrl+Shift+F12 fired")
	}); err != nil {
		probe.record("global-shortcut-error", err.Error())
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
