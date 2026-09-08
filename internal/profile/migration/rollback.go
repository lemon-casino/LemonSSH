package migration

// Reverse rollback export (P2-06): the Wails store can hand the profile back
// to the Electron shell inside the same encrypted bundle format used for
// cutover. All records are exported as raw values; secrets are already
// provider-sealed envelopes and are NOT unsealed for the rollback path.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/binaricat/netcatty/internal/profile/store"
)

// BuildRollbackBundle reads every raw record from the store and encrypts it
// for the Electron rollback target. Classification is carried per record so
// the Electron side can restore keys to localStorage verbatim.
func BuildRollbackBundle(profileStore *store.Store, targetPublicKeyDER []byte, sourceFingerprint string) ([]byte, error) {
	if profileStore == nil || len(targetPublicKeyDER) == 0 {
		return nil, ErrBundleInvalid
	}
	parsedTarget, err := x509.ParsePKIXPublicKey(targetPublicKeyDER)
	if err != nil {
		return nil, ErrBundleInvalid
	}
	if _, ok := parsedTarget.(*ecdh.PublicKey); !ok {
		return nil, ErrBundleInvalid
	}
	var records []Record
	for _, domain := range store.Domains {
		keys, err := profileStore.DomainKeys(domain)
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			value, err := profileStore.GetRaw(domain, key)
			if err != nil {
				return nil, err
			}
			records = append(records, Record{
				Domain:      domain,
				Key:         key,
				ValueBase64: base64.StdEncoding.EncodeToString(value),
			})
			for i := range value {
				value[i] = 0
			}
		}
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("%w: rollback of an empty store", ErrBundleInvalid)
	}
	payload := payload{Version: BundleVersion, SourceFingerprint: sourceFingerprint, Records: records}
	for _, record := range records {
		payload.Manifest = append(payload.Manifest, ManifestEntry{
			Domain: record.Domain,
			Key:    record.Key,
		})
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	ephemeral, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	targetKey, ok := parsedTarget.(*ecdh.PublicKey)
	if !ok {
		return nil, ErrBundleInvalid
	}
	shared, err := ephemeral.ECDH(targetKey)
	if err != nil {
		return nil, err
	}
	defer zero(shared)
	purpose := "profile-cutover"
	key := hkdfSHA256(shared, []byte(purpose), []byte(BundlePurpose+"/"+purpose), 32)
	defer zero(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	aad := []byte(BundlePurpose + "/" + purpose)
	ciphertext := aead.Seal(nil, nonce, plaintext, aad)
	zero(plaintext)
	ephemeralSPKI, err := x509.MarshalPKIXPublicKey(ephemeral.PublicKey())
	if err != nil {
		return nil, err
	}
	bundle := Bundle{
		BundleVersion:     BundleVersion,
		SourceFingerprint: sourceFingerprint,
		RecordCount:       len(records),
		SecretCount:       0,
		Encrypted: EncryptedPayload{
			Version:            1,
			Algorithm:          "X25519-HKDF-SHA256-AES-256-GCM",
			Purpose:            purpose,
			EphemeralPublicKey: base64.StdEncoding.EncodeToString(ephemeralSPKI),
			Nonce:              base64.StdEncoding.EncodeToString(nonce),
			Ciphertext:         base64.StdEncoding.EncodeToString(ciphertext),
		},
	}
	return json.Marshal(bundle)
}
