package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func TestSSHConfiguredKeyPassphraseCertificateAndAgent(t *testing.T) {
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := gossh.NewSignerFromKey(private)
	_, caPrivate, _ := ed25519.GenerateKey(rand.Reader)
	ca, _ := gossh.NewSignerFromKey(caPrivate)
	cert := &gossh.Certificate{Key: signer.PublicKey(), CertType: gossh.UserCert, KeyId: "fixture", ValidPrincipals: []string{"user"}, ValidBefore: gossh.CertTimeInfinity}
	if err := cert.SignCert(rand.Reader, ca); err != nil {
		t.Fatal(err)
	}
	plain, _ := gossh.MarshalPrivateKey(private, "")
	encrypted, _ := gossh.MarshalPrivateKeyWithPassphrase(private, "", []byte("phrase"))
	identity := filepath.Join(t.TempDir(), "identity")
	if err := os.WriteFile(identity, pem.EncodeToMemory(encrypted), 0600); err != nil {
		t.Fatal(err)
	}
	listener := newAuthAgentListener(t)
	t.Setenv("SSH_AUTH_SOCK", listener.Addr().String())
	ring := agent.NewKeyring()
	if err := ring.Add(agent.AddedKey{PrivateKey: private}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() { defer wg.Done(); defer conn.Close(); agent.ServeAgent(ring, conn) }()
		}
	}()
	t.Cleanup(func() { listener.Close(); wg.Wait() })
	for _, kind := range []string{"private-key", "passphrase", "certificate", "identity-file", "agent", "agent-certificate"} {
		t.Run(kind, func(t *testing.T) {
			expected := signer.PublicKey()
			input := ConnectInput{Username: "user"}
			switch kind {
			case "private-key":
				input.PrivateKey = string(pem.EncodeToMemory(plain))
			case "passphrase":
				input.PrivateKey = string(pem.EncodeToMemory(encrypted))
				input.Passphrase = "phrase"
			case "certificate":
				input.PrivateKey = string(pem.EncodeToMemory(encrypted))
				input.Passphrase = "phrase"
				input.Certificate = string(gossh.MarshalAuthorizedKey(cert))
				expected = cert
			case "identity-file":
				input.IdentityFilePaths = []string{identity}
				input.Passphrase = "phrase"
			case "agent":
				input.UseAgent = true
			case "agent-certificate":
				input.UseAgent = true
				input.Certificate = string(gossh.MarshalAuthorizedKey(cert))
				expected = cert
			}
			address, policy, _ := sshDialFixture(t, nil, &gossh.ServerConfig{PublicKeyCallback: func(_ gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
				if string(key.Marshal()) != string(expected.Marshal()) {
					return nil, errors.New("wrong configured identity")
				}
				return nil, nil
			}})
			endpoint := configAt(address, policy)
			input.Hostname, input.Port = endpoint.Hostname, endpoint.Port
			config, err := BuildDialConfigErr(input, policy, nil)
			if err != nil {
				t.Fatal(err)
			}
			transport, err := Dial(context.Background(), config)
			if err != nil {
				t.Fatal(err)
			}
			transport.Close()
		})
	}
}

func TestAgentRequestCancelledWithSSHContext(t *testing.T) {
	listener := newAuthAgentListener(t)
	defer listener.Close()
	t.Setenv("SSH_AUTH_SOCK", listener.Addr().String())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := buildAuthMethods(ctx, AuthMethod{UseAgent: true}); done <- err }()
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// A stalled agent has accepted the socket but never answers List.
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled agent request succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("agent blocked SSH cancellation")
	}
}
