package main

import (
	windowowner "github.com/lemon-casino/lemonssh/internal/terminal/windows"
	"testing"
	"time"
)

func TestSessionWindowConfigAndHeartbeatRequireOwningTokenAndRole(t *testing.T) {
	s := newSessionWindowService(nil)
	window, _ := s.owner.Create(windowowner.RoleSession)
	record := &sessionWindowRecord{identity: window, config: map[string]any{"sourceSession": "private-session"}, timer: time.NewTimer(time.Hour), lastSeen: time.Now()}
	defer record.timer.Stop()
	s.windows[window.ID] = record

	if _, err := s.GetConfig(window.ID, "wrong"); err == nil {
		t.Fatal("config disclosed to wrong token")
	}
	if err := s.Heartbeat(window.ID, "wrong"); err == nil {
		t.Fatal("wrong token extended lease")
	}
	config, err := s.GetConfig(window.ID, window.Token)
	if err != nil || config["sourceSession"] != "private-session" {
		t.Fatalf("owner config: %v %v", config, err)
	}
	// The stored payload must survive caller-side mutation of the returned map.
	config["sourceSession"] = "tampered"
	next, _ := s.GetConfig(window.ID, window.Token)
	if next["sourceSession"] != "private-session" {
		t.Fatal("caller mutated stored config")
	}

	// Session config must not be readable through a main/settings identity.
	main, _ := s.owner.Create(windowowner.RoleMain)
	s.windows[main.ID] = record
	if _, err := s.GetConfig(main.ID, main.Token); err == nil {
		t.Fatal("main role read session-window config")
	}
}

func TestSessionWindowOpenRequiresAttachedApplication(t *testing.T) {
	s := newSessionWindowService(nil)
	result := s.Open(map[string]any{"title": "Prod"})
	if result.Success || result.Error == "" {
		t.Fatalf("Open without an app must fail closed: %+v", result)
	}
}

func TestSessionWindowOptionsTargetSessionWindowRoute(t *testing.T) {
	options := sessionWindowOptions("session-1", "Prod SSH")
	if options.Name != "session-1" {
		t.Fatalf("session window name = %q", options.Name)
	}
	if options.Title != "Prod SSH" {
		t.Fatalf("session window title = %q", options.Title)
	}
	if sessionWindowOptions("session-1", "").Title != "LemonSSH" {
		t.Fatal("empty title must fall back to the app name")
	}
	if options.URL != "/index.html#/session-window" {
		t.Fatalf("session window URL = %q", options.URL)
	}
	if !options.Frameless {
		t.Fatal("session window must be frameless so the in-app titlebar is the only chrome")
	}
	if !options.EnableFileDrop {
		t.Fatal("session terminals must accept native file drops")
	}
	if options.Width != 1280 || options.Height != 800 || options.MinWidth != 960 || options.MinHeight != 600 {
		t.Fatalf("session window size = %dx%d (min %dx%d)", options.Width, options.Height, options.MinWidth, options.MinHeight)
	}
	if options.Hidden {
		t.Fatal("session window must be visible immediately; the payload arrives via pull handshake")
	}
}

func TestSessionWindowURLCarriesIdentityAndRoute(t *testing.T) {
	url := sessionWindowURL("win id/1", "tok&en")
	if url != "/index.html?sessionWindowId=win+id%2F1&sessionWindowToken=tok%26en#/session-window" {
		t.Fatalf("session window URL = %q", url)
	}
}
