package ssh

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadIdentityFilePEMsReadsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(path, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nAAA\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pem, err := LoadIdentityFilePEMs([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if string(pem) == "" || string(pem)[:10] != "-----BEGIN" {
		t.Fatalf("pem = %q", pem)
	}
}

func TestLoadIdentityFilePEMsFailsClosedOnMissing(t *testing.T) {
	if _, err := LoadIdentityFilePEMs([]string{filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatal("missing identity file must fail closed")
	}
}

func TestBuildDialConfigUsesAgentFlagWithoutPanic(t *testing.T) {
	policy := StrictPolicy(NewKnownHosts(filepath.Join(t.TempDir(), "known_hosts")))
	config := BuildDialConfig(ConnectInput{
		Hostname:  "h",
		Username:  "u",
		UseAgent:  true,
		EnableMFA: false,
	}, policy, nil)
	if !config.Auth.UseAgent {
		t.Fatal("UseAgent must be copied onto Auth")
	}
}
