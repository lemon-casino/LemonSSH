package main

import (
	windowowner "github.com/binaricat/netcatty/internal/terminal/windows"
	"testing"
	"time"
)

func TestPopupConfigAndHeartbeatRequireOwningTokenAndRole(t *testing.T) {
	s := newPopupWindowService(nil)
	popup, _ := s.owner.Create(windowowner.RolePopup)
	record := &popupRecord{identity: popup, config: map[string]any{"sessionId": "private-session"}, timer: time.NewTimer(time.Hour), lastSeen: time.Now()}
	defer record.timer.Stop()
	s.popups[popup.ID] = record
	if _, err := s.GetConfig(popup.ID, "wrong"); err == nil {
		t.Fatal("config disclosed to wrong token")
	}
	if err := s.Heartbeat(popup.ID, "wrong"); err == nil {
		t.Fatal("wrong token extended lease")
	}
	config, err := s.GetConfig(popup.ID, popup.Token)
	if err != nil || config["sessionId"] != "private-session" {
		t.Fatalf("owner config: %v %v", config, err)
	}
	config["sessionId"] = "tampered"
	next, _ := s.GetConfig(popup.ID, popup.Token)
	if next["sessionId"] != "private-session" {
		t.Fatal("caller mutated stored config")
	}
	settings, _ := s.owner.Create(windowowner.RoleSettings)
	s.popups[settings.ID] = record
	if _, err := s.GetConfig(settings.ID, settings.Token); err == nil {
		t.Fatal("settings role read terminal config")
	}
}

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
