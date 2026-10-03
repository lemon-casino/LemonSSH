package main

import (
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestGenerateED25519KeyPair(t *testing.T) {
	service := &TerminalService{}
	result := service.GenerateKeyPair(KeyPairOptions{Type: "ed25519", Comment: "LemonSSH test"})
	if !result.Success || result.Error != "" {
		t.Fatalf("key generation failed: %+v", result)
	}
	if _, err := gossh.ParseRawPrivateKey([]byte(result.PrivateKey)); err != nil {
		t.Fatalf("private key is not parseable: %v", err)
	}
	publicKey, comment, _, _, err := gossh.ParseAuthorizedKey([]byte(result.PublicKey))
	if err != nil || publicKey == nil {
		t.Fatalf("public key is not parseable: %v", err)
	}
	if !strings.Contains(comment, "LemonSSH test") {
		t.Fatalf("public key comment was not preserved: %q", comment)
	}
}

func TestGenerateKeyPairRejectsUnsupportedSize(t *testing.T) {
	result := (&TerminalService{}).GenerateKeyPair(KeyPairOptions{Type: "RSA", Bits: 1024})
	if result.Success || !strings.Contains(result.Error, "RSA bits") {
		t.Fatalf("invalid RSA size was accepted: %+v", result)
	}
}
