//go:build windows || ((linux || darwin) && cgo)

package shortcuts

import (
	"fmt"
	"golang.design/x/hotkey"
)

// RegisterNative owns the OS registration until its returned release function succeeds.
// dispatch must execute native operations on the GUI thread on macOS.
func RegisterNative(raw string, callback func(), dispatch func(func())) (func() error, error) {
	a, err := ParseAccelerator(raw)
	if err != nil {
		return nil, err
	}
	keys := map[string]hotkey.Key{
		"SPACE": hotkey.KeySpace, "ENTER": hotkey.KeyReturn, "RETURN": hotkey.KeyReturn, "ESCAPE": hotkey.KeyEscape, "TAB": hotkey.KeyTab, "DELETE": hotkey.KeyDelete,
		"LEFT": hotkey.KeyLeft, "RIGHT": hotkey.KeyRight, "UP": hotkey.KeyUp, "DOWN": hotkey.KeyDown,
		"A": hotkey.KeyA, "B": hotkey.KeyB, "C": hotkey.KeyC, "D": hotkey.KeyD, "E": hotkey.KeyE, "F": hotkey.KeyF, "G": hotkey.KeyG, "H": hotkey.KeyH, "I": hotkey.KeyI, "J": hotkey.KeyJ, "K": hotkey.KeyK, "L": hotkey.KeyL, "M": hotkey.KeyM, "N": hotkey.KeyN, "O": hotkey.KeyO, "P": hotkey.KeyP, "Q": hotkey.KeyQ, "R": hotkey.KeyR, "S": hotkey.KeyS, "T": hotkey.KeyT, "U": hotkey.KeyU, "V": hotkey.KeyV, "W": hotkey.KeyW, "X": hotkey.KeyX, "Y": hotkey.KeyY, "Z": hotkey.KeyZ,
		"0": hotkey.Key0, "1": hotkey.Key1, "2": hotkey.Key2, "3": hotkey.Key3, "4": hotkey.Key4, "5": hotkey.Key5, "6": hotkey.Key6, "7": hotkey.Key7, "8": hotkey.Key8, "9": hotkey.Key9,
		"F1": hotkey.KeyF1, "F2": hotkey.KeyF2, "F3": hotkey.KeyF3, "F4": hotkey.KeyF4, "F5": hotkey.KeyF5, "F6": hotkey.KeyF6, "F7": hotkey.KeyF7, "F8": hotkey.KeyF8, "F9": hotkey.KeyF9, "F10": hotkey.KeyF10, "F11": hotkey.KeyF11, "F12": hotkey.KeyF12,
	}
	key, ok := keys[a.Key]
	if !ok {
		return nil, fmt.Errorf("unsupported native hotkey key %q", a.Key)
	}
	mods, err := nativeModifiers(a.Modifiers)
	if err != nil {
		return nil, err
	}
	if callback == nil || dispatch == nil {
		return nil, fmt.Errorf("native hotkey application owner unavailable")
	}
	hk := hotkey.New(mods, key)
	dispatch(func() { err = hk.Register() })
	if err != nil {
		return nil, err
	}
	down, up := hk.Keydown(), hk.Keyup()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for down != nil || up != nil {
			select {
			case _, ok := <-down:
				if !ok {
					down = nil
				} else {
					callback()
				}
			case _, ok := <-up:
				if !ok {
					up = nil
				}
			}
		}
	}()
	return func() error {
		var releaseErr error
		dispatch(func() { releaseErr = hk.Unregister() })
		if releaseErr == nil {
			<-done
		}
		return releaseErr
	}, nil
}
