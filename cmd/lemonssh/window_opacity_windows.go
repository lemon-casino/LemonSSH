//go:build windows

package main

import (
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Wails v3 (v3.0.0-beta.12) exposes no cross-platform opacity API on
// WebviewWindow, so the Windows shell drives the same layered-window
// mechanism Electron's win.setOpacity used: WS_EX_LAYERED plus per-pixel
// alpha through SetLayeredWindowAttributes.
const (
	// gwlExstyle is GWL_EXSTYLE (-20) as the unsigned index user32 expects.
	gwlExstyle  = ^uintptr(19)
	wsExLayered = 0x00080000
	lwaAlpha    = 0x00000002
)

var user32Opacity = syscall.NewLazyDLL("user32.dll")

var (
	procGetWindowLongPtrW          = user32Opacity.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW          = user32Opacity.NewProc("SetWindowLongPtrW")
	procSetLayeredWindowAttributes = user32Opacity.NewProc("SetLayeredWindowAttributes")
)

// applyWindowOpacity applies whole-window transparency to the given window.
// Runs on the main thread because window styles must be owned by the
// window's creating thread.
func applyWindowOpacity(window *application.WebviewWindow, opacity float64) bool {
	if window == nil {
		return false
	}
	alpha, keepLayered, ok := windowOpacityPlan(opacity)
	if !ok {
		return false
	}
	hwnd := uintptr(window.NativeWindow())
	if hwnd == 0 {
		return false
	}
	applied := make(chan bool, 1)
	application.InvokeSync(func() {
		applied <- setLayeredWindowAlpha(hwnd, alpha, keepLayered)
	})
	return <-applied
}

func setLayeredWindowAlpha(hwnd uintptr, alpha uint32, keepLayered bool) bool {
	previous, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlExstyle)
	if keepLayered {
		if previous&wsExLayered == 0 {
			// Last error is unreliable around style changes; treat a failed
			// style update as unsupported rather than silently no-op alpha.
			if result, _, _ := procSetWindowLongPtrW.Call(hwnd, gwlExstyle, previous|wsExLayered); result == 0 {
				return false
			}
		}
		result, _, _ := procSetLayeredWindowAttributes.Call(hwnd, 0, uintptr(uint8(alpha)), lwaAlpha)
		return result != 0
	}
	// Fully opaque: drop the layered style again so DWM stops compositing
	// the window as translucent.
	if previous&wsExLayered != 0 {
		procSetWindowLongPtrW.Call(hwnd, gwlExstyle, previous&^uintptr(wsExLayered))
	}
	return true
}
