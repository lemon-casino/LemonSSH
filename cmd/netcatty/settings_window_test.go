package main

import "testing"

func TestSettingsWindowOptionsAreDedicatedAndFrameless(t *testing.T) {
	options := settingsWindowOptions()
	if options.Name != "settings" {
		t.Fatalf("settings window name = %q, want settings", options.Name)
	}
	if !options.Frameless {
		t.Fatal("settings window must be frameless")
	}
	if options.URL != "/index.html#/settings" {
		t.Fatalf("settings window URL = %q", options.URL)
	}
	if options.Width != 980 || options.Height != 720 {
		t.Fatalf("settings window size = %dx%d", options.Width, options.Height)
	}
	if !options.Hidden {
		t.Fatal("settings window must start hidden until the frontend has painted")
	}
}
