//go:build darwin && cgo

package shortcuts

import "golang.design/x/hotkey"

func nativeModifiers(raw []string) ([]hotkey.Modifier, error) {
	var out []hotkey.Modifier
	seen := map[hotkey.Modifier]bool{}
	for _, m := range raw {
		var v hotkey.Modifier
		switch m {
		case "ctrl", "control":
			v = hotkey.ModCtrl
		case "alt":
			v = hotkey.ModOption
		case "shift":
			v = hotkey.ModShift
		case "cmd", "super", "cmdorctrl", "cmdoralt":
			v = hotkey.ModCmd
		}
		if !seen[v] {
			out = append(out, v)
			seen[v] = true
		}
	}
	return out, nil
}
