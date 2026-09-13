//go:build windows

package shortcuts

import "golang.design/x/hotkey"

func nativeModifiers(raw []string) ([]hotkey.Modifier, error) {
	var out []hotkey.Modifier
	seen := map[hotkey.Modifier]bool{}
	for _, m := range raw {
		var v hotkey.Modifier
		switch m {
		case "ctrl", "control", "cmdorctrl":
			v = hotkey.ModCtrl
		case "alt", "cmdoralt":
			v = hotkey.ModAlt
		case "shift":
			v = hotkey.ModShift
		case "cmd", "super":
			v = hotkey.ModWin
		}
		if !seen[v] {
			out = append(out, v)
			seen[v] = true
		}
	}
	return out, nil
}
