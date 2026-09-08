package ssh

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestAgentAuthMethodUnreachableFailsClosed(t *testing.T) {
	if _, err := AgentAuthMethod(`/nonexistent/agent/sock`); !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("unreachable agent must fail closed, got %v", err)
	}
	if _, err := AgentAuthMethod(""); err == nil {
		// On Windows the default named pipe may exist; on Linux without
		// SSH_AUTH_SOCK this must fail. Only enforce when no default exists.
		if agentAddressFromEnv() == "" {
			t.Fatal("empty default agent must fail closed")
		}
	}
}

func TestProxyDialValidation(t *testing.T) {
	direct, err := ProxyDial(context.Background(), "")
	if err != nil || direct != nil {
		t.Fatalf("empty proxy means direct dial, got %v %v", direct != nil, err)
	}
	if _, err := ProxyDial(context.Background(), "::::not-a-url"); err == nil {
		t.Fatal("invalid proxy URL must fail")
	}
	dial, err := ProxyDial(context.Background(), "socks5://127.0.0.1:1")
	if err != nil || dial == nil {
		t.Fatalf("valid socks5 proxy must build a dialer: %v", err)
	}
	_, err = dial(context.Background(), "tcp", "example.invalid:22")
	if err == nil {
		t.Fatal("dialing through a dead proxy must fail")
	}
}

// The ssh2+1.17.0 patch exists because Node's ssh2 mishandled OpenSSH RSA
// certificate algorithm negotiation. x/crypto/ssh handles RSA certificates
// and rsa-sha2-256/512 natively; this test pins that capability so the Go
// migration can retire the patch.
func TestRSACertificateSignerNegotiation(t *testing.T) {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	userKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	userSigner, err := ssh.NewSignerFromKey(userKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &ssh.Certificate{
		Key:             userSigner.PublicKey(),
		Serial:          1,
		CertType:        ssh.UserCert,
		KeyId:           "netcatty-compat-test",
		ValidPrincipals: []string{"netcatty"},
		ValidAfter:      uint64(time.Now().Add(-time.Minute).Unix()),
		ValidBefore:     uint64(time.Now().Add(time.Hour).Unix()),
	}
	caSigner, err := ssh.NewSignerFromKey(caKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := certificate.SignCert(rand.Reader, caSigner); err != nil {
		t.Fatal(err)
	}
	certSigner, err := ssh.NewCertSigner(certificate, userSigner)
	if err != nil {
		t.Fatalf("cert signer: %v", err)
	}
	if certSigner.PublicKey().Type() != "ssh-rsa-cert-v01@openssh.com" {
		t.Fatalf("unexpected cert type %s", certSigner.PublicKey().Type())
	}
	// The handshake selects rsa-sha2-512/256 via the AlgorithmSigner
	// interface; pin that capability (the ssh2 patch reimplements exactly
	// this selection for Node).
	algorithmSigner, ok := certSigner.(ssh.AlgorithmSigner)
	if !ok {
		t.Fatal("RSA cert signer must implement AlgorithmSigner")
	}
	data := []byte("negotiation probe")
	for _, algorithm := range []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256} {
		signature, err := algorithmSigner.SignWithAlgorithm(rand.Reader, data, algorithm)
		if err != nil {
			t.Fatalf("sign with %s: %v", algorithm, err)
		}
		if signature.Format != algorithm {
			t.Fatalf("expected %s signature, got %s", algorithm, signature.Format)
		}
	}
}
