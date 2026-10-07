package ssh

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestKnownHostsAddLookupAndStrictPolicy(t *testing.T) {
	hosts := NewKnownHosts(filepath.Join(t.TempDir(), "known_hosts"))
	remote := &netAddrStub{}
	key := &staticKey{keyType: "ssh-ed25519", blob: []byte("blob-1")}

	policy := StrictPolicy(hosts)
	if err := policy("host1", remote, key); err != nil {
		t.Fatalf("first sighting must pin: %v", err)
	}
	stored, err := hosts.Lookup("host1", "ssh-ed25519")
	if err != nil || stored == "" {
		t.Fatalf("pinned key missing: %q %v", stored, err)
	}
	if err := policy("host1", remote, key); err != nil {
		t.Fatalf("matching key must pass: %v", err)
	}
	changed := &staticKey{keyType: "ssh-ed25519", blob: []byte("blob-2")}
	mismatchErr := policy("host1", remote, changed)
	if mismatchErr == nil || !strings.Contains(mismatchErr.Error(), "host key mismatch") {
		t.Fatalf("changed key must fail closed, got %v", mismatchErr)
	}
	if err := policy("host1", remote, &staticKey{keyType: "ssh-rsa", blob: []byte("x")}); err != nil {
		t.Fatalf("different key type is a separate entry: %v", err)
	}
}

func TestBuildAuthMethodsOrderAndValidation(t *testing.T) {
	if _, err := BuildAuthMethods(AuthMethod{}); !errors.Is(err, ErrNoAuthMethod) {
		t.Fatalf("empty auth must be rejected: %v", err)
	}
	_, invalidPEMErr := BuildAuthMethods(AuthMethod{PrivateKeyPEM: []byte("not a pem")})
	if invalidPEMErr == nil {
		t.Fatal("invalid PEM must be rejected")
	}
	methods, err := BuildAuthMethods(AuthMethod{
		Password:    "secret",
		Interactive: func(string, bool) (string, error) { return "code", nil },
	})
	if err != nil || len(methods) != 2 {
		t.Fatalf("password + interactive expected, got %d methods (%v)", len(methods), err)
	}
}

func TestStrictPolicyPersistsAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	first := StrictPolicy(NewKnownHosts(path))
	key := &staticKey{keyType: "ssh-ed25519", blob: []byte("persist-me")}
	if err := first("persisted.host", &netAddrStub{}, key); err != nil {
		t.Fatal(err)
	}
	second := StrictPolicy(NewKnownHosts(path))
	if err := second("persisted.host", &netAddrStub{}, key); err != nil {
		t.Fatalf("pinned key must survive across policy instances: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("known-hosts file missing: %v", err)
	}
}

type staticKey struct {
	keyType string
	blob    []byte
}

func (k *staticKey) Type() string                        { return k.keyType }
func (k *staticKey) Marshal() []byte                     { return k.blob }
func (k *staticKey) Verify([]byte, *ssh.Signature) error { return errors.New("not implemented") }

type netAddrStub struct{}

func (*netAddrStub) Network() string { return "tcp" }
func (*netAddrStub) String() string  { return "127.0.0.1:22" }
