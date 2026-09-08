// Package migration imports the encrypted Electron profile bundle (P2-06).
// It verifies the bundle before touching the target store, re-seals secret
// records through the platform credential provider, and only then promotes a
// complete staging profile.
package migration

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/binaricat/netcatty/internal/platform/credentials"
	"github.com/binaricat/netcatty/internal/profile/store"
)

const (
	BundleVersion = 1
	BundlePurpose = "netcatty/profile-migration-bundle/v1"
)

var (
	ErrBundleInvalid     = errors.New("invalid migration bundle")
	ErrBundleFingerprint = errors.New("migration source fingerprint mismatch")
	ErrBundleManifest    = errors.New("migration manifest mismatch")
)

type Bundle struct {
	BundleVersion     int              `json:"bundleVersion"`
	SourceFingerprint string           `json:"sourceFingerprint"`
	BackupManifest    json.RawMessage  `json:"backupManifest"`
	RecordCount       int              `json:"recordCount"`
	SecretCount       int              `json:"secretCount"`
	Encrypted         EncryptedPayload `json:"encrypted"`
}

type EncryptedPayload struct {
	Version            int    `json:"version"`
	Algorithm          string `json:"algorithm"`
	Purpose            string `json:"purpose"`
	EphemeralPublicKey string `json:"ephemeralPublicKey"`
	Nonce              string `json:"nonce"`
	Ciphertext         string `json:"ciphertext"`
}

type payload struct {
	Version           int             `json:"version"`
	SourceFingerprint string          `json:"sourceFingerprint"`
	Records           []Record        `json:"records"`
	Manifest          []ManifestEntry `json:"manifest"`
}

type Record struct {
	Domain         string `json:"domain"`
	Key            string `json:"key"`
	Classification string `json:"classification"`
	ValueBase64    string `json:"valueBase64,omitempty"`
	Plaintext      string `json:"plaintext,omitempty"`
}

type ManifestEntry struct {
	Domain         string `json:"domain"`
	Key            string `json:"key"`
	Classification string `json:"classification"`
	Secret         bool   `json:"secret"`
}

// Import decrypts and verifies a bundle. It returns store mutations; callers
// must invoke StageProfile/PromoteProfile only after all records verify.
func Import(bundleBytes []byte, targetPrivateKey *ecdh.PrivateKey, expectedFingerprint string, provider credentials.Provider) ([]store.Mutation, error) {
	var bundle Bundle
	decoder := json.NewDecoder(strings.NewReader(string(bundleBytes)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bundle); err != nil || bundle.BundleVersion != BundleVersion || bundle.Encrypted.Algorithm != "X25519-HKDF-SHA256-AES-256-GCM" {
		return nil, ErrBundleInvalid
	}
	if bundle.SourceFingerprint != expectedFingerprint {
		return nil, ErrBundleFingerprint
	}
	if targetPrivateKey == nil {
		return nil, ErrBundleInvalid
	}
	plain, err := decrypt(bundle.Encrypted, targetPrivateKey)
	if err != nil {
		return nil, err
	}
	defer zero(plain)
	var value payload
	decoder = json.NewDecoder(strings.NewReader(string(plain)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil || value.Version != BundleVersion || value.SourceFingerprint != expectedFingerprint || len(value.Records) != bundle.RecordCount || len(value.Manifest) != bundle.RecordCount {
		return nil, ErrBundleManifest
	}
	if err := verifyManifest(value.Records, value.Manifest, bundle.SecretCount); err != nil {
		return nil, err
	}
	mutations := make([]store.Mutation, 0, len(value.Records))
	for _, record := range value.Records {
		secret := record.Classification == "secret" || record.Classification == "re-seal-secrets"
		if secret {
			if provider == nil || !provider.Available() {
				return nil, credentials.ErrUnavailable
			}
			envelope, err := provider.Seal([]byte(record.Plaintext), "profile-migration/"+record.Domain+"/"+record.Key)
			zero([]byte(record.Plaintext))
			if err != nil {
				return nil, err
			}
			mutations = append(mutations, store.Mutation{Domain: record.Domain, Key: record.Key, Value: envelope})
			continue
		}
		valueBytes, err := base64.StdEncoding.Strict().DecodeString(record.ValueBase64)
		if err != nil {
			return nil, ErrBundleInvalid
		}
		mutations = append(mutations, store.Mutation{Domain: record.Domain, Key: record.Key, Value: valueBytes})
	}
	return mutations, nil
}

func decrypt(value EncryptedPayload, privateKey *ecdh.PrivateKey) ([]byte, error) {
	ephemeralBytes, err := base64.StdEncoding.Strict().DecodeString(value.EphemeralPublicKey)
	if err != nil {
		return nil, ErrBundleInvalid
	}
	ephemeral, err := x509.ParsePKIXPublicKey(ephemeralBytes)
	if err != nil {
		return nil, ErrBundleInvalid
	}
	ecdhPublic, ok := ephemeral.(*ecdh.PublicKey)
	if !ok {
		return nil, ErrBundleInvalid
	}
	shared, err := privateKey.ECDH(ecdhPublic)
	if err != nil {
		return nil, ErrBundleInvalid
	}
	defer zero(shared)
	key, err := hkdfKey(shared, value.Purpose)
	if err != nil {
		return nil, err
	}
	defer zero(key)
	nonce, err := base64.StdEncoding.Strict().DecodeString(value.Nonce)
	if err != nil {
		return nil, ErrBundleInvalid
	}
	ciphertext, err := base64.StdEncoding.Strict().DecodeString(value.Ciphertext)
	if err != nil {
		return nil, ErrBundleInvalid
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrBundleInvalid
	}
	aead, err := cipher.NewGCM(block)
	if err != nil || len(nonce) != aead.NonceSize() {
		return nil, ErrBundleInvalid
	}
	plain, err := aead.Open(nil, nonce, ciphertext, []byte(BundlePurpose+"/"+value.Purpose))
	if err != nil {
		return nil, ErrBundleInvalid
	}
	return plain, nil
}

func hkdfKey(shared []byte, purpose string) ([]byte, error) {
	if purpose == "" || !strings.HasPrefix(purpose, "profile-cutover") {
		return nil, ErrBundleInvalid
	}
	// HKDF-SHA256 extract/expand using stdlib HMAC.
	return hkdfSHA256(shared, []byte(purpose), []byte(BundlePurpose+"/"+purpose), 32), nil
}

func verifyManifest(records []Record, manifest []ManifestEntry, secretCount int) error {
	if len(records) != len(manifest) {
		return ErrBundleManifest
	}
	secrets := 0
	for i, record := range records {
		entry := manifest[i]
		secret := record.Classification == "secret" || record.Classification == "re-seal-secrets"
		if entry.Domain != record.Domain || entry.Key != record.Key || entry.Classification != record.Classification || entry.Secret != secret {
			return ErrBundleManifest
		}
		if secret {
			secrets++
		}
	}
	if secrets != secretCount {
		return ErrBundleManifest
	}
	return nil
}

func zero(value []byte) {
	for i := range value {
		value[i] = 0
	}
}
