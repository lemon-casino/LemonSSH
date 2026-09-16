package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/binaricat/netcatty/internal/platform/credentials"
	"github.com/binaricat/netcatty/internal/profile/store"
)

// cloudSyncSessionPassword holds the vault master key for the running
// session so auto-sync can re-encrypt without re-prompting. It mirrors the
// Electron semantics: in-memory for the session, plus an OS-keyring-sealed
// copy in the profile directory so a restart remembers the key until the
// user resets the vault.
type cloudSyncSessionPassword struct {
	mu       sync.Mutex
	password string
	known    bool
	provider credentials.Provider
	filePath string
}

const cloudSyncPasswordPurpose = "cloudsync:session-password"

func newCloudSyncSessionPassword(profileDir string, provider credentials.Provider) *cloudSyncSessionPassword {
	return &cloudSyncSessionPassword{
		provider: provider,
		filePath: filepath.Join(profileDir, "cloudsync", "session-password"),
	}
}

func (s *cloudSyncSessionPassword) Set(password string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if password == "" {
		s.password, s.known = "", true
		_ = os.Remove(s.filePath)
		return true
	}
	s.password, s.known = password, true
	sealed, err := s.provider.Seal([]byte(password), cloudSyncPasswordPurpose)
	if err != nil {
		return false
	}
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0o700); err != nil {
		return false
	}
	return os.WriteFile(s.filePath, sealed, 0o600) == nil
}

func (s *cloudSyncSessionPassword) Get() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.known {
		return s.password, true
	}
	sealed, err := os.ReadFile(s.filePath)
	if err != nil {
		return "", false
	}
	password, err := s.provider.Open(sealed, cloudSyncPasswordPurpose)
	if err != nil {
		return "", false
	}
	s.password, s.known = string(password), true
	return s.password, true
}

func (s *cloudSyncSessionPassword) Clear() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.password, s.known = "", true
	_ = os.Remove(s.filePath)
	return true
}

// resetSyncProfilePrefixes define the cloud sync identity in the profile
// store: the master key verifier, OAuth client IDs, convergent replicas
// and baselines, per-provider snapshots/state and history. Forgetting the
// master key removes all of them so the next sync run starts from scratch.
var resetSyncProfilePrefixes = []string{
	"netcatty_master_key_config_v1",
	"netcatty_convergent_sync_replica_v2",
	"netcatty_convergent_sync_provider_baseline_v2_",
	"netcatty_sync_base_payload_v1",
	"netcatty_sync_snapshots_v1_",
	"netcatty_sync_history_v1",
	"netcatty_provider_",
	"netcatty_cloudsync_session_password",
	"netcatty_sync_oauth_client_ids_v1",
	"netcatty_sync_oauth_client_secrets_v1",
}

func isResetSyncProfileKey(key string) bool {
	for _, prefix := range resetSyncProfilePrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// CloudSyncResetService owns the "forgot master key, start over" flow.
type CloudSyncResetService struct {
	mu        sync.Mutex
	profile   *store.Store
	passwords *cloudSyncSessionPassword
}

func newCloudSyncResetService(profile *store.Store, passwords *cloudSyncSessionPassword) *CloudSyncResetService {
	return &CloudSyncResetService{profile: profile, passwords: passwords}
}

// ResetSyncEverything removes every cloud sync identity key from the profile
// store in one CAS transaction and clears the session password. Returns the
// removed keys for the confirmation dialog.
func (s *CloudSyncResetService) ResetSyncEverything(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var targets []string
	for _, domain := range []string{"settings", "vault"} {
		keys, err := s.profile.DomainKeys(domain)
		if err != nil {
			return nil, fmt.Errorf("list %s keys: %w", domain, err)
		}
		for _, key := range keys {
			if isResetSyncProfileKey(key) {
				targets = append(targets, key)
			}
		}
	}
	s.passwords.Clear()
	if len(targets) == 0 {
		return nil, nil
	}
	revision, err := s.profile.Revision()
	if err != nil {
		return nil, err
	}
	mutations := make([]store.Mutation, 0, len(targets))
	for _, key := range targets {
		mutations = append(mutations, store.Mutation{Domain: "settings", Key: key, Delete: true})
	}
	if _, err := s.profile.Write(store.WriteRequest{ExpectedRevision: revision, Mutations: mutations}); err != nil {
		return nil, fmt.Errorf("reset sync identity: %w", err)
	}
	return targets, nil
}

var errSyncResetUnavailable = errors.New("cloud sync reset service unavailable")
