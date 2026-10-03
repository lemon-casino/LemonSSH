//go:build !windows

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// applyWindowOpacity reports unsupported outside Windows: the Wails v3 build
// pinned by go.mod (v3.0.0-beta.12) exposes no opacity control for macOS or
// Linux windows (its Linux GTK backend has an internal setOpacity that is not
// reachable through the public API). The settings UI already documents the
// limitation and the renderer treats a false result as "not applied".
func applyWindowOpacity(window *application.WebviewWindow, opacity float64) bool {
	_ = window
	_ = opacity
	return false
}
