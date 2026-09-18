package profiledata

import (
	"fmt"
	"strings"
	"time"

	"github.com/binaricat/netcatty/internal/platform/credentials"
	"github.com/binaricat/netcatty/internal/profile/store"
)

// SecretStore persists sealed envelopes. The production implementation is
// the profile store's settings domain; the AI envelope bytes are ciphertext
// under the dedicated AI purpose, so profile storage never sees plaintext.
type SecretStore interface {
	Put(key string, value []byte) error
	Get(key string) ([]byte, error)
	Delete(key string) error
}

// ProfileSecretStore adapts the profile store to SecretStore.
type ProfileSecretStore struct {
	Store *store.Store
}

func (s ProfileSecretStore) Put(key string, value []byte) error {
	return s.Store.SetRaw("settings", key, value)
}

func (s ProfileSecretStore) Get(key string) ([]byte, error) {
	return s.Store.GetRaw("settings", key)
}

func (s ProfileSecretStore) Delete(key string) error {
	return s.Store.DeleteRaw("settings", key)
}

func secretStoreKey(reference string) string {
	return "ai/secrets/" + strings.TrimPrefix(reference, "secret_")
}

// SecretService is the dedicated AI secret API (design §7.3): Put returns
// only a reference, Replace rotates in place, Delete removes, Status
// reports existence. Resolve opens the secret for the HOST provider path
// inside a single request — it must never cross back to the renderer
// bindings (T38).
type SecretService struct {
	provider credentials.Provider
	store    SecretStore
	now      func() time.Time
}

func NewSecretService(provider credentials.Provider, secretStore SecretStore) *SecretService {
	return &SecretService{provider: provider, store: secretStore, now: time.Now}
}

// Put stores a new secret and returns its reference. The plaintext
// argument is the one-hop exception the design grants (the settings form
// submits it once); nothing returns it afterwards.
func (s *SecretService) Put(plaintext []byte) (string, error) {
	reference := newSecretRef()
	envelope, err := s.provider.Seal(plaintext, AIPurpose)
	if err != nil {
		return "", fmt.Errorf("profiledata: secret seal failed: %w", err)
	}
	if err := s.store.Put(secretStoreKey(reference), envelope); err != nil {
		return "", fmt.Errorf("profiledata: secret persist failed: %w", err)
	}
	return reference, nil
}

// Replace rotates the secret under an existing reference.
func (s *SecretService) Replace(reference string, plaintext []byte) error {
	envelope, err := s.provider.Seal(plaintext, AIPurpose)
	if err != nil {
		return fmt.Errorf("profiledata: secret seal failed: %w", err)
	}
	return s.store.Put(secretStoreKey(reference), envelope)
}

// Delete removes the secret under a reference.
func (s *SecretService) Delete(reference string) error {
	return s.store.Delete(secretStoreKey(reference))
}

// SecretStatus reports reference existence without any secret material.
type SecretStatus struct {
	Exists      bool  `json:"exists"`
	CreatedAtMS int64 `json:"createdAtMs,omitempty"`
}

// Status reports whether a reference resolves. The envelope stays opaque
// here: existence is the only claim without decrypting.
func (s *SecretService) Status(reference string) SecretStatus {
	envelope, err := s.store.Get(secretStoreKey(reference))
	if err != nil || len(envelope) == 0 {
		return SecretStatus{Exists: false}
	}
	return SecretStatus{Exists: true}
}

// Resolve opens the secret for the host provider path. Callers are Go-side
// only; the bindings layer must never expose this method (T38).
func (s *SecretService) Resolve(reference string) ([]byte, error) {
	envelope, err := s.store.Get(secretStoreKey(reference))
	if err != nil {
		return nil, fmt.Errorf("profiledata: secret load failed: %w", err)
	}
	if len(envelope) == 0 {
		return nil, fmt.Errorf("profiledata: secret %q not found", reference)
	}
	plaintext, err := s.provider.Open(envelope, AIPurpose)
	if err != nil {
		return nil, fmt.Errorf("profiledata: secret open failed: %w", err)
	}
	return plaintext, nil
}

// serviceSink adapts SecretService to the migration SecretSink: the
// service generates its own reference and the adapter reports it back so
// the migrated config and the receipt agree on it.
type serviceSink struct {
	service *SecretService
}

func (s serviceSink) Store(_ string, plaintext []byte) (string, error) {
	return s.service.Put(plaintext)
}
