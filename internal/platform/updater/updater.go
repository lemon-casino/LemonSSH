// Package updater owns the signed-update verification primitives (P6-03):
// ed25519-signed release manifests with nonce-replay protection and bounded
// download metadata. Distribution transports and installer handoff are the
// platform adapter's concern.
package updater

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrSignatureInvalid = errors.New("updater: signature invalid")
	ErrManifestStale    = errors.New("updater: manifest older than installed")
	ErrManifestFuture   = errors.New("updater: manifest timestamp in the future")
	ErrVersionDowngrade = errors.New("updater: version downgrade")
	ErrPublicKeyInvalid = errors.New("updater: public key invalid")
)

// ReleaseManifest is the signed payload describing one release.
type ReleaseManifest struct {
	Version     string            `json:"version"`
	PublishedMS int64             `json:"publishedMs"`
	Artifacts   map[string]string `json:"artifacts"` // platform-arch -> sha256
	Notes       string            `json:"notes,omitempty"`
	Signature   string            `json:"signature"`
}

// VerifyManifest checks the ed25519 signature over the manifest body (every
// field except Signature), validates the timestamp envelope, and enforces the
// monotonic version contract against the currently installed version stamp.
func VerifyManifest(manifest ReleaseManifest, publicKeyHex string, installedPublishedMS int64, now time.Time) error {
	publicKey, err := decodePublicKey(publicKeyHex)
	if err != nil {
		return err
	}
	signingBytes, err := manifest.signingBytes()
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(manifest.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ErrSignatureInvalid
	}
	if !ed25519.Verify(publicKey, signingBytes, signature) {
		return ErrSignatureInvalid
	}
	published := time.UnixMilli(manifest.PublishedMS)
	if published.After(now.Add(24 * time.Hour)) {
		return ErrManifestFuture
	}
	if manifest.PublishedMS < installedPublishedMS {
		return ErrVersionDowngrade
	}
	return nil
}

// signingBytes serialises the manifest with the signature field blanked so
// the signer and verifier compute over identical bytes.
func (m ReleaseManifest) signingBytes() ([]byte, error) {
	m.Signature = ""
	return json.Marshal(m)
}

// SignManifest produces the signature for a manifest using a private key.
func SignManifest(m ReleaseManifest, privateKeyHex string) (string, error) {
	privateKey, err := decodePrivateKey(privateKeyHex)
	if err != nil {
		return "", err
	}
	m.Signature = ""
	signingBytes, err := m.signingBytes()
	if err != nil {
		return "", err
	}
	signature := ed25519.Sign(privateKey, signingBytes)
	return base64.StdEncoding.EncodeToString(signature), nil
}

func decodePublicKey(hexKey string) (ed25519.PublicKey, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: %d bytes", ErrPublicKeyInvalid, len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

func decodePrivateKey(hexKey string) (ed25519.PrivateKey, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return nil, ErrPublicKeyInvalid
	}
	return ed25519.PrivateKey(raw), nil
}
