package main

import "testing"

func TestWailsAcceleratorMapsCmdOrCtrl(t *testing.T) {
	got, err := wailsAccelerator("CmdOrCtrl+Shift+K")
	if err != nil {
		t.Fatal(err)
	}
	if got != "CmdOrCtrl+Shift+K" && got != "Ctrl+Shift+K" && got != "Cmd+Shift+K" {
		t.Fatalf("accelerator = %q", got)
	}
}

func TestWailsAcceleratorRejectsEmpty(t *testing.T) {
	if _, err := wailsAccelerator("   "); err == nil {
		t.Fatal("empty accelerator must fail closed")
	}
}

func TestShortcutRegisterFailsClosedWithoutNativeHotkeys(t *testing.T) {
	service := newShortcutService()
	result := service.Register("CmdOrCtrl+Shift+K")
	if result.Success {
		t.Fatal("alpha.63 must not claim native global hotkey success")
	}
	if result.Error == "" {
		t.Fatal("failure must explain that native hotkeys are unavailable")
	}
}
