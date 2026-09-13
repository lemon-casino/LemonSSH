package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/binaricat/netcatty/internal/platform/credentials"
	"github.com/binaricat/netcatty/internal/profile/store"
)

func TestResetSyncEverythingRemovesOAuthClientIDs(t *testing.T) {
	profilePath := filepath.Join(t.TempDir(), "profile.db")
	profile, err := store.Open(profilePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = profile.Close() })

	reset := newCloudSyncResetService(profile, newCloudSyncSessionPassword(t.TempDir(), credentials.New(nil)))
		const (
			oauthKey    = "netcatty_sync_oauth_client_ids_v1"
			secretsKey  = "netcatty_sync_oauth_client_secrets_v1"
			masterKey   = "netcatty_master_key_config_v1"
			themeKey    = "netcatty_theme_v1"
			hostsKey    = "netcatty_hosts_v1"
		)
		if err := profile.SetRaw("settings", oauthKey, []byte(`{"github":"Ov23licrO6aqtR2h1WBC","google":"desktop.apps.googleusercontent.com"}`)); err != nil {
			t.Fatal(err)
		}
		if err := profile.SetRaw("settings", secretsKey, []byte(`{"google":"GOCSPX-fixture"}`)); err != nil {
			t.Fatal(err)
		}
	if err := profile.SetRaw("settings", masterKey, []byte(`{"salt":"x"}`)); err != nil {
		t.Fatal(err)
	}
	if err := profile.SetRaw("settings", themeKey, []byte("dark")); err != nil {
		t.Fatal(err)
	}
	if err := profile.SetRaw("vault", hostsKey, []byte("[]")); err != nil {
		t.Fatal(err)
	}

	removed, err := reset.ResetSyncEverything(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	foundOAuth := false
	for _, key := range removed {
		if key == oauthKey {
			foundOAuth = true
		}
	}
	if !foundOAuth {
		t.Fatalf("reset must report the OAuth client IDs key, got %v", removed)
	}

		if _, err := profile.GetRaw("settings", oauthKey); !errors.Is(err, store.ErrNoSuchKey) {
			t.Fatalf("OAuth client IDs must be gone after reset, got %v", err)
		}
		if _, err := profile.GetRaw("settings", secretsKey); !errors.Is(err, store.ErrNoSuchKey) {
			t.Fatalf("OAuth client secrets must be gone after reset, got %v", err)
		}
	if _, err := profile.GetRaw("settings", masterKey); !errors.Is(err, store.ErrNoSuchKey) {
		t.Fatalf("master key config must be gone after reset, got %v", err)
	}
	theme, err := profile.GetRaw("settings", themeKey)
	if err != nil || string(theme) != "dark" {
		t.Fatalf("unrelated settings must survive reset: %s %v", theme, err)
	}
	hosts, err := profile.GetRaw("vault", hostsKey)
	if err != nil || string(hosts) != "[]" {
		t.Fatalf("local vault data must survive reset: %s %v", hosts, err)
	}
}

func TestIsResetSyncProfileKeyIncludesOAuthClientIDs(t *testing.T) {
		if !isResetSyncProfileKey("netcatty_sync_oauth_client_ids_v1") {
			t.Fatal("OAuth client IDs are part of cloud-sync identity and must be reset")
		}
		if !isResetSyncProfileKey("netcatty_sync_oauth_client_secrets_v1") {
			t.Fatal("OAuth client secrets are part of cloud-sync identity and must be reset")
		}
	if isResetSyncProfileKey("netcatty_theme_v1") {
		t.Fatal("theme is not cloud-sync identity")
	}
}
