//go:build linux && cgo

package shortcuts

import (
	"fmt"
	"golang.design/x/hotkey"
	"os"
)

func nativeModifiers(raw []string) ([]hotkey.Modifier, error) {
	if os.Getenv("DISPLAY") == "" || os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		return nil, fmt.Errorf("global shortcuts require an X11 session; Wayland is unsupported")
	}
	var out []hotkey.Modifier
	seen := map[hotkey.Modifier]bool{}
	for _, m := range raw {
		var v hotkey.Modifier
		switch m {
		case "ctrl", "control", "cmdorctrl":
			v = hotkey.Modifier(4)
		case "alt", "cmdoralt":
			v = hotkey.Modifier(8)
		case "shift":
			v = hotkey.Modifier(1)
		case "cmd", "super":
			v = hotkey.Modifier(64)
		}
		if !seen[v] {
			out = append(out, v)
			seen[v] = true
		}
	}
	return out, nil
}
