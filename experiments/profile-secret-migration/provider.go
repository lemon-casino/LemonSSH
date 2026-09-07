package migrationprobe

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

const (
	EnvelopeVersion = 1
	ProviderVersion = 1
	ProviderName    = "windows-dpapi-user"
	maxEnvelopeSize = 256 * 1024
)

var ErrProviderUnavailable = errors.New("credential provider unavailable")

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
	Ciphertext string `json:"ciphertext"`
}

func validatePurpose(purpose string) error {
	if len(purpose) < 1 || len(purpose) > 256 || strings.TrimSpace(purpose) != purpose {
		return errors.New("invalid purpose")
	}
	for _, r := range purpose {
		if r < 0x21 || r > 0x7e {
			return errors.New("invalid purpose")
		}
	}
	return nil
}

func encodeEnvelope(purpose string, ciphertext []byte) ([]byte, error) {
	if err := validatePurpose(purpose); err != nil {
		return nil, err
	}
	if len(ciphertext) == 0 || len(ciphertext) > maxEnvelopeSize {
		return nil, errors.New("invalid ciphertext size")
	}
	encoded, err := json.Marshal(envelope{
		Version:    EnvelopeVersion,
		Provider:   ProviderName,
		Purpose:    purpose,
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	})
	if err != nil {
		return nil, err
	}
	if len(encoded) > maxEnvelopeSize {
		zero(encoded)
		return nil, errors.New("invalid envelope size")
	}
	return encoded, nil
}

func decodeEnvelope(data []byte, purpose string) ([]byte, error) {
	if len(data) == 0 || len(data) > maxEnvelopeSize {
		return nil, errors.New("invalid envelope size")
	}
	if err := validatePurpose(purpose); err != nil {
		return nil, err
	}
	var value envelope
	if err := decodeStrictJSON(data, &value); err != nil {
		return nil, errors.New("invalid envelope")
	}
	if value.Version != EnvelopeVersion || value.Provider != ProviderName || value.Purpose != purpose {
		return nil, errors.New("envelope metadata mismatch")
	}
	if value.Ciphertext == "" || len(value.Ciphertext) > maxEnvelopeSize {
		return nil, errors.New("invalid ciphertext")
	}
	ciphertext, err := base64.StdEncoding.Strict().DecodeString(value.Ciphertext)
	if err != nil || len(ciphertext) == 0 || len(ciphertext) > maxEnvelopeSize {
		return nil, errors.New("invalid ciphertext")
	}
	return ciphertext, nil
}
