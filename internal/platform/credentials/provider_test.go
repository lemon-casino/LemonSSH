package credentials

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	gokeyring "github.com/zalando/go-keyring"
)

type memoryKeyring struct {
	mu              sync.Mutex
	m               map[string]string
	err             error
	getErrByService map[string]error // injected per-service Get failures
	setErrByUser    map[string]error // injected per-user Set rejections
}

func newMemoryKeyring() *memoryKeyring { return &memoryKeyring{m: make(map[string]string)} }
func (k *memoryKeyring) Get(service, user string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if err, ok := k.getErrByService[service]; ok {
		return "", err
	}
	if k.err != nil {
		return "", k.err
	}
	value, ok := k.m[service+"/"+user]
	if !ok {
		return "", gokeyring.ErrNotFound
	}
	return value, nil
}
func (k *memoryKeyring) Set(service, user, password string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if err, ok := k.setErrByUser[user]; ok {
		return err
	}
	if k.err != nil {
		return k.err
	}
	k.m[service+"/"+user] = password
	return nil
}
func (k *memoryKeyring) Delete(service, user string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, service+"/"+user)
	return nil
}

// purposeUser mirrors the provider's internal per-purpose keyring username.
func purposeUser(purpose string) string {
	digest := sha256.Sum256([]byte(purpose))
	return fmt.Sprintf("credential-purpose-%x", digest[:])
}

func encodePurposeKeyForTest(key []byte) string {
	return base64.RawStdEncoding.EncodeToString(key)
}

func TestProviderRoundTripAndPurposeBinding(t *testing.T) {
	provider := New(newMemoryKeyring())
	if !provider.Available() {
		t.Fatal("memory keyring must be available")
	}
	plaintext := []byte("secret-value")
	envelope, err := provider.Seal(plaintext, "host.password")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	opened, err := provider.Open(envelope, "host.password")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if string(opened) != string(plaintext) {
		t.Fatalf("roundtrip mismatch: %s", opened)
	}
	zero(opened)
	if _, err := provider.Open(envelope, "host.other"); !errors.Is(err, ErrPurposeMismatch) {
		t.Fatalf("cross-purpose replay must fail, got %v", err)
	}
	second, err := provider.Seal(plaintext, "host.password")
	if err != nil {
		t.Fatalf("second seal: %v", err)
	}
	if string(second) == string(envelope) {
		t.Fatal("fresh seals must use a fresh nonce")
	}
}

func TestProviderRejectsMalformedAndTamperedEnvelopes(t *testing.T) {
	provider := New(newMemoryKeyring())
	envelope, err := provider.Seal([]byte("secret"), "key.passphrase")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	bad := append([]byte(nil), envelope...)
	bad[len(bad)-2] ^= 1
	if _, err := provider.Open(bad, "key.passphrase"); err == nil {
		t.Fatal("tampered envelope must fail")
	}
	for _, value := range [][]byte{
		nil,
		[]byte("{}"),
		[]byte(`{"version":1,"provider":"wrong","purpose":"key.passphrase","nonce":"AA","ciphertext":"AA"}`),
		[]byte(`{"version":1,"provider":"os-keyring-aes-gcm","purpose":"other","nonce":"AA","ciphertext":"AA"}`),
	} {
		if _, err := provider.Open(value, "key.passphrase"); err == nil {
			t.Fatalf("malformed envelope accepted: %s", value)
		}
	}
}

func TestProviderFailClosedWhenKeyringUnavailable(t *testing.T) {
	keyring := newMemoryKeyring()
	keyring.err = errors.New("secret service unavailable")
	provider := New(keyring)
	if provider.Available() {
		t.Fatal("unavailable keyring must report unavailable")
	}
	if _, err := provider.Seal([]byte("secret"), "host.password"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("seal must fail closed, got %v", err)
	}
	if _, err := provider.Open([]byte(`{"version":1}`), "host.password"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("open must fail closed, got %v", err)
	}
}

func TestProviderBoundsAndPurposeValidation(t *testing.T) {
	provider := New(newMemoryKeyring())
	for _, purpose := range []string{"", " host.password", "host password", strings.Repeat("x", 257)} {
		if _, err := provider.Seal([]byte("secret"), purpose); !errors.Is(err, ErrInvalidPurpose) {
			t.Fatalf("invalid purpose %q accepted: %v", purpose, err)
		}
	}
	if _, err := provider.Seal(nil, "host.password"); !errors.Is(err, ErrPlaintextTooLarge) {
		t.Fatalf("empty plaintext must fail, got %v", err)
	}
	if _, err := provider.Seal(make([]byte, MaxPlaintext+1), "host.password"); !errors.Is(err, ErrPlaintextTooLarge) {
		t.Fatalf("oversized plaintext must fail, got %v", err)
	}
}

func TestProviderKeyIsSeparatedPerPurpose(t *testing.T) {
	keyring := newMemoryKeyring()
	provider := New(keyring)
	first, err := provider.Seal([]byte("one"), "host.password")
	if err != nil {
		t.Fatalf("first seal: %v", err)
	}
	second, err := provider.Seal([]byte("two"), "host.other")
	if err != nil {
		t.Fatalf("second seal: %v", err)
	}
	if len(keyring.m) != 2 { // availability probe is deleted; one key per purpose
		t.Fatalf("expected two purpose keys, got %d", len(keyring.m))
	}
	if _, err := provider.Open(first, "host.password"); err != nil {
		t.Fatalf("first open: %v", err)
	}
	if _, err := provider.Open(second, "host.other"); err != nil {
		t.Fatalf("second open: %v", err)
	}
}

