package credentials

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

type memoryKeyring struct {
	mu  sync.Mutex
	m   map[string]string
	err error
}

func newMemoryKeyring() *memoryKeyring { return &memoryKeyring{m: make(map[string]string)} }
func (k *memoryKeyring) Get(service, user string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.err != nil {
		return "", k.err
	}
	value, ok := k.m[service+"/"+user]
	if !ok {
		return "", errors.New("missing")
	}
	return value, nil
}
func (k *memoryKeyring) Set(service, user, password string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
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
