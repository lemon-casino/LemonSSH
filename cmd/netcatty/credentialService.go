package main

import (
	"github.com/binaricat/netcatty/internal/platform/credentials"
)

// CredentialService is the Wails-facing facade over the platform keyring
// provider. It never returns or persists provider keys; the provider owns
// those inside the OS keyring.
type CredentialService struct {
	provider credentials.Provider
}

func newCredentialService(provider credentials.Provider) *CredentialService {
	return &CredentialService{provider: provider}
}

// Available reports whether secure platform storage is available.
func (s *CredentialService) Available() bool {
	return s.provider != nil && s.provider.Available()
}

// ProviderName reports the stable provider identity.
func (s *CredentialService) ProviderName() string {
	if s.provider == nil {
		return ""
	}
	return s.provider.Name()
}

// Seal protects one plaintext value for a purpose. Plaintext is accepted only
// for the duration of the call and is never returned.
func (s *CredentialService) Seal(plaintext []byte, purpose string) ([]byte, error) {
	if s.provider == nil {
		return nil, credentials.ErrUnavailable
	}
	return s.provider.Seal(plaintext, purpose)
}

// Open unseals one purpose-bound envelope.
func (s *CredentialService) Open(envelope []byte, purpose string) ([]byte, error) {
	if s.provider == nil {
		return nil, credentials.ErrUnavailable
	}
	return s.provider.Open(envelope, purpose)
}
