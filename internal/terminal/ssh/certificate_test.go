package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

func TestParseCertificateSignerRoundTrip(t *testing.T) {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	userKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	userSigner, err := gossh.NewSignerFromKey(userKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &gossh.Certificate{
		Key:             userSigner.PublicKey(),
		Serial:          1,
		CertType:        gossh.UserCert,
		KeyId:           "netcatty-cert",
		ValidPrincipals: []string{"root"},
		ValidAfter:      uint64(time.Now().Add(-time.Minute).Unix()),
		ValidBefore:     uint64(time.Now().Add(time.Hour).Unix()),
	}
	caSigner, err := gossh.NewSignerFromKey(caKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := certificate.SignCert(rand.Reader, caSigner); err != nil {
		t.Fatal(err)
	}
	block, err := gossh.MarshalPrivateKey(userKey, "")
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(block)
	certPEM := gossh.MarshalAuthorizedKey(certificate)
	signer, err := ParseCertificateSigner(keyPEM, "", certPEM)
	if err != nil {
		t.Fatal(err)
	}
	if signer.PublicKey().Type() != "ssh-rsa-cert-v01@openssh.com" {
		t.Fatalf("type = %s", signer.PublicKey().Type())
	}
}

func TestParseCertificateSignerFailsClosedWithoutKey(t *testing.T) {
	if _, err := ParseCertificateSigner(nil, "", []byte("ssh-rsa-cert-v01@openssh.com AAAA")); err == nil {
		t.Fatal("certificate without private key must fail closed")
	}
}
