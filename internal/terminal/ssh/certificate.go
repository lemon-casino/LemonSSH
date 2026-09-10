package ssh

import (
	"fmt"
	"strings"

	gossh "golang.org/x/crypto/ssh"
)

// ParseCertificateSigner builds an x/crypto cert signer from a private key PEM
// and an OpenSSH user certificate (authorized_keys line or PEM).
func ParseCertificateSigner(privateKeyPEM []byte, passphrase string, certificate []byte) (gossh.Signer, error) {
	if len(privateKeyPEM) == 0 {
		return nil, fmt.Errorf("certificate auth requires a private key")
	}
	if len(certificate) == 0 {
		return nil, fmt.Errorf("certificate is empty")
	}
	var signer gossh.Signer
	var err error
	if passphrase != "" {
		signer, err = gossh.ParsePrivateKeyWithPassphrase(privateKeyPEM, []byte(passphrase))
	} else {
		signer, err = gossh.ParsePrivateKey(privateKeyPEM)
	}
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}
	parsed, _, _, _, err := gossh.ParseAuthorizedKey(certificate)
	if err != nil {
		trimmed := strings.TrimSpace(string(certificate))
		parsed, _, _, _, err = gossh.ParseAuthorizedKey([]byte(trimmed))
		if err != nil {
			return nil, fmt.Errorf("invalid certificate: %w", err)
		}
	}
	cert, ok := parsed.(*gossh.Certificate)
	if !ok {
		return nil, fmt.Errorf("not an OpenSSH certificate")
	}
	return gossh.NewCertSigner(cert, signer)
}
