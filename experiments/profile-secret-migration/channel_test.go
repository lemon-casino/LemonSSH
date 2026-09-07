package migrationprobe

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestDeriveChannelKeyDeterministicAndTranscriptBound(t *testing.T) {
	shared := bytes.Repeat([]byte{0x11}, 32)
	challenge := bytes.Repeat([]byte{0x22}, 32)
	transcriptHash := sha256.Sum256([]byte("deterministic transcript"))
	first, err := deriveChannelKey(shared, challenge, transcriptHash[:])
	if err != nil {
		t.Fatal(err)
	}
	defer zero(first)
	second, err := deriveChannelKey(shared, challenge, transcriptHash[:])
	if err != nil {
		t.Fatal(err)
	}
	defer zero(second)
	if !bytes.Equal(first, second) {
		t.Fatal("same KDF inputs produced different keys")
	}
	if got := hex.EncodeToString(first); got != "24c3916d6286ae8291194cfa4587c8675217c8c36356770c4f062f89afa847bb" {
		t.Fatalf("KDF contract changed: %s", got)
	}
	changedHash := transcriptHash
	changedHash[0] ^= 0x80
	changed, err := deriveChannelKey(shared, challenge, changedHash[:])
	if err != nil {
		t.Fatal(err)
	}
	defer zero(changed)
	if bytes.Equal(first, changed) {
		t.Fatal("transcript change did not change key")
	}
}

func TestNonceDeterministicAndDirectionSeparated(t *testing.T) {
	electronFirst, err := channelNonce("electron-to-go", 0x0102030405060708)
	if err != nil {
		t.Fatal(err)
	}
	electronSecond, err := channelNonce("electron-to-go", 0x0102030405060708)
	if err != nil {
		t.Fatal(err)
	}
	goNonce, err := channelNonce("go-to-electron", 0x0102030405060708)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(electronFirst, electronSecond) {
		t.Fatal("nonce is not deterministic")
	}
	if bytes.Equal(electronFirst, goNonce) {
		t.Fatal("directions reused nonce")
	}
	if got := hex.EncodeToString(electronFirst); got != "453247310102030405060708" {
		t.Fatalf("electron nonce contract changed: %s", got)
	}
	if got := hex.EncodeToString(goNonce); got != "473245310102030405060708" {
		t.Fatalf("Go nonce contract changed: %s", got)
	}
	if _, err := channelNonce("other", 1); err == nil {
		t.Fatal("invalid direction accepted")
	}
}

func TestCanonicalAADDeterministicAndDirectionSeparated(t *testing.T) {
	runID := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	first := canonicalAAD(runID, "electron-to-go", 7, "host.password", "enc:v1", "profile-secret-migration/host.password")
	defer zero(first)
	second := canonicalAAD(runID, "electron-to-go", 7, "host.password", "enc:v1", "profile-secret-migration/host.password")
	defer zero(second)
	reverse := canonicalAAD(runID, "go-to-electron", 7, "host.password", "enc:v1", "profile-secret-migration/host.password")
	defer zero(reverse)
	if !bytes.Equal(first, second) {
		t.Fatal("AAD is not deterministic")
	}
	if bytes.Equal(first, reverse) {
		t.Fatal("AAD directions are not separated")
	}
	if got := base64.StdEncoding.EncodeToString(first); got != "AAAAJG5ldGNhdHR5LXByb2ZpbGUtc2VjcmV0LW1pZ3JhdGlvbi12MQAAABAAAQIDBAUGBwgJCgsMDQ4PAAAADmVsZWN0cm9uLXRvLWdvAAAAAAAAAAcAAAANaG9zdC5wYXNzd29yZAAAAAZlbmM6djEAAAAmcHJvZmlsZS1zZWNyZXQtbWlncmF0aW9uL2hvc3QucGFzc3dvcmQ=" {
		t.Fatalf("AAD contract changed: %s", got)
	}
}

