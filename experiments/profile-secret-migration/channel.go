package migrationprobe

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"errors"
)

const protocolName = "netcatty-profile-secret-migration-v1"

type secureChannel struct {
	runID []byte
	aead  cipher.AEAD
}

func newServerHandshake() (*ecdh.PrivateKey, serverHello, error) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, serverHello{}, err
	}
	runID := make([]byte, 16)
	challenge := make([]byte, 32)
	if _, err := rand.Read(runID); err != nil {
		return nil, serverHello{}, err
	}
	if _, err := rand.Read(challenge); err != nil {
		return nil, serverHello{}, err
	}
	spki, err := x509.MarshalPKIXPublicKey(privateKey.PublicKey())
	if err != nil {
		return nil, serverHello{}, err
	}
	return privateKey, serverHello{
		Version:   ProtocolVersion,
		Type:      "server_hello",
		RunID:     base64.StdEncoding.EncodeToString(runID),
		Challenge: base64.StdEncoding.EncodeToString(challenge),
		PublicKey: base64.StdEncoding.EncodeToString(spki),
	}, nil
}

func finishServerHandshake(privateKey *ecdh.PrivateKey, hello serverHello, client clientHello) (*secureChannel, []byte, []byte, error) {
	if privateKey == nil {
		return nil, nil, nil, protocolError("server key")
	}
	if client.Version != ProtocolVersion || client.Type != "client_hello" || client.RunID != hello.RunID || client.Challenge != hello.Challenge {
		return nil, nil, nil, protocolError("client hello")
	}
	runID, err := decodeFixedBase64(hello.RunID, 16)
	if err != nil {
		return nil, nil, nil, err
	}
	challenge, err := decodeFixedBase64(hello.Challenge, 32)
	if err != nil {
		zero(runID)
		return nil, nil, nil, err
	}
	defer zero(challenge)
	serverSPKI, err := base64.StdEncoding.Strict().DecodeString(hello.PublicKey)
	if err != nil || len(serverSPKI) == 0 || len(serverSPKI) > 256 {
		zero(runID)
		return nil, nil, nil, protocolError("server key")
	}
	clientSPKI, err := base64.StdEncoding.Strict().DecodeString(client.PublicKey)
	if err != nil || len(clientSPKI) > 256 {
		zero(runID)
		return nil, nil, nil, protocolError("client key")
	}
	parsed, err := x509.ParsePKIXPublicKey(clientSPKI)
	if err != nil {
		zero(runID)
		return nil, nil, nil, protocolError("client key")
	}
	clientKey, ok := parsed.(*ecdh.PublicKey)
	if !ok || clientKey.Curve() != ecdh.X25519() {
		zero(runID)
		return nil, nil, nil, protocolError("client key")
	}
	shared, err := privateKey.ECDH(clientKey)
	if err != nil {
		zero(runID)
		return nil, nil, nil, protocolError("key agreement")
	}
	defer zero(shared)
	transcript := canonicalTranscript(runID, challenge, serverSPKI, clientSPKI)
	transcriptHash := sha256.Sum256(transcript)
	zero(transcript)
	key, err := deriveChannelKey(shared, challenge, transcriptHash[:])
	if err != nil {
		zero(runID)
		zero(transcriptHash[:])
		return nil, nil, nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		zero(runID)
		zero(key)
		zero(transcriptHash[:])
		return nil, nil, nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		zero(runID)
		zero(key)
		zero(transcriptHash[:])
		return nil, nil, nil, err
	}
	serverProof := confirmationProof(key, transcriptHash[:], "go")
	clientProof := confirmationProof(key, transcriptHash[:], "electron")
	zero(key)
	zero(transcriptHash[:])
	return &secureChannel{runID: runID, aead: aead}, serverProof, clientProof, nil
}

func deriveChannelKey(shared, challenge, transcriptHash []byte) ([]byte, error) {
	info := protocolName + "/channel-key/" + base64.StdEncoding.EncodeToString(transcriptHash)
	return hkdf.Key(crypto.SHA256.New, shared, challenge, info, 32)
}

func confirmationProof(key, transcriptHash []byte, role string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(protocolName))
	mac.Write([]byte{0})
	mac.Write(transcriptHash)
	mac.Write([]byte{0})
	mac.Write([]byte(role))
	return mac.Sum(nil)
}

func canonicalTranscript(runID, challenge, serverSPKI, clientSPKI []byte) []byte {
	value := make([]byte, 0, 128+len(serverSPKI)+len(clientSPKI))
	value = appendField(value, protocolName)
	value = appendBytes(value, runID)
	value = appendBytes(value, challenge)
	value = appendBytes(value, serverSPKI)
	value = appendBytes(value, clientSPKI)
	return value
}

func (c *secureChannel) open(direction string, sequence uint64, fixtureID, sourceFormat, purpose, encoded string) ([]byte, error) {
	if c == nil || c.aead == nil || sequence == 0 {
		return nil, errors.New("invalid sequence")
	}
	ciphertext, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(ciphertext) < c.aead.Overhead() || len(ciphertext) > MaxFrameBytes {
		zero(ciphertext)
		return nil, errors.New("invalid ciphertext")
	}
	defer zero(ciphertext)
	nonce, err := channelNonce(direction, sequence)
	if err != nil {
		return nil, err
	}
	aad := canonicalAAD(c.runID, direction, sequence, fixtureID, sourceFormat, purpose)
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, aad)
	zero(aad)
	if err != nil {
		return nil, errors.New("authentication failed")
	}
	return plaintext, nil
}

func (c *secureChannel) seal(direction string, sequence uint64, fixtureID, sourceFormat, purpose string, plaintext []byte) (string, error) {
	if c == nil || c.aead == nil || sequence == 0 {
		return "", errors.New("invalid sequence")
	}
	nonce, err := channelNonce(direction, sequence)
	if err != nil {
		return "", err
	}
	aad := canonicalAAD(c.runID, direction, sequence, fixtureID, sourceFormat, purpose)
	ciphertext := c.aead.Seal(nil, nonce, plaintext, aad)
	zero(aad)
	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	zero(ciphertext)
	return encoded, nil
}

func (c *secureChannel) destroy() {
	if c == nil {
		return
	}
	zero(c.runID)
	c.runID = nil
	c.aead = nil
}

func channelNonce(direction string, sequence uint64) ([]byte, error) {
	nonce := make([]byte, 12)
	switch direction {
	case "electron-to-go":
		copy(nonce, []byte("E2G1"))
	case "go-to-electron":
		copy(nonce, []byte("G2E1"))
	default:
		return nil, errors.New("invalid direction")
	}
	binary.BigEndian.PutUint64(nonce[4:], sequence)
	return nonce, nil
}

func canonicalAAD(runID []byte, direction string, sequence uint64, fixtureID, sourceFormat, purpose string) []byte {
	value := make([]byte, 0, 128+len(fixtureID)+len(sourceFormat)+len(purpose))
	value = appendField(value, protocolName)
	value = appendBytes(value, runID)
	value = appendField(value, direction)
	var sequenceBytes [8]byte
	binary.BigEndian.PutUint64(sequenceBytes[:], sequence)
	value = append(value, sequenceBytes[:]...)
	value = appendField(value, fixtureID)
	value = appendField(value, sourceFormat)
	value = appendField(value, purpose)
	return value
}

func appendField(target []byte, value string) []byte {
	return appendBytes(target, []byte(value))
}

func appendBytes(target, value []byte) []byte {
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(value)))
	target = append(target, size[:]...)
	return append(target, value...)
}
