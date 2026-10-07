package updater

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"
)

func generateKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return public, private
}

func TestVerifyManifestValidSignature(t *testing.T) {
	public, private := generateKeyPair(t)
	publicHex := hexEncode(public)
	manifest := ReleaseManifest{
		Version:     "1.1.0",
		PublishedMS: time.Now().UnixMilli(),
		Artifacts:   map[string]string{"windows-x64": "abc123"},
	}
	sign, err := SignManifest(manifest, hexEncode(private))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Signature = sign
	if err := VerifyManifest(manifest, publicHex, 0, time.Now()); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
}

func TestVerifyManifestTampered(t *testing.T) {
	public, private := generateKeyPair(t)
	manifest := ReleaseManifest{
		Version:     "1.1.0",
		PublishedMS: time.Now().UnixMilli(),
		Artifacts:   map[string]string{"win": "abc"},
	}
	sign, err := SignManifest(manifest, hexEncode(private))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Signature = sign
	manifest.Version = "9.9.9" // tampered
	if err := VerifyManifest(manifest, hexEncode(public), 0, time.Now()); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("tampered manifest must fail: %v", err)
	}
}

func TestVerifyManifestReplayAndDowngrade(t *testing.T) {
	public, private := generateKeyPair(t)
	old := ReleaseManifest{Version: "1.0.0", PublishedMS: 1000}
	new := ReleaseManifest{Version: "1.1.0", PublishedMS: 2000}
	for index := range []ReleaseManifest{old, new} {
		sign, err := SignManifest([]ReleaseManifest{old, new}[index], hexEncode(private))
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			old.Signature = sign
		} else {
			new.Signature = sign
		}
	}
	if err := VerifyManifest(old, hexEncode(public), 2000, time.Now()); !errors.Is(err, ErrVersionDowngrade) {
		t.Fatalf("downgrade must fail: %v", err)
	}
	future := ReleaseManifest{Version: "2.0.0", PublishedMS: time.Now().Add(48 * time.Hour).UnixMilli()}
	sign, err := SignManifest(future, hexEncode(private))
	if err != nil {
		t.Fatal(err)
	}
	future.Signature = sign
	if err := VerifyManifest(future, hexEncode(public), 0, time.Now()); !errors.Is(err, ErrManifestFuture) {
		t.Fatalf("future manifest must fail: %v", err)
	}
}

func TestVerifyManifestBadKey(t *testing.T) {
	if _, err := decodePublicKey("short"); err == nil {
		t.Fatal("short key must fail")
	}
}
