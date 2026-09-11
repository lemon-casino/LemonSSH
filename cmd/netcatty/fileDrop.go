package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

func registerFileDrops(window *application.WebviewWindow) {
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		files := event.Context().DroppedFiles()
		if len(files) == 0 {
			return
		}
		target := event.Context().DropTargetDetails()
		payload := map[string]any{"filenames": files}
		if target != nil {
			payload["x"] = target.X
			payload["y"] = target.Y
			payload["elementDetails"] = target
		}
		window.EmitEvent("netcatty:files-dropped", payload)
	})
}
