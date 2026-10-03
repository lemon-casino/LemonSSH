package main

import (
	"encoding/json"
	"log"

	"github.com/binaricat/lemonssh/internal/app/updateuse"
	"github.com/binaricat/lemonssh/internal/profile/store"
)

// UpdateService is the Wails-facing in-app update facade. It owns no update
// logic: state, downloads, verification and install live in
// internal/app/updateuse. Event names and payload shapes mirror the
// electron-updater bridge contract consumed by useUpdateCheck
// (types/global/lemonssh-bridge-app.d.ts).
type UpdateService struct {
	core  *updateuse.Service
	store *store.Store
}

const (
	updateSettingsDomain   = "settings"
	updateEnabledKey       = "auto-update-enabled"
	updateLastCheckKey     = "update-last-auto-check"
	updateDefaultAutoState = true
)

// newUpdateService wires the shell-neutral update core with the profile
// store (auto-update toggle + check throttle) and the running version.
func newUpdateService(currentVersion, storageDir, publicKeyHex string, profileStore *store.Store) *UpdateService {
	service := &UpdateService{
		store: profileStore,
	}
	opts := updateuse.Options{
		CurrentVersion: currentVersion,
		StorageDir:     storageDir,
		PublicKeyHex:   publicKeyHex,
		AutoUpdate:     service.persistedAutoUpdate(),
		PersistAutoUpdate: func(enabled bool) error {
			return service.persistBool(updateEnabledKey, enabled)
		},
		PersistedLastCheck: service.persistedLastCheck,
		PersistLastCheck: func(at int64) error {
			raw, err := json.Marshal(at)
			if err != nil {
				return err
			}
			return service.storeSet(updateLastCheckKey, raw)
		},
	}
	service.core = updateuse.New(opts)
	return service
}

func (s *UpdateService) setEventEmitter(emit func(name string, payload any)) {
	s.core.SetEmitter(emit)
}

func (s *UpdateService) setQuit(quit func()) {
	s.core.SetQuit(quit)
}

// CheckForUpdate queries GitHub Releases for a newer version. When
// auto-update is on, the download starts in the background and progress
// arrives through the update:* events.
func (s *UpdateService) CheckForUpdate() updateuse.CheckResult {
	return s.core.Check()
}

// DownloadUpdate downloads the pending artifact; progress streams through
// update:download-progress. Resolves when the download completes.
func (s *UpdateService) DownloadUpdate() updateuse.DownloadResult {
	return s.core.Download()
}

// InstallUpdate swaps the running executable with the downloaded artifact,
// schedules a relaunch and quits the app.
func (s *UpdateService) InstallUpdate() error {
	return s.core.Install()
}

// GetUpdateStatus hydrates renderer state (late-opening windows, retries).
func (s *UpdateService) GetUpdateStatus() updateuse.StatusSnapshot {
	return s.core.StatusSnapshot()
}

// GetAutoUpdate reports the persisted auto-download toggle.
func (s *UpdateService) GetAutoUpdate() map[string]any {
	return map[string]any{"enabled": s.core.AutoUpdate()}
}

// SetAutoUpdate persists the auto-download toggle.
func (s *UpdateService) SetAutoUpdate(enabled bool) map[string]any {
	s.core.SetAutoUpdate(enabled)
	return map[string]any{"success": true}
}

// AutoCheck runs the throttled startup check. The shell schedules it after
// the first window's runtime is ready so update events reach renderers.
func (s *UpdateService) AutoCheck() {
	s.core.AutoCheck()
}

func (s *UpdateService) persistedAutoUpdate() bool {
	if s.store == nil {
		return updateDefaultAutoState
	}
	raw, err := s.store.GetRaw(updateSettingsDomain, updateEnabledKey)
	if err != nil || len(raw) == 0 {
		return updateDefaultAutoState
	}
	var enabled bool
	if json.Unmarshal(raw, &enabled) != nil {
		return updateDefaultAutoState
	}
	return enabled
}

func (s *UpdateService) persistedLastCheck() int64 {
	if s.store == nil {
		return 0
	}
	raw, err := s.store.GetRaw(updateSettingsDomain, updateLastCheckKey)
	if err != nil || len(raw) == 0 {
		return 0
	}
	var at int64
	if json.Unmarshal(raw, &at) != nil {
		return 0
	}
	return at
}

func (s *UpdateService) persistBool(key string, value bool) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.storeSet(key, raw)
}

func (s *UpdateService) storeSet(key string, raw []byte) error {
	if s.store == nil {
		return nil
	}
	if err := s.store.SetRaw(updateSettingsDomain, key, raw); err != nil {
		log.Printf("update: persist %s failed: %v", key, err)
		return err
	}
	return nil
}
