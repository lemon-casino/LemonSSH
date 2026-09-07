package migrationprobe

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
)

type fakeProvider struct {
	mu        sync.Mutex
	available bool
	failSeal  bool
	failOpen  bool
	records   map[string]fakeProviderRecord
	sealCalls []string
	openCalls []string
}

type fakeProviderRecord struct {
	purpose   string
	plaintext []byte
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{available: true, records: make(map[string]fakeProviderRecord)}
}

func (p *fakeProvider) Name() string { return ProviderName }

func (p *fakeProvider) Available() bool { return p.available }

func (p *fakeProvider) Seal(plaintext []byte, purpose string) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.available {
		return nil, ErrProviderUnavailable
	}
	if p.failSeal {
		return nil, fmt.Errorf("synthetic seal failure")
	}
	id := fmt.Sprintf("fake-%d", len(p.sealCalls)+1)
	p.records[id] = fakeProviderRecord{purpose: purpose, plaintext: append([]byte(nil), plaintext...)}
	p.sealCalls = append(p.sealCalls, purpose)
	return []byte(id), nil
}

func (p *fakeProvider) Open(envelope []byte, purpose string) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.available {
		return nil, ErrProviderUnavailable
	}
	if p.failOpen {
		return nil, fmt.Errorf("synthetic open failure")
	}
	record, ok := p.records[string(envelope)]
	if !ok || record.purpose != purpose {
		return nil, fmt.Errorf("synthetic envelope mismatch")
	}
	p.openCalls = append(p.openCalls, purpose)
	return append([]byte(nil), record.plaintext...), nil
}

func (p *fakeProvider) destroy() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, record := range p.records {
		zero(record.plaintext)
		delete(p.records, id)
	}
	for index := range p.sealCalls {
		p.sealCalls[index] = ""
	}
	for index := range p.openCalls {
		p.openCalls[index] = ""
	}
}

type testClientHandshake struct {
	hello               clientHello
	channel             *secureChannel
	serverProof         []byte
	clientConfirmation  confirmation
	transcriptHashBytes []byte
}

func makeTestClientHandshake(t *testing.T, server serverHello) testClientHandshake {
	t.Helper()
	runID := mustDecodeFixed(t, server.RunID, 16)
	challenge := mustDecodeFixed(t, server.Challenge, 32)
	serverSPKI, err := base64.StdEncoding.Strict().DecodeString(server.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParsePKIXPublicKey(serverSPKI)
	if err != nil {
		t.Fatal(err)
	}
	serverKey, ok := parsed.(*ecdh.PublicKey)
	if !ok || serverKey.Curve() != ecdh.X25519() {
		t.Fatal("server key is not X25519")
	}
	privateKey, err := ecdh.X25519().GenerateKey(deterministicReader{next: 1})
	if err != nil {
		t.Fatal(err)
	}
	clientSPKI, err := x509.MarshalPKIXPublicKey(privateKey.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	shared, err := privateKey.ECDH(serverKey)
	if err != nil {
		t.Fatal(err)
	}
	transcript := canonicalTranscript(runID, challenge, serverSPKI, clientSPKI)
	transcriptHash := sha256.Sum256(transcript)
	zero(transcript)
	key, err := deriveChannelKey(shared, challenge, transcriptHash[:])
	zero(shared)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	serverProof := confirmationProof(key, transcriptHash[:], "go")
	clientProof := confirmationProof(key, transcriptHash[:], "electron")
	zero(key)
	return testClientHandshake{
		hello: clientHello{
			Version:   ProtocolVersion,
			Type:      "client_hello",
			RunID:     server.RunID,
			Challenge: server.Challenge,
			PublicKey: base64.StdEncoding.EncodeToString(clientSPKI),
		},
		channel:             &secureChannel{runID: runID, aead: aead},
		serverProof:         serverProof,
		clientConfirmation:  confirmation{Version: ProtocolVersion, Type: "client_confirm", Proof: base64.StdEncoding.EncodeToString(clientProof)},
		transcriptHashBytes: append([]byte(nil), transcriptHash[:]...),
	}
}

func (h *testClientHandshake) destroy() {
	h.channel.destroy()
	zero(h.serverProof)
	zero(h.transcriptHashBytes)
	h.clientConfirmation.Proof = ""
}

func (h testClientHandshake) verifyServerConfirmation(t *testing.T, frame confirmation) {
	t.Helper()
	if frame.Version != ProtocolVersion || frame.Type != "server_confirm" {
		t.Fatal("invalid server confirmation shape")
	}
	proof := mustDecodeFixed(t, frame.Proof, 32)
	defer zero(proof)
	if !hmac.Equal(proof, h.serverProof) {
		t.Fatal("server confirmation mismatch")
	}
}

func mustDecodeFixed(t *testing.T, value string, size int) []byte {
	t.Helper()
	decoded, err := decodeFixedBase64(value, size)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func readJSONForTest(t *testing.T, reader frameReader, target any) []byte {
	t.Helper()
	data, err := reader.Read(defaultTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if err := decodeStrictJSON(data, target); err != nil {
		zero(data)
		t.Fatal(err)
	}
	return data
}

func writeJSONForTest(t *testing.T, writer interface{ Write([]byte) (int, error) }, value any) {
	t.Helper()
	if err := writeJSONFrame(writer, value); err != nil {
		t.Fatal(err)
	}
}

func decodeReceiptBodyForTest[T any](t *testing.T, channel *secureChannel, direction string, sequence uint64, id, format, purpose, ciphertext string) T {
	t.Helper()
	opened, err := channel.open(direction, sequence, id, format, purpose, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	defer zero(opened)
	var value T
	if err := decodeStrictJSON(opened, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

type deterministicReader struct {
	next byte
}

func (r deterministicReader) Read(target []byte) (int, error) {
	value := r.next
	for index := range target {
		target[index] = value
		value++
	}
	return len(target), nil
}

func marshalForTest(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
