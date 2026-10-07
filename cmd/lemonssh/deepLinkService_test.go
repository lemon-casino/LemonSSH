package main

import (
	"reflect"
	"testing"
)

// TestDeepLinkURLsFromArgsKeepsAllFiveSchemes pins the launch-argument filter
// to all five schemes: ssh/telnet are direct, lemonssh is the app scheme,
// netcatty is the legacy app scheme (pre-rename OS registrations), and jms://
// is the JumpServer handoff — dropping any of them breaks existing flows.
func TestDeepLinkURLsFromArgsKeepsAllFiveSchemes(t *testing.T) {
	args := []string{
		"--some-flag",
		"ssh://user@example.com:2222",
		"C:\\some\\file.txt",
		"telnet://router.local:23",
		"LEMONSSH://ssh/caps.example.com",
		"netcatty://ssh/legacy.example.com",
		"jms://jump.example.com/token",
		"--open-terminal",
	}
	got := deepLinkURLsFromArgs(args)
	want := []string{
		"ssh://user@example.com:2222",
		"telnet://router.local:23",
		"LEMONSSH://ssh/caps.example.com",
		"netcatty://ssh/legacy.example.com",
		"jms://jump.example.com/token",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("deepLinkURLsFromArgs = %v, want %v", got, want)
	}
}

func TestDeepLinkURLsFromArgsIgnoresNonLinks(t *testing.T) {
	got := deepLinkURLsFromArgs([]string{"plain-arg", "https://example.com", "lemonssh:/missing-slash"})
	if len(got) != 0 {
		t.Fatalf("non deep-link arguments must be ignored, got %v", got)
	}
}
