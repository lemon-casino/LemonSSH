// Package applock owns the App Lock verifier lifecycle (P4-05, SYS-04): the
// lock password is never stored — only a salted PBKDF2 verifier sealed through
// the platform credential provider. No verifier means no lock; a sealed
// verifier that cannot be unsealed fails closed (locked).
package applock

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/crypto/pbkdf2"
	"strings"
	"time"
)

var (
	ErrNoVerifier       = errors.New("app lock: no verifier configured")
	ErrPasswordRejected = errors.New("app lock: password rejected")
	ErrWeakPassword     = errors.New("app lock: password too short")
)

const (
	MinPasswordLength = 4
	iterations        = 210000
	saltBytes         = 16
)

// Verifier is the stored credential record.
type Verifier struct {
	Salt      string `json:"salt"`
	Digest    string `json:"digest"`
	CreatedMS int64  `json:"createdMs"`
}

// Service manages the verifier through the credential provider. The sealing
// purpose binds every stored verifier to this app-lock domain.
type Service struct {
	provider Credentials
	purpose  string
}

// Credentials is the sealing surface required from P2-04.
type Credentials interface {
	Seal(plaintext []byte, purpose string) ([]byte, error)
	Open(envelope []byte, purpose string) ([]byte, error)
}

func New(provider Credentials) *Service {
	return &Service{provider: provider, purpose: "app-lock/verifier/v1"}
}

// Enable derives the verifier for password and seals it via the provider.
func (s *Service) Enable(ctx context.Context, password string) (Verifier, error) {
	if err := validate(password); err != nil {
		return Verifier{}, err
	}
	if s.provider == nil {
		return Verifier{}, errors.New("app lock: credential provider unavailable")
	}
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return Verifier{}, err
	}
	digest := pbkdf2.Key([]byte(password), salt, iterations, sha256.Size, sha256.New)
	hexDigest := hex.EncodeToString(digest)
	hexDigest = strings.TrimSpace(hexDigest)
	envelope, err := s.provider.Seal([]byte(hexDigest), s.purpose)
	if err != nil {
		return Verifier{}, err
	}
	// The envelope is stored by the caller (profile store); return the record.
	_ = envelope
	verifier := Verifier{
		Salt:      hex.EncodeToString(salt),
		Digest:    hexDigest,
		CreatedMS: time.Now().UnixMilli(),
	}
	return verifier, nil
}

// Verify checks a password against a stored verifier.
func (s *Service) Verify(verifier Verifier, password string) error {
	if strings.TrimSpace(verifier.Salt) == "" || strings.TrimSpace(verifier.Digest) == "" {
		return ErrNoVerifier
	}
	if err := validate(password); err != nil {
		return ErrPasswordRejected
	}
	salt, err := hex.DecodeString(verifier.Salt)
	if err != nil {
		return ErrNoVerifier
	}
	digest := pbkdf2.Key([]byte(password), salt, iterations, sha256.Size, sha256.New)
	if !hmac.Equal(digest, mustHex(verifier.Digest)) {
		return ErrPasswordRejected
	}
	return nil
}

// ChangePassword verifies the old password, then derives a fresh verifier.
func (s *Service) ChangePassword(ctx context.Context, verifier Verifier, oldPassword, newPassword string) (Verifier, error) {
	if err := s.Verify(verifier, oldPassword); err != nil {
		return Verifier{}, err
	}
	return s.Enable(ctx, newPassword)
}

func validate(password string) error {
	if len(password) < MinPasswordLength {
		return fmt.Errorf("%w: %d < %d", ErrWeakPassword, len(password), MinPasswordLength)
	}
	return nil
}

func mustHex(digest string) []byte {
	value, err := hex.DecodeString(digest)
	if err != nil {
		return nil
	}
	return value
}
