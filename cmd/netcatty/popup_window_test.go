package main

import "testing"

func TestPopupWindowOptionsTargetTerminalPopupRoute(t *testing.T) {
	options := popupWindowOptions("popup-1")
	if options.Name != "popup-1" {
		t.Fatalf("popup window name = %q", options.Name)
	}
	if options.URL != "/index.html#/terminal-popup" {
		t.Fatalf("popup window URL = %q", options.URL)
	}
	if !options.Frameless {
		t.Fatal("popup window must be frameless")
	}
	if options.Width < 640 || options.Height < 400 {
		t.Fatalf("popup window too small: %dx%d", options.Width, options.Height)
	}
}