// TestProviderReadsLegacyServiceNameAndUpgradesCopy covers the rename
// compatibility path: a purpose key stored under the legacy "Netcatty"
// service name must decrypt, be copied to the new "LemonSSH" service name,
// and never be deleted from the legacy slot.
func TestProviderReadsLegacyServiceNameAndUpgradesCopy(t *testing.T) {
	keyring := newMemoryKeyring()
	legacyKey := make([]byte, 32)
	for i := range legacyKey {
		legacyKey[i] = byte(i + 1)
	}
	encoded := encodePurposeKeyForTest(legacyKey)
	user := purposeUser("host.password")
	keyring.m[legacyKeyringService+"/"+user] = encoded

	provider := New(keyring)
	envelope, err := provider.Seal([]byte("secret"), "host.password")
	if err != nil {
		t.Fatalf("seal with legacy key: %v", err)
	}
	if got := keyring.m[keyringService+"/"+user]; got != encoded {
		t.Fatalf("upgrade copy must land under the new service name, got %q", got)
	}
	if got := keyring.m[legacyKeyringService+"/"+user]; got != encoded {
		t.Fatalf("legacy entry must be preserved, got %q", got)
	}
	opened, err := provider.Open(envelope, "host.password")
	if err != nil {
		t.Fatalf("open after upgrade copy: %v", err)
	}
	if string(opened) != "secret" {
		t.Fatalf("roundtrip mismatch: %s", opened)
	}
}

// TestProviderPrefersNewServiceNameOverLegacy proves the new service name
// wins when both entries exist, and that the legacy entry is left untouched.
func TestProviderPrefersNewServiceNameOverLegacy(t *testing.T) {
	keyring := newMemoryKeyring()
	newKey := make([]byte, 32)
	legacyKey := make([]byte, 32)
	for i := range newKey {
		newKey[i] = byte(0xA0 + i)
		legacyKey[i] = byte(0x50 + i)
	}
	user := purposeUser("host.password")
	keyring.m[keyringService+"/"+user] = encodePurposeKeyForTest(newKey)
	keyring.m[legacyKeyringService+"/"+user] = encodePurposeKeyForTest(legacyKey)

	provider := New(keyring)
	envelope, err := provider.Seal([]byte("secret"), "host.password")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if got := keyring.m[legacyKeyringService+"/"+user]; got != encodePurposeKeyForTest(legacyKey) {
		t.Fatal("legacy entry must not be overwritten when the new name hits")
	}
	// The envelope must be encrypted with the new-name key: remove it and the
	// legacy key can no longer decrypt it.
	delete(keyring.m, keyringService+"/"+user)
	if _, err := provider.Open(envelope, "host.password"); !errors.Is(err, ErrPurposeMismatch) {
		t.Fatalf("envelope must be bound to the new-name key, got %v", err)
	}
}

// TestProviderReadSucceedsWhenUpgradeSetRejected proves a rejected upgrade
// copy never blocks the read: the legacy key still decrypts and no new-name
// entry appears.
func TestProviderReadSucceedsWhenUpgradeSetRejected(t *testing.T) {
	keyring := newMemoryKeyring()
	legacyKey := make([]byte, 32)
	for i := range legacyKey {
		legacyKey[i] = byte(i + 7)
	}
	encoded := encodePurposeKeyForTest(legacyKey)
	user := purposeUser("host.password")
	keyring.m[legacyKeyringService+"/"+user] = encoded
	keyring.setErrByUser = map[string]error{user: errors.New("keychain ACL denied")}

	provider := New(keyring)
	envelope, err := provider.Seal([]byte("secret"), "host.password")
	if err != nil {
		t.Fatalf("seal must succeed despite rejected upgrade copy: %v", err)
	}
	if _, ok := keyring.m[keyringService+"/"+user]; ok {
		t.Fatal("rejected upgrade copy must not write the new-name entry")
	}
	if got := keyring.m[legacyKeyringService+"/"+user]; got != encoded {
		t.Fatal("legacy entry must be preserved")
	}
	opened, err := provider.Open(envelope, "host.password")
	if err != nil || string(opened) != "secret" {
		t.Fatalf("open must succeed via the legacy entry: %v", err)
	}
}

// TestProviderNonNotFoundGetDoesNotMintKey proves fail-closed behavior: when
// a keyring Get returns an error other than ErrNotFound (unavailable service,
// ACL denial, locked Secret Service), no replacement key may be generated —
// otherwise existing ciphertext would silently become undecryptable.
func TestProviderNonNotFoundGetDoesNotMintKey(t *testing.T) {
	for _, service := range []string{keyringService, legacyKeyringService} {
		keyring := newMemoryKeyring()
		keyring.getErrByService = map[string]error{service: errors.New("secret service locked")}
		provider := New(keyring)
		if _, err := provider.Seal([]byte("secret"), "host.password"); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("seal must fail closed for %s Get failure, got %v", service, err)
		}
		if len(keyring.m) != 0 { // availability probe is deleted; no key may be minted
			t.Fatalf("no purpose key may be minted for %s Get failure, have %v", service, keyring.m)
		}
	}
}
