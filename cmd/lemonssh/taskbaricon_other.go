//go:build !windows

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// setTaskbarIcon is a no-op off Windows: the taskbar icon comes from the
// bundled .desktop file (Linux) or Info.plist (macOS).
func setTaskbarIcon(*application.WebviewWindow) {}
