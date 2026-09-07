package main

import (
	"errors"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const mainWindowName = "terminal-data-plane"

type probeWindowHost interface {
	OpenPopup(name, path string, onClosed func()) error
	CloseWindow(name string) error
}

type wailsWindowHost struct {
	app *application.App
}

func (p *ProbeService) attachApp(app *application.App, window *application.WebviewWindow) {
	p.mu.Lock()
	p.windowHost = &wailsWindowHost{app: app}
	p.mu.Unlock()
	if !p.autorun.config.Enabled {
		return
	}
	detachApplicationReady := app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		p.autorun.signalReady()
	})
	detachRuntimeReady := window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		p.autorun.signalReady()
	})
	p.autorun.addReadinessDetachers(detachApplicationReady, detachRuntimeReady)
	p.autorun.start(p.ctx, &p.wg, app.Quit)
}

func (w *wailsWindowHost) OpenPopup(name, path string, onClosed func()) error {
	if w.app == nil {
		return errors.New("Wails application is not attached")
	}
	if _, exists := w.app.Window.GetByName(name); exists {
		return fmt.Errorf("window %q already exists", name)
	}
	window := w.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             name,
		Title:            "Terminal Data Plane Probe - Popup",
		Width:            980,
		Height:           680,
		MinWidth:         560,
		MinHeight:        460,
		BackgroundColour: application.NewRGB(18, 20, 23),
		URL:              path,
	})
	window.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		if onClosed != nil {
			onClosed()
		}
	})
	window.Show()
	window.Focus()
	return nil
}

func (w *wailsWindowHost) CloseWindow(name string) error {
	window, exists := w.app.Window.GetByName(name)
	if !exists {
		return fmt.Errorf("window %q not found", name)
	}
	window.Close()
	return nil
}
