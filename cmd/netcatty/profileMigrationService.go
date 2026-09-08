package main

import (
	"crypto/ecdh"
	"encoding/base64"
	"path/filepath"

	"github.com/binaricat/netcatty/internal/platform/credentials"
	"github.com/binaricat/netcatty/internal/profile/migration"
	"github.com/binaricat/netcatty/internal/profile/store"
)

// ProfileMigrationService is the Wails-facing import/verify/promote facade
// (P2-06). It never promotes before migration.Import returns fully verified
// mutations and StageProfile marks the complete staging store.
type ProfileMigrationService struct {
	provider   credentials.Provider
	profileDir string
}

func newProfileMigrationService(provider credentials.Provider, profileDir string) *ProfileMigrationService {
	return &ProfileMigrationService{provider: provider, profileDir: profileDir}
}

// ImportAndPromote verifies the encrypted bundle, re-seals secrets using the
// platform provider, stages all mutations, and atomically promotes the result.
func (s *ProfileMigrationService) ImportAndPromote(bundleBase64, targetPrivateKeyBase64, fingerprint string) (*store.MigrationReceipt, error) {
	bundle, err := base64.StdEncoding.Strict().DecodeString(bundleBase64)
	if err != nil {
		return nil, migration.ErrBundleInvalid
	}
	privateBytes, err := base64.StdEncoding.Strict().DecodeString(targetPrivateKeyBase64)
	if err != nil {
		return nil, migration.ErrBundleInvalid
	}
	privateKey, err := ecdh.X25519().NewPrivateKey(privateBytes)
	if err != nil {
		return nil, migration.ErrBundleInvalid
	}
	mutations, err := migration.Import(bundle, privateKey, fingerprint, s.provider)
	if err != nil {
		return nil, err
	}
	stagingDir := filepath.Join(s.profileDir, "staging")
	stagingPath, err := store.StageProfile(stagingDir, mutations)
	if err != nil {
		return nil, err
	}
	targetPath := filepath.Join(s.profileDir, "profile.db")
	backupDir := filepath.Join(s.profileDir, "backups")
	return store.PromoteProfile(stagingPath, targetPath, backupDir, fingerprint)
}

// VerifyBundle checks the encrypted bundle without writing or promoting.
func (s *ProfileMigrationService) VerifyBundle(bundleBase64, targetPrivateKeyBase64, fingerprint string) (int, error) {
	bundle, err := base64.StdEncoding.Strict().DecodeString(bundleBase64)
	if err != nil {
		return 0, migration.ErrBundleInvalid
	}
	privateBytes, err := base64.StdEncoding.Strict().DecodeString(targetPrivateKeyBase64)
	if err != nil {
		return 0, migration.ErrBundleInvalid
	}
	privateKey, err := ecdh.X25519().NewPrivateKey(privateBytes)
	if err != nil {
		return 0, migration.ErrBundleInvalid
	}
	mutations, err := migration.Import(bundle, privateKey, fingerprint, s.provider)
	if err != nil {
		return 0, err
	}
	return len(mutations), nil
}
