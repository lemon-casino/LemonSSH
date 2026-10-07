package updateuse

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Byte-contract tests for the signed release manifest production side
// (scripts/sign-release-manifest.mjs) against this package's verification
// side (verifySignedManifest / verifyIntegrity). The vectors below were
// produced by the Node signer for the RFC 8032 ed25519 TEST 1 seed — a
// public test key, never a release key. If any of these tests fail, the
// Node signer and the Go verifier drifted apart byte-wise and releases
// signed by packaging would be rejected at runtime.

const (
	nodeTestSeedHex = "9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60"
	nodeTestPubHex  = "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a"
	nodeDigestWin   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	nodeDigestLinux = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	// nodeSignedManifest is the exact release-manifest.json body emitted by
	// scripts/sign-release-manifest.mjs (RFC 8032 TEST 1 key) for:
	//   version "1.2.3", publishedMs 1699999999000,
	//   artifacts {darwin-arm64: cc…, linux-amd64: bb…, windows-amd64: aa…},
	//   notes `LemonSSH 1.2.3 <b>&safe</b> "quoted"  line`
	// The notes pin Go's HTML escaping (\u003c/\u003e/\u0026); the artifact
	// order pins the byte-wise map-key sort; the trailing blank-signature
	// field pins the omitempty-less signature contract.
	nodeSignedManifest = `{"version":"1.2.3","publishedMs":1699999999000,"artifacts":{"darwin-arm64":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","linux-amd64":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","windows-amd64":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"notes":"LemonSSH 1.2.3 \u003cb\u003e\u0026safe\u003c/b\u003e \"quoted\"  line","signature":"xqasm1+4dQqRkBjafjLOR0knkd0ydOIUHDrLUMZMeKDU6FuOCjkJVfOSzrdsT+kIYVkaZKKtA9lzT+2iZidYCA=="}`
)

func writeNodeManifest(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, signedManifestAsset), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestVerifySignedManifestAcceptsNodeSignerBytes(t *testing.T) {
	dir := writeNodeManifest(t, nodeSignedManifest)
	digest, err := verifySignedManifest(dir, nodeTestPubHex, "windows", "amd64", time.Now)
	if err != nil {
		t.Fatalf("node-signed manifest rejected: %v", err)
	}
	if digest != nodeDigestWin {
		t.Fatalf("digest = %q, want windows-amd64 entry %q", digest, nodeDigestWin)
	}
	linuxDigest, err := verifySignedManifest(dir, nodeTestPubHex, "linux", "amd64", time.Now)
	if err != nil || linuxDigest != nodeDigestLinux {
		t.Fatalf("linux lookup = %q, %v; want %q", linuxDigest, err, nodeDigestLinux)
	}
	// A platform the signer did not cover yields no digest, which lets
	// verifyIntegrity fall back to checksums.txt (asserted below).
	other, err := verifySignedManifest(dir, nodeTestPubHex, "darwin", "amd64", time.Now)
	if err != nil || other != "" {
		t.Fatalf("uncovered platform = %q, %v; want empty digest", other, err)
	}
}

func TestVerifySignedManifestWithoutPublicKeySkipsManifest(t *testing.T) {
	dir := writeNodeManifest(t, nodeSignedManifest)
	digest, err := verifySignedManifest(dir, "", "windows", "amd64", time.Now)
	if err != nil || digest != "" {
		t.Fatalf("empty public key must skip the manifest, got %q, %v", digest, err)
	}
}

func TestVerifySignedManifestRejectsTamperedNodeManifest(t *testing.T) {
	tampered := `{"version":"9.9.9","publishedMs":1699999999000,"artifacts":{"windows-amd64":"` + nodeDigestWin + `"},"notes":"","signature":"xqasm1+4dQqRkBjafjLOR0knkd0ydOIUHDrLUMZMeKDU6FuOCjkJVfOSzrdsT+kIYVkaZKKtA9lzT+2iZidYCA=="}`
	dir := writeNodeManifest(t, tampered)
	if _, err := verifySignedManifest(dir, nodeTestPubHex, "windows", "amd64", time.Now); err == nil {
		t.Fatal("tampered manifest must be rejected")
	}
}

func TestVerifyIntegrityPrefersVerifiedManifestDigest(t *testing.T) {
	dir := writeNodeManifest(t, nodeSignedManifest)
	checksums := map[string]string{"LemonSSH-1.2.3-windows-amd64.exe": nodeDigestLinux}
	// The verified manifest digest wins over a (here wrong) checksums.txt
	// entry for the same artifact.
	if err := verifyIntegrity(nodeDigestWin, "LemonSSH-1.2.3-windows-amd64.exe", checksums, dir, nodeTestPubHex, "windows", "amd64", time.Now); err != nil {
		t.Fatalf("verified manifest digest must win: %v", err)
	}
	// A downloaded artifact not matching the manifest digest fails.
	if err := verifyIntegrity(nodeDigestLinux, "LemonSSH-1.2.3-windows-amd64.exe", checksums, dir, nodeTestPubHex, "windows", "amd64", time.Now); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("mismatched digest must fail with ErrChecksumMismatch, got %v", err)
	}
	// The manifest covers the platform but the downloaded digest differs
	// from checksums.txt while matching the manifest: still accepted.
	checksumsMissing := map[string]string{"other.exe": nodeDigestLinux}
	if err := verifyIntegrity(nodeDigestWin, "LemonSSH-1.2.3-windows-amd64.exe", checksumsMissing, dir, nodeTestPubHex, "windows", "amd64", time.Now); err != nil {
		t.Fatalf("manifest digest must apply even without a checksums entry: %v", err)
	}
}

func TestVerifyIntegrityWithoutManifestFallsBackToChecksums(t *testing.T) {
	dir := t.TempDir() // no release-manifest.json staged
	checksums := map[string]string{"LemonSSH-1.2.3-windows-amd64.exe": nodeDigestWin}
	if err := verifyIntegrity(nodeDigestWin, "LemonSSH-1.2.3-windows-amd64.exe", checksums, dir, nodeTestPubHex, "windows", "amd64", time.Now); err != nil {
		t.Fatalf("checksums fallback failed: %v", err)
	}
	if err := verifyIntegrity(nodeDigestLinux, "LemonSSH-1.2.3-windows-amd64.exe", checksums, dir, nodeTestPubHex, "windows", "amd64", time.Now); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("checksum mismatch must fail, got %v", err)
	}
	// checksums.txt entry missing for the artifact: refuse.
	if err := verifyIntegrity(nodeDigestWin, "LemonSSH-1.2.3-windows-amd64.exe", map[string]string{}, dir, nodeTestPubHex, "windows", "amd64", time.Now); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("missing checksums entry must fail, got %v", err)
	}
}

// TestVerifyIntegrityContinuesWithoutAnyDigest locks the current degraded
// behavior: a release that publishes neither checksums.txt nor a verifiable
// manifest digest installs unverified, with only a log line. Tightening this
// is a release-policy decision (see the packaging docs), not a drive-by fix.
func TestVerifyIntegrityContinuesWithoutAnyDigest(t *testing.T) {
	dir := t.TempDir()
	if err := verifyIntegrity(nodeDigestWin, "LemonSSH-1.2.3-windows-amd64.exe", nil, dir, "", "windows", "amd64", time.Now); err != nil {
		t.Fatalf("no-digest release must keep installing (current contract), got %v", err)
	}
}
