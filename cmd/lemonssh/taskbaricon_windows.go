//go:build windows

package main

import (
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Wails registers the WebView window class with the stock IDI_APPLICATION
// icon, so the taskbar shows the default Go icon even though the exe embeds
// the LemonSSH icon. Sending WM_SETICON with the embedded resource fixes the
// taskbar (and alt-tab) presentation at runtime.
const (
	imageIcon        = 1 // IMAGE_ICON
	loadDefaultSize  = 0x40
	iconSmall        = 0
	iconBig          = 1
	wmSetIcon        = 0x0080
	trayIconResource = 3 // ID embedded by scripts/wails-resource (rsrc_windows.syso)
)

var (
	user32Taskbar   = syscall.NewLazyDLL("user32.dll")
	procSendMessage = user32Taskbar.NewProc("SendMessageW")
	procLoadImage   = user32Taskbar.NewProc("LoadImageW")

	kernel32Taskbar      = syscall.NewLazyDLL("kernel32.dll")
	procGetModuleHandleW = kernel32Taskbar.NewProc("GetModuleHandleW")
)

// setTaskbarIcon loads the embedded LemonSSH icon and applies it to the
// window. Resource IDs are tried in order so a regenerated .syso keeps
// working even if the embedder changes the icon identifier.
func setTaskbarIcon(window *application.WebviewWindow) {
	if window == nil {
		return
	}
	hwnd := uintptr(window.NativeWindow())
	if hwnd == 0 {
		return
	}
	instance, _, _ := procGetModuleHandleW.Call(0)
	if instance == 0 {
		return
	}
	for _, id := range []uintptr{trayIconResource, 32512, 1} {
		icon, _, _ := procLoadImage.Call(instance, id, imageIcon, 0, 0, loadDefaultSize)
		if icon == 0 {
			continue
		}
		procSendMessage.Call(hwnd, wmSetIcon, iconSmall, icon)
		procSendMessage.Call(hwnd, wmSetIcon, iconBig, icon)
		return
	}
}
