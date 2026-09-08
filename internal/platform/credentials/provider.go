// Package credentials owns the platform credential provider for the Go
// profile runtime (P2-04). It stores one random AES-256 key per purpose in the
// OS keyring and encrypts each value with a fresh GCM nonce. The keyring
// backend is injectable for tests; the real backend is go-keyring, which maps
// to Windows Credential Manager, macOS Keychain, and Linux Secret Service.
package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

const (
	EnvelopeVersion = 1
	ProviderVersion = 1
	ProviderName    = "os-keyring-aes-gcm"
	MaxPlaintext    = 128 << 10
	MaxEnvelope     = 256 << 10
	keyringService  = "Netcatty"
)

var (
	ErrUnavailable       = errors.New("credential provider unavailable")
	ErrInvalidPurpose    = errors.New("invalid credential purpose")
	ErrPurposeMismatch   = errors.New("credential purpose mismatch")
	ErrMalformedEnvelope = errors.New("malformed credential envelope")
	ErrPlaintextTooLarge = errors.New("credential plaintext exceeds bound")
)

// Keyring is the minimum OS secure-storage surface required by the provider.
type Keyring interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

// Provider is the production credential contract used by profile migration.
type Provider interface {
	Name() string
	Available() bool
	Seal(plaintext []byte, purpose string) ([]byte, error)
	Open(envelope []byte, purpose string) ([]byte, error)
}

type envelope struct {
	Version    int    `json:"version"`
	Provider   string `json:"provider"`
	Purpose    string `json:"purpose"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// ProviderImpl is a keyring-backed provider. Do not copy after first use.
type ProviderImpl struct {
	keyring Keyring
	mu      sync.Mutex
}

// New constructs a provider using the supplied keyring. A nil backend is
// unavailable and all seal/open operations fail closed.
func New(keyring Keyring) *ProviderImpl { return &ProviderImpl{keyring: keyring} }

func (p *ProviderImpl) Name() string { return ProviderName }

func (p *ProviderImpl) Available() bool {
	if p == nil || p.keyring == nil {
		return false
	}
	// A keyring is considered available only if a harmless probe can be
	// completed. The probe key is never used for secrets and is removed again.
	const probeUser = "availability-probe"
	if err := p.keyring.Set(keyringService, probeUser, "ok"); err != nil {
		return false
	}
	_ = p.keyring.Delete(keyringService, probeUser)
	return true
}

func (p *ProviderImpl) Seal(plaintext []byte, purpose string) ([]byte, error) {
	if p == nil || p.keyring == nil || !p.Available() {
		return nil, ErrUnavailable
	}
	if err := validatePurpose(purpose); err != nil {
		return nil, err
	}
	if len(plaintext) == 0 || len(plaintext) > MaxPlaintext {
		return nil, ErrPlaintextTooLarge
	}
	key, err := p.keyForPurpose(purpose, true)
	if err != nil {
		return nil, err
	}
	defer zero(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrUnavailable
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrUnavailable
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, ErrUnavailable
	}
	aad := []byte(purpose)
	ciphertext := aead.Seal(nil, nonce, plaintext, aad)
	zero(aad)
	result := envelope{
		Version:    EnvelopeVersion,
		Provider:   ProviderName,
		Purpose:    purpose,
		Nonce:      base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawStdEncoding.EncodeToString(ciphertext),
	}
	zero(nonce)
	zero(ciphertext)
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > MaxEnvelope {
		zero(encoded)
		return nil, ErrMalformedEnvelope
	}
	return encoded, nil
}

func (p *ProviderImpl) Open(encoded []byte, purpose string) ([]byte, error) {
	if p == nil || p.keyring == nil || !p.Available() {
		return nil, ErrUnavailable
	}
	if err := validatePurpose(purpose); err != nil {
		return nil, err
	}
	if len(encoded) == 0 || len(encoded) > MaxEnvelope {
		return nil, ErrMalformedEnvelope
	}
	var value envelope
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return nil, ErrMalformedEnvelope
	}
	if value.Version != EnvelopeVersion || value.Provider != ProviderName || value.Purpose != purpose {
		return nil, ErrPurposeMismatch
	}
	nonce, err := base64.RawStdEncoding.Strict().DecodeString(value.Nonce)
	if err != nil {
		return nil, ErrMalformedEnvelope
	}
	defer zero(nonce)
	ciphertext, err := base64.RawStdEncoding.Strict().DecodeString(value.Ciphertext)
	if err != nil {
		return nil, ErrMalformedEnvelope
	}
	defer zero(ciphertext)
	key, err := p.keyForPurpose(purpose, false)
	if err != nil {
		return nil, err
	}
	defer zero(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrUnavailable
	}
	aead, err := cipher.NewGCM(block)
	if err != nil || len(nonce) != aead.NonceSize() {
		return nil, ErrMalformedEnvelope
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, []byte(purpose))
	if err != nil {
		return nil, ErrPurposeMismatch
	}
	if len(plaintext) == 0 || len(plaintext) > MaxPlaintext {
		zero(plaintext)
		return nil, ErrPlaintextTooLarge
	}
	return plaintext, nil
}

func (p *ProviderImpl) keyForPurpose(purpose string, create bool) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	digest := sha256.Sum256([]byte(purpose))
	user := fmt.Sprintf("credential-purpose-%x", digest[:])
	encoded, err := p.keyring.Get(keyringService, user)
	if err == nil {
		key, decodeErr := base64.RawStdEncoding.Strict().DecodeString(encoded)
		if decodeErr != nil || len(key) != 32 {
			zero(key)
			return nil, ErrUnavailable
		}
		return key, nil
	}
	if !create {
		return nil, ErrUnavailable
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		zero(key)
		return nil, ErrUnavailable
	}
	if err := p.keyring.Set(keyringService, user, base64.RawStdEncoding.EncodeToString(key)); err != nil {
		zero(key)
		return nil, ErrUnavailable
	}
	return key, nil
}

func validatePurpose(purpose string) error {
	if purpose == "" || len(purpose) > 256 || strings.TrimSpace(purpose) != purpose {
		return ErrInvalidPurpose
	}
	for _, r := range purpose {
		if r < 0x21 || r > 0x7e {
			return ErrInvalidPurpose
		}
	}
	return nil
}

func zero(value []byte) {
	for i := range value {
		value[i] = 0
	}
}
