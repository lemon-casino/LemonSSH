package migration

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/binaricat/netcatty/internal/platform/credentials"
	"github.com/binaricat/netcatty/internal/profile/store"
)

type fakeProvider struct{ available bool }

func (p fakeProvider) Name() string    { return "fake" }
func (p fakeProvider) Available() bool { return p.available }
func (p fakeProvider) Seal(value []byte, purpose string) ([]byte, error) {
	return append([]byte("sealed:"+purpose+":"), value...), nil
}
func (p fakeProvider) Open(value []byte, purpose string) ([]byte, error) {
	return nil, credentials.ErrUnavailable
}

func makeTestBundle(t *testing.T, key *ecdh.PrivateKey, records []Record, fingerprint string) []byte {
	t.Helper()
	payload := payload{Version: BundleVersion, SourceFingerprint: fingerprint, Records: records}
	for _, record := range records {
		payload.Manifest = append(payload.Manifest, ManifestEntry{Domain: record.Domain, Key: record.Key, Classification: record.Classification, Secret: record.Classification == "secret" || record.Classification == "re-seal-secrets"})
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	ephemeral, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := ephemeral.ECDH(key.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	bundleKey := hkdfSHA256(shared, []byte("profile-cutover"), []byte(BundlePurpose+"/profile-cutover"), 32)
	block, _ := aes.NewCipher(bundleKey)
	aead, _ := cipher.NewGCM(block)
	nonce := make([]byte, aead.NonceSize())
	_, _ = rand.Read(nonce)
	aad := []byte("netcatty/profile-migration-bundle/v1/profile-cutover")
	ciphertext := aead.Seal(nil, nonce, plaintext, aad)
	ephemeralSPKI, _ := x509.MarshalPKIXPublicKey(ephemeral.PublicKey())
	secretCount := 0
	for _, record := range records {
		if record.Classification == "secret" || record.Classification == "re-seal-secrets" {
			secretCount++
		}
	}
	bundle := Bundle{BundleVersion: BundleVersion, SourceFingerprint: fingerprint, RecordCount: len(records), SecretCount: secretCount, Encrypted: EncryptedPayload{Version: 1, Algorithm: "X25519-HKDF-SHA256-AES-256-GCM", Purpose: "profile-cutover", EphemeralPublicKey: base64.StdEncoding.EncodeToString(ephemeralSPKI), Nonce: base64.StdEncoding.EncodeToString(nonce), Ciphertext: base64.StdEncoding.EncodeToString(ciphertext)}}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestImportRawAndSecretRecords(t *testing.T) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	records := []Record{
		{Domain: "settings", Key: "theme", Classification: "canonical-migrated", ValueBase64: base64.StdEncoding.EncodeToString([]byte("dark"))},
		{Domain: "vault", Key: "host.password", Classification: "re-seal-secrets", Plaintext: "secret"},
	}
	bundle := makeTestBundle(t, privateKey, records, "fingerprint-123456")
	mutations, err := Import(bundle, privateKey, "fingerprint-123456", fakeProvider{available: true})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(mutations) != 2 || string(mutations[0].Value) != "dark" || string(mutations[1].Value) != "sealed:profile-migration/vault/host.password:secret" {
		t.Fatalf("unexpected mutations: %+v", mutations)
	}
}

func TestImportRejectsFingerprintAndMalformed(t *testing.T) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	bundle := makeTestBundle(t, privateKey, []Record{{Domain: "settings", Key: "theme", Classification: "canonical-migrated", ValueBase64: "ZGFyaw=="}}, "fingerprint-123456")
	if _, err := Import(bundle, privateKey, "other-fingerprint", fakeProvider{available: true}); !errors.Is(err, ErrBundleFingerprint) {
		t.Fatalf("fingerprint must fail, got %v", err)
	}
	if _, err := Import([]byte("{}"), privateKey, "fingerprint-123456", fakeProvider{available: true}); !errors.Is(err, ErrBundleInvalid) {
		t.Fatalf("malformed must fail, got %v", err)
	}
	if _, err := Import(bundle, privateKey, "fingerprint-123456", fakeProvider{available: false}); err != nil {
		t.Fatalf("raw-only bundle should not require provider: %v", err)
	}
}

func TestImportSecretFailsClosedWithoutProvider(t *testing.T) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	bundle := makeTestBundle(t, privateKey, []Record{{Domain: "vault", Key: "p", Classification: "secret", Plaintext: "secret"}}, "fingerprint-123456")
	if _, err := Import(bundle, privateKey, "fingerprint-123456", nil); !errors.Is(err, credentials.ErrUnavailable) {
		t.Fatalf("secret import must fail closed, got %v", err)
	}
}

func TestImportProducesStoreCompatibleMutations(t *testing.T) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	bundle := makeTestBundle(t, privateKey, []Record{{Domain: "settings", Key: "theme", Classification: "canonical-migrated", ValueBase64: "ZGFyaw=="}}, "fingerprint-123456")
	mutations, err := Import(bundle, privateKey, "fingerprint-123456", nil)
	if err != nil {
		t.Fatal(err)
	}
	if mutations[0].Domain != "settings" || mutations[0].Key != "theme" || mutations[0].Delete || len(mutations[0].Value) == 0 {
		t.Fatalf("not a store mutation: %+v", mutations[0])
	}
	_ = store.MaxRawValueBytes
}
