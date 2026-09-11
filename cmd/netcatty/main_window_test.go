package main

import "testing"

func TestMainWindowIsFrameless(t *testing.T) {
	options := mainWindowOptions()
	if !options.Frameless {
		t.Fatal("main window must be frameless so the in-app titlebar is the only window chrome")
	}
	if options.DisableResize {
		t.Fatal("frameless main window must remain resizable")
	}
}

func TestTerminalWindowsAcceptNativeFileDrops(t *testing.T) {
	if !mainWindowOptions().EnableFileDrop || !popupWindowOptions("popup").EnableFileDrop {
		t.Fatal("main and popup terminals must accept native file drops")
	}
}
