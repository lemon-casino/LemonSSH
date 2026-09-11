package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/platform/updater"
)

func writeArtifact(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestArtifactMapInfersPlatformAndHashes(t *testing.T) {
	exe := writeArtifact(t, "LemonSSH-0.0.2-windows-amd64.exe", "win-payload")
	bin := writeArtifact(t, "LemonSSH-0.0.2-linux-arm64", "linux-payload")
	platforms, err := artifactMap([]string{exe, bin})
	if err != nil {
		t.Fatal(err)
	}
	if got := platforms["windows-amd64"]; got == "" {
		t.Fatal("windows-amd64 missing")
	}
	if got := platforms["linux-arm64"]; got == "" {
		t.Fatal("linux-arm64 missing")
	}
	if platforms["windows-amd64"] == platforms["linux-arm64"] {
		t.Fatal("distinct payloads must hash distinctly")
	}
	if _, err := artifactMap([]string{"no-platform-info.txt"}); err == nil {
		t.Fatal("uninferrable name must fail")
	}
}

func TestSignFeedVerifiesWithUpdater(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privateHex := hex.EncodeToString(privateKey)
	exe := writeArtifact(t, "LemonSSH-0.0.2-windows-amd64.exe", "payload-v2")
	out := filepath.Join(t.TempDir(), "latest.json")

	err = runSign([]string{
		"-key", privateHex,
		"-version", "0.0.2",
		"-notes", "signed by test",
		"-out", out,
		exe,
	})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var manifest updater.ReleaseManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "0.0.2" || manifest.Signature == "" {
		t.Fatalf("feed incomplete: %+v", manifest)
	}
	if err := updater.VerifyManifest(manifest, hex.EncodeToString(publicKey), 0, time.Now()); err != nil {
		t.Fatalf("updater must accept the signed feed: %v", err)
	}

	// Any payload drift must break the signature.
	manifest.Artifacts["windows-amd64"] = "0000000000000000000000000000000000000000000000000000000000000000"
	if err := updater.VerifyManifest(manifest, hex.EncodeToString(publicKey), 0, time.Now()); err == nil {
		t.Fatal("tampered artifact hash must fail verification")
	}
}

func TestSignRequiresKeyVersionAndArtifacts(t *testing.T) {
	if err := runSign([]string{"-version", "1.0.0", "-out", "x.json"}); err == nil {
		t.Fatal("missing key must fail")
	}
	if err := runSign([]string{"-key", "abcd", "-version", "1.0.0", "-out", "x.json"}); err == nil {
		t.Fatal("missing artifacts must fail")
	}
	if err := runSign([]string{"-key", "zzzz", "-version", "1.0.0", "-out", "x.json", "a.exe"}); err == nil {
		t.Fatal("non-hex key must fail")
	}
}
