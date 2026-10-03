package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/binaricat/lemonssh/internal/profile/store"
)

// ProfileService is the Wails-facing facade over the transactional profile
// store (P2-02). It holds no policy: validation lives in the store.
type ProfileService struct {
	store *store.Store
}

func newProfileService(profileStore *store.Store) *ProfileService {
	return &ProfileService{store: profileStore}
}

// Revision reports the current committed profile revision.
func (s *ProfileService) Revision() (uint64, error) {
	return s.store.Revision()
}

// GetRaw returns one opaque raw value (base64 on the wire).
func (s *ProfileService) GetRaw(domain, key string) ([]byte, error) {
	value, err := s.store.GetRaw(domain, key)
	if err != nil {
		return nil, err
	}
	return value, nil
}

// SetRaw writes one opaque raw value atomically.
func (s *ProfileService) SetRaw(domain, key string, value []byte) error {
	return s.store.SetRaw(domain, key, value)
}

// DeleteRaw removes one raw value.
func (s *ProfileService) DeleteRaw(domain, key string) error {
	return s.store.DeleteRaw(domain, key)
}

// Write applies an atomic multi-key transaction with optional CAS.
func (s *ProfileService) Write(expectedRevision uint64, mutations []store.Mutation) (store.WriteResult, error) {
	return s.store.Write(store.WriteRequest{
		ExpectedRevision: expectedRevision,
		Mutations:        mutations,
	})
}

// Domains lists the declared profile domains.
func (s *ProfileService) Domains() []string {
	return store.Domains
}

// DomainKeys lists keys in one profile domain, used to hydrate localStorage.
func (s *ProfileService) DomainKeys(domain string) ([]string, error) {
	return s.store.DomainKeys(domain)
}

const (
	// profileDirEnv overrides the profile directory. profileDirLegacyEnv is
	// still honored as a read-only fallback so launchers and scripts written
	// for the previous release keep working.
	profileDirEnv       = "LEMONSSH_PROFILE_DIR"
	profileDirLegacyEnv = "NETCATTY_PROFILE_DIR"

	profileDirName       = "lemonssh"
	legacyProfileDirName = "netcatty"

	// profileMigrationMarker completes the one-time copy migration from the
	// legacy directory. A missing marker with an existing target directory
	// means "resume": existing target entries are never overwritten.
	profileMigrationMarker = ".lemonssh-profile-migrated"
)

// osUserConfigDir is indirected so tests can redirect the default base.
var osUserConfigDir = os.UserConfigDir

// profileDirCandidates resolves the target profile directory and the legacy
// directory to migrate from. An explicit LEMONSSH_PROFILE_DIR is a deliberate
// portable/testing override: it wins outright and disables migration. The
// legacy NETCATTY_PROFILE_DIR only redirects the migration source while the
// target stays the default per-user directory.
func profileDirCandidates() (target, legacy string) {
	if dir := os.Getenv(profileDirEnv); dir != "" {
		return dir, ""
	}
	base, err := osUserConfigDir()
	if err != nil {
		return ".", ""
	}
	if dir := os.Getenv(profileDirLegacyEnv); dir != "" {
		return filepath.Join(base, profileDirName), dir
	}
	return filepath.Join(base, profileDirName), filepath.Join(base, legacyProfileDirName)
}

// migrationWarnOnce keeps the migration-failure notice to one stderr line
// even though several startup services consult baseProfileDir.
var migrationWarnOnce sync.Once

// baseProfileDir reports the profile data directory, running the one-time
// copy migration from the legacy "netcatty" directory first. On migration
// failure the legacy directory keeps serving this session (no empty-looking
// first launch); the migration retries on the next start. The legacy
// directory is never deleted or modified.
func baseProfileDir() string {
	target, legacy := profileDirCandidates()
	resolved, err := ensureProfileDirMigrated(target, legacy)
	if err != nil {
		migrationWarnOnce.Do(func() {
			fmt.Fprintf(os.Stderr, "[LemonSSH] profile directory migration deferred (will retry next start): %v\n", err)
		})
		return legacy
	}
	return resolved
}

// ensureProfileDirMigrated performs the one-time legacy→target copy:
//   - no legacy directory: fresh machine, just create the target;
//   - target missing or missing the marker: copy legacy content, skipping
//     entries that already exist in the target (never overwrite newer data),
//     then write the marker last so a crash mid-copy resumes on next start.
//
// The marker makes the migration idempotent and resumable.
func ensureProfileDirMigrated(target, legacy string) (string, error) {
	if legacy == "" || target == legacy {
		if err := os.MkdirAll(target, 0o755); err != nil {
			return "", err
		}
		return target, nil
	}
	if _, err := os.Stat(legacy); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Fresh machine: nothing to migrate.
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
			return target, nil
		}
		return "", err
	}
	if _, err := os.Stat(filepath.Join(target, profileMigrationMarker)); err == nil {
		return target, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return "", err
	}
	if err := copyProfileTree(legacy, target); err != nil {
		return "", err
	}
	payload := fmt.Sprintf(`{"v":1,"at":%q}`, time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(target, profileMigrationMarker), []byte(payload), 0o644); err != nil {
		return "", err
	}
	return target, nil
}

// copyProfileTree is a package variable so tests can inject copy failures.
var copyProfileTree = copyTreeSkipExisting

// copyTreeSkipExisting copies src into dst recursively, creating directories
// as needed and skipping entries that already exist in dst (target entries
// are never overwritten). Symlinks and other irregular entries are skipped;
// the profile tree contains only regular files and directories.
func copyTreeSkipExisting(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() && !entry.IsDir() {
			continue
		}
		source := filepath.Join(src, entry.Name())
		destination := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := os.MkdirAll(destination, 0o755); err != nil {
				return err
			}
			if err := copyTreeSkipExisting(source, destination); err != nil {
				return err
			}
			continue
		}
		if _, err := os.Lstat(destination); err == nil {
			continue // existing target entries are never overwritten
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := copyFileContents(source, destination, entry); err != nil {
			return err
		}
	}
	return nil
}

func copyFileContents(src, dst string, entry os.DirEntry) error {
	info, err := entry.Info()
	if err != nil {
		return err
	}
	// Paths derive from the resolved profile directories, not user input.
	source, err := os.Open(src) // #nosec G304 -- profile dir tree copy
	if err != nil {
		return err
	}
	defer source.Close()
	destination, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(destination, source); err != nil {
		_ = destination.Close()
		return err
	}
	return destination.Close()
}

// openProfileStore opens the host-owned profile store. The directory can be
// overridden with LEMONSSH_PROFILE_DIR (legacy NETCATTY_PROFILE_DIR is still
// honored as fallback) for tests and portable layouts.
func openProfileStore() (*store.Store, error) {
	return store.Open(filepath.Join(baseProfileDir(), "profile.db"), nil)
}
