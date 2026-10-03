package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net"
	"path/filepath"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func ed25519Signer(t *testing.T) gossh.PublicKey {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	public, err := gossh.NewPublicKey(private.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	return public
}

func TestKnownHostsReplaceRewritesAndAppends(t *testing.T) {
	hosts := NewKnownHosts(filepath.Join(t.TempDir(), "known_hosts"))
	if err := hosts.Add("h1", "ssh-ed25519", "AAAOLD"); err != nil {
		t.Fatal(err)
	}
	if err := hosts.Add("other", "ssh-ed25519", "AAAOTHER"); err != nil {
		t.Fatal(err)
	}
	if err := hosts.Replace("h1", "ssh-ed25519", "AAANEW"); err != nil {
		t.Fatal(err)
	}
	stored, err := hosts.Lookup("h1", "ssh-ed25519")
	if err != nil || stored != "AAANEW" {
		t.Fatalf("lookup after replace: %q, %v", stored, err)
	}
	if stored, _ := hosts.Lookup("other", "ssh-ed25519"); stored != "AAAOTHER" {
		t.Fatalf("unrelated entry lost: %q", stored)
	}
	// Missing entry appends.
	if err := hosts.Replace("h2", "ssh-ed25519", "AAAAH2"); err != nil {
		t.Fatal(err)
	}
	if stored, _ := hosts.Lookup("h2", "ssh-ed25519"); stored != "AAAAH2" {
		t.Fatalf("append missing: %q", stored)
	}
}

func TestConfirmPolicyStoreRotatesPinnedKey(t *testing.T) {
	hosts := NewKnownHosts(filepath.Join(t.TempDir(), "known_hosts"))
	oldKey := ed25519Signer(t)
	if err := hosts.Add("h", oldKey.Type(), base64.StdEncoding.EncodeToString(oldKey.Marshal())); err != nil {
		t.Fatal(err)
	}
	newKey := ed25519Signer(t)
	confirms := 0
	policy := ConfirmPolicy(hosts, func(change HostKeyChange) (bool, bool, error) {
		confirms++
		if change.Hostname != "h" {
			t.Fatalf("change: %+v", change)
		}
		if change.KeyType != newKey.Type() || change.KnownFingerprint == "" || change.Fingerprint == "" {
			t.Fatalf("fingerprints missing: %+v", change)
		}
		if change.Fingerprint == change.KnownFingerprint {
			t.Fatal("rotated fingerprint must differ")
		}
		if !strings.HasPrefix(change.PublicKey, newKey.Type()+" ") {
			t.Fatalf("public key line: %q", change.PublicKey)
		}
		return true, true, nil
	})
	if err := policy("h", &net.TCPAddr{}, newKey); err != nil {
		t.Fatal(err)
	}
	if confirms != 1 {
		t.Fatalf("confirms %d", confirms)
	}
	stored, _ := hosts.Lookup("h", newKey.Type())
	if stored != base64.StdEncoding.EncodeToString(newKey.Marshal()) {
		t.Fatal("pinned key not rotated")
	}
	// The rotated key now passes without prompting.
	if err := policy("h", &net.TCPAddr{}, newKey); err != nil {
		t.Fatal(err)
	}
	if confirms != 1 {
		t.Fatalf("unexpected extra confirms: %d", confirms)
	}
}

func TestConfirmPolicyAcceptWithoutStoreAllowsOnce(t *testing.T) {
	hosts := NewKnownHosts(filepath.Join(t.TempDir(), "known_hosts"))
	oldKey := ed25519Signer(t)
	if err := hosts.Add("h", oldKey.Type(), base64.StdEncoding.EncodeToString(oldKey.Marshal())); err != nil {
		t.Fatal(err)
	}
	newKey := ed25519Signer(t)
	confirms := 0
	policy := ConfirmPolicy(hosts, func(HostKeyChange) (bool, bool, error) {
		confirms++
		return true, false, nil
	})
	if err := policy("h", &net.TCPAddr{}, newKey); err != nil {
		t.Fatal(err)
	}
	// Stored key untouched, so the next connection prompts again.
	if err := policy("h", &net.TCPAddr{}, newKey); err != nil {
		t.Fatal(err)
	}
	if confirms != 2 {
		t.Fatalf("confirms %d", confirms)
	}
	if stored, _ := hosts.Lookup("h", newKey.Type()); stored != base64.StdEncoding.EncodeToString(oldKey.Marshal()) {
		t.Fatal("stored key must stay untouched when store=false")
	}
}

func TestConfirmPolicyRejectFailsClosed(t *testing.T) {
	hosts := NewKnownHosts(filepath.Join(t.TempDir(), "known_hosts"))
	oldKey := ed25519Signer(t)
	if err := hosts.Add("h", oldKey.Type(), base64.StdEncoding.EncodeToString(oldKey.Marshal())); err != nil {
		t.Fatal(err)
	}
	newKey := ed25519Signer(t)
	policy := ConfirmPolicy(hosts, func(HostKeyChange) (bool, bool, error) {
		return false, false, nil
	})
	err := policy("h", &net.TCPAddr{}, newKey)
	if err == nil || !strings.Contains(err.Error(), "host key mismatch") {
		t.Fatalf("reject: %v", err)
	}
	if stored, _ := hosts.Lookup("h", newKey.Type()); stored != base64.StdEncoding.EncodeToString(oldKey.Marshal()) {
		t.Fatal("stored key must survive a rejection")
	}
}

func TestConfirmPolicyNilKeepsStrictAndFirstSightPins(t *testing.T) {
	hosts := NewKnownHosts(filepath.Join(t.TempDir(), "known_hosts"))
	key := ed25519Signer(t)
	policy := ConfirmPolicy(hosts, nil)
	if err := policy("h", &net.TCPAddr{}, key); err != nil {
		t.Fatalf("first sight must auto-pin: %v", err)
	}
	if stored, _ := hosts.Lookup("h", key.Type()); stored == "" {
		t.Fatal("first sight was not pinned")
	}
	rotated := ed25519Signer(t)
	if err := policy("h", &net.TCPAddr{}, rotated); err == nil {
		t.Fatal("nil confirmer must keep the strict failure")
	}
}

func TestFingerprintMatchesSHA256(t *testing.T) {
	key := ed25519Signer(t)
	fingerprint := Fingerprint(key)
	if !strings.HasPrefix(fingerprint, "SHA256:") {
		t.Fatalf("fingerprint %q", fingerprint)
	}
	decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(fingerprint, "SHA256:"))
	if err != nil {
		t.Fatalf("fingerprint not raw base64: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("sha256 length %d", len(decoded))
	}
}
