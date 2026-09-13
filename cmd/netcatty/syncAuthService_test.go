package main

import (
	"context"
	"strings"
	"testing"
)

func TestOpenProviderConsoleAllowlist(t *testing.T) {
	original := openExternalLauncher
	openExternalLauncher = func(_ context.Context, rawURL string) error {
		launcherCalls = append(launcherCalls, rawURL)
		return nil
	}
	t.Cleanup(func() { openExternalLauncher = original })

	service := &SyncService{}
	if err := service.OpenProviderConsole(context.Background(), "unknown"); err == nil {
		t.Fatal("unknown provider must be rejected")
	}
	if len(launcherCalls) != 0 {
		t.Fatal("unknown provider must not reach the launcher")
	}

	providers := map[string]string{
		"github":   "https://github.com/settings/developers",
		"google":   "https://console.cloud.google.com/apis/credentials",
		"onedrive": "https://portal.azure.com/#blade/Microsoft_AAD_RegisteredApps/ApplicationsListBlade",
	}
	for _, provider := range []string{"github", "google", "onedrive"} {
		if err := service.OpenProviderConsole(context.Background(), provider); err != nil {
			t.Fatalf("provider %q: %v", provider, err)
		}
	}
	_ = providers
	wantURLs := []string{
		"https://github.com/settings/developers",
		"https://console.cloud.google.com/apis/credentials",
		"https://portal.azure.com/#blade/Microsoft_AAD_RegisteredApps/ApplicationsListBlade",
	}
	if strings.Join(launcherCalls, "|") != strings.Join(wantURLs, "|") {
		t.Fatalf("launcher URLs = %v", launcherCalls)
	}
}

var launcherCalls []string