func TestHandshakeMutualConfirmationAndTranscriptBinding(t *testing.T) {
	privateKey, hello, err := newServerHandshake()
	if err != nil {
		t.Fatal(err)
	}
	client := makeTestClientHandshake(t, hello)
	defer client.destroy()
	serverChannel, serverProof, clientProof, err := finishServerHandshake(privateKey, hello, client.hello)
	if err != nil {
		t.Fatal(err)
	}
	defer serverChannel.destroy()
	defer zero(serverProof)
	defer zero(clientProof)
	if !bytes.Equal(serverProof, client.serverProof) {
		t.Fatal("server proof mismatch")
	}
	decodedClientProof := mustDecodeFixed(t, client.clientConfirmation.Proof, 32)
	defer zero(decodedClientProof)
	if !bytes.Equal(clientProof, decodedClientProof) {
		t.Fatal("client proof mismatch")
	}
	if bytes.Equal(serverProof, clientProof) {
		t.Fatal("role confirmations are not separated")
	}

	body := []byte("authenticated handshake")
	sealed, err := client.channel.seal("electron-to-go", 1, "host.password", "enc:v1", "profile-secret-migration/host.password", body)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := serverChannel.open("electron-to-go", 1, "host.password", "enc:v1", "profile-secret-migration/host.password", sealed)
	if err != nil {
		t.Fatal(err)
	}
	defer zero(opened)
	if !bytes.Equal(opened, body) {
		t.Fatal("handshake channels disagree")
	}

	tamperedProof := append([]byte(nil), clientProof...)
	tamperedProof[0] ^= 1
	if bytes.Equal(tamperedProof, clientProof) {
		t.Fatal("confirmation tamper was not detected")
	}
}

func TestHandshakeRejectsTamperedHello(t *testing.T) {
	privateKey, hello, err := newServerHandshake()
	if err != nil {
		t.Fatal(err)
	}
	client := makeTestClientHandshake(t, hello)
	defer client.destroy()

	cases := map[string]func(clientHello) clientHello{
		"run id": func(value clientHello) clientHello {
			value.RunID = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 16))
			return value
		},
		"challenge": func(value clientHello) clientHello {
			value.Challenge = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32))
			return value
		},
		"key encoding": func(value clientHello) clientHello {
			value.PublicKey = "%%%"
			return value
		},
		"key type": func(value clientHello) clientHello {
			p256, keyErr := ecdh.P256().GenerateKey(deterministicReader{next: 2})
			if keyErr != nil {
				t.Fatal(keyErr)
			}
			spki, keyErr := x509.MarshalPKIXPublicKey(p256.PublicKey())
			if keyErr != nil {
				t.Fatal(keyErr)
			}
			value.PublicKey = base64.StdEncoding.EncodeToString(spki)
			return value
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			channel, serverProof, clientProof, err := finishServerHandshake(privateKey, hello, mutate(client.hello))
			if channel != nil {
				channel.destroy()
			}
			zero(serverProof)
			zero(clientProof)
			if err == nil {
				t.Fatal("tampered hello accepted")
			}
		})
	}
}

func TestAESGCMRejectsWrongAADDirectionAndTamper(t *testing.T) {
	key := bytes.Repeat([]byte{0x5a}, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	channel := &secureChannel{runID: bytes.Repeat([]byte{0x33}, 16), aead: aead}
	defer channel.destroy()
	plaintext := []byte("secret value")
	sealed, err := channel.seal("electron-to-go", 1, "host.password", "enc:v1", "profile-secret-migration/host.password", plaintext)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := channel.open("electron-to-go", 1, "host.password", "enc:v1", "profile-secret-migration/host.password", sealed)
	if err != nil {
		t.Fatal(err)
	}
	zero(opened)
	wrongCases := []struct {
		direction   string
		sequence    uint64
		fixtureID   string
		source      string
		purpose     string
		ciphertext  string
	}{
		{direction: "go-to-electron", sequence: 1, fixtureID: "host.password", source: "enc:v1", purpose: "profile-secret-migration/host.password", ciphertext: sealed},
		{direction: "electron-to-go", sequence: 2, fixtureID: "host.password", source: "enc:v1", purpose: "profile-secret-migration/host.password", ciphertext: sealed},
		{direction: "electron-to-go", sequence: 1, fixtureID: "host.other", source: "enc:v1", purpose: "profile-secret-migration/host.password", ciphertext: sealed},
		{direction: "electron-to-go", sequence: 1, fixtureID: "host.password", source: "safeStorage-raw", purpose: "profile-secret-migration/host.password", ciphertext: sealed},
		{direction: "electron-to-go", sequence: 1, fixtureID: "host.password", source: "enc:v1", purpose: "profile-secret-migration/host.other", ciphertext: sealed},
	}
	for index, test := range wrongCases {
		if opened, err := channel.open(test.direction, test.sequence, test.fixtureID, test.source, test.purpose, test.ciphertext); err == nil {
			zero(opened)
			t.Fatalf("wrong AAD case %d accepted", index)
		}
	}
	ciphertext, err := base64.StdEncoding.Strict().DecodeString(sealed)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext[len(ciphertext)-1] ^= 1
	tampered := base64.StdEncoding.EncodeToString(ciphertext)
	zero(ciphertext)
	if opened, err := channel.open("electron-to-go", 1, "host.password", "enc:v1", "profile-secret-migration/host.password", tampered); err == nil {
		zero(opened)
		t.Fatal("tampered ciphertext accepted")
	}
}
