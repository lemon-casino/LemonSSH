//go:build windows

package shortcuts

import "testing"

func TestNativeWindowsRegistrationConflictAndRelease(t *testing.T) {
	dispatch := func(fn func()) { fn() }
	release, err := RegisterNative("Ctrl+Alt+Shift+F11", func() {}, dispatch)
	if err != nil {
		t.Fatalf("RegisterHotKey failed: %v", err)
	}
	active := true
	defer func() {
		if active {
			_ = release()
		}
	}()
	if second, err := RegisterNative("Ctrl+Alt+Shift+F11", func() {}, dispatch); err == nil {
		_ = second()
		t.Fatal("OS accepted duplicate hotkey")
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	active = false
	again, err := RegisterNative("Ctrl+Alt+Shift+F11", func() {}, dispatch)
	if err != nil {
		t.Fatalf("released key remained reserved: %v", err)
	}
	if err := again(); err != nil {
		t.Fatal(err)
	}
}
