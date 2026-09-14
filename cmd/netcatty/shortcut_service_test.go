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

func TestWailsAcceleratorMapsUIRecording(t *testing.T) {
	got, err := wailsAccelerator("Ctrl + `")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Ctrl+GRAVE" {
		t.Fatalf("accelerator = %q", got)
	}
	got, err = wailsAccelerator("⌘ + Space")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Cmd+SPACE" {
		t.Fatalf("accelerator = %q", got)
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
	if service.Status().Enabled || len(service.List()) != 0 {
		t.Fatal("failed native registration leaked into active registry")
	}
}

func TestShortcutRegisterAcceptsUIRecording(t *testing.T) {
	service := newShortcutService()
	var seen string
	service.registerNative = func(raw string) (func() error, error) {
		seen = raw
		return func() error { return nil }, nil
	}
	result := service.Register("Ctrl + `")
	if !result.Success {
		t.Fatalf("register failed: %+v", result)
	}
	if seen != "Ctrl+GRAVE" {
		t.Fatalf("native accel = %q", seen)
	}
	if !service.Status().Enabled {
		t.Fatal("registered UI hotkey must show as enabled")
	}
}

func TestOSProtocolStatusReadable(t *testing.T) {
	result := newDeepLinkService().GetOSProtocolStatus()
	if !result.Success {
		t.Fatalf("status read must succeed, got %q", result.Error)
	}
	// The value depends on the machine; either state is a valid read.
	_ = result.Registered
}

func TestBiometricUnlockFailsClosed(t *testing.T) {
	result := newAppLockService().UnlockWithBiometrics()
	if result.Success {
		t.Fatal("must not claim Hello or Touch ID success")
	}
	if result.Error == "" {
		t.Fatal("failure must name the missing biometric owner")
	}
}
