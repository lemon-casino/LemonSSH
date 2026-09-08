package main

import (
	"os"
	"path/filepath"

	"github.com/binaricat/netcatty/internal/profile/store"
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

// openProfileStore opens the host-owned profile store. The directory can be
// overridden with NETCATTY_PROFILE_DIR for tests and portable layouts.
func openProfileStore() (*store.Store, error) {
	dir := os.Getenv("NETCATTY_PROFILE_DIR")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(base, "netcatty")
	}
	return store.Open(filepath.Join(dir, "profile.db"), nil)
}
