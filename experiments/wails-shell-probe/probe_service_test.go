package main

import (
	"strings"
	"testing"
)

func TestNormalizeRole(t *testing.T) {
	role, err := normalizeRole(" Settings ")
	if err != nil || role != "settings" {
		t.Fatalf("normalizeRole() = %q, %v", role, err)
	}
	if _, err := normalizeRole("admin"); err == nil {
		t.Fatal("normalizeRole() accepted an unsupported role")
	}
}

func TestRoleTokensAreStableAndDistinct(t *testing.T) {
	probe := NewProbeService()
	mainToken := probe.token("main")
	if mainToken != probe.token("main") {
		t.Fatal("main role token changed")
	}
	if mainToken == probe.token("settings") {
		t.Fatal("different roles received the same token")
	}
	if len(mainToken) != 24 {
		t.Fatalf("token length = %d, want 24", len(mainToken))
	}
	if !strings.Contains(probe.windowName("main"), mainToken) {
		t.Fatal("window name does not contain its opaque token")
	}
}

func TestFindProbeDeepLink(t *testing.T) {
	value, ok := findProbeDeepLink([]string{"--flag", "netcatty-wails-probe://open/session?id=7"})
	if !ok || value != "netcatty-wails-probe://open/session?id=7" {
		t.Fatalf("findProbeDeepLink() = %q, %t", value, ok)
	}
	for _, value := range []string{"https://example.com", "netcatty-wails-probe:", "://broken", ""} {
		if isProbeDeepLink(value) {
			t.Fatalf("isProbeDeepLink(%q) = true", value)
		}
	}
}

func TestRecordKeepsBoundedHistory(t *testing.T) {
	probe := NewProbeService()
	for index := 0; index < 120; index++ {
		probe.record("test", "event")
	}
	snapshot, err := probe.Snapshot("main", probe.token("main"))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(snapshot.Events); got != 100 {
		t.Fatalf("event history length = %d, want 100", got)
	}
}

func TestWindowRoleAuthorization(t *testing.T) {
	probe := NewProbeService()
	mainToken := probe.token("main")
	settingsToken := probe.token("settings")

	if _, err := probe.authorize("main", mainToken, "settings"); err != nil {
		t.Fatalf("main role could not control settings: %v", err)
	}
	if target, err := probe.authorize("settings", settingsToken, " Settings "); err != nil || target != "settings" {
		t.Fatalf("settings role could not control itself: %v", err)
	}
	if _, err := probe.authorize("settings", settingsToken, "main"); err == nil {
		t.Fatal("non-main role controlled another role")
	}
	if _, err := probe.authorize("main", "forged", "settings"); err == nil {
		t.Fatal("forged role token was accepted")
	}
	if err := probe.SetCloseVeto("main", mainToken, " Settings ", true); err != nil {
		t.Fatal(err)
	}
	if !probe.closeVeto["settings"] {
		t.Fatal("normalized role did not update the canonical close-veto key")
	}
	if _, exists := probe.closeVeto[" Settings "]; exists {
		t.Fatal("noncanonical close-veto key was created")
	}
}

func TestCloseObservationIsSingleFlight(t *testing.T) {
	probe := NewProbeService()
	if !probe.beginCloseObservation(41) {
		t.Fatal("first close observation was rejected")
	}
	if probe.beginCloseObservation(41) {
		t.Fatal("duplicate close observation was accepted")
	}
	if !probe.beginCloseObservation(42) {
		t.Fatal("another window generation was incorrectly blocked")
	}
	probe.finishCloseObservation(41)
	if !probe.beginCloseObservation(41) {
		t.Fatal("completed close observation was not released")
	}
}
