package profiledata

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

var testNow = time.UnixMilli(1_726_000_000_000)

// memSink records plaintexts under their references (test double for the
// credential provider).
type memSink struct {
	values map[string]string
}

func newMemSink() *memSink { return &memSink{values: map[string]string{}} }

func (s *memSink) Store(_ string, plaintext []byte) (string, error) {
	// The service generates its own reference, ignoring the placeholder.
	reference := fmt.Sprintf("secret_%04d", len(s.values)+1)
	s.values[reference] = string(plaintext)
	return reference, nil
}

// codec opens only the origin it knows and seals under the AI purpose.
type fakeCodec struct {
	openOrigin SecretOrigin
	plain      string
}

func (c fakeCodec) Open(origin SecretOrigin, _ []byte) ([]byte, error) {
	if origin != c.openOrigin {
		return nil, &BlockedOriginError{StorageKey: "test", Origin: origin}
	}
	return []byte(c.plain), nil
}

func (c fakeCodec) Seal(plaintext []byte) ([]byte, error) {
	return append([]byte("sealed:"), plaintext...), nil
}

func TestExtractJSONSecrets(t *testing.T) {
	sink := newMemSink()
	providers := []byte(`[
		{"id":"p1","name":"OpenAI","apiKey":"sk-live-123"},
		{"id":"p2","name":"Local","apiKey":""},
		{"id":"p3","name":"Anthropic"}
	]`)

	out, receipts, err := ExtractJSONSecrets("netcatty_ai_providers_v1", providers, sink, testNow)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	// Exactly one secret extracted; the sink holds the plaintext.
	if len(receipts) != 1 || len(sink.values) != 1 {
		t.Fatalf("receipts=%d sink=%d, want 1/1", len(receipts), len(sink.values))
	}
	for reference, plaintext := range sink.values {
		if plaintext != "sk-live-123" {
			t.Errorf("sink plaintext = %q", plaintext)
		}
		if !strings.HasPrefix(reference, "secret_") {
			t.Errorf("reference = %q", reference)
		}
	}

	// The rewritten value carries secretRef + version marker and no raw
	// channel (T38).
	if err := VerifyNoRawSecrets(out); err != nil {
		t.Fatalf("verify: %v", err)
	}
	var configs []map[string]any
	if err := json.Unmarshal(out, &configs); err != nil {
		t.Fatalf("parse rewritten: %v", err)
	}
	if configs[0]["secretRef"] == nil || configs[0]["configVersion"] != float64(2) {
		t.Errorf("config 0 = %v", configs[0])
	}
	if _, present := configs[0]["apiKey"]; present {
		t.Errorf("apiKey must be removed, got %v", configs[0]["apiKey"])
	}
	// Empty/absent keys stay untouched without references.
	if _, present := configs[1]["secretRef"]; present {
		t.Errorf("empty-key config must not gain a reference")
	}

	// Receipts carry fingerprints, never plaintext.
	blob, _ := json.Marshal(receipts)
	if strings.Contains(string(blob), "sk-live-123") {
		t.Errorf("receipts must not carry plaintext: %s", blob)
	}
}

func TestExtractJSONSecretsRejectsMalformed(t *testing.T) {
	_, _, err := ExtractJSONSecrets("netcatty_ai_providers_v1", []byte(`{truncated`), newMemSink(), testNow)
	if err == nil || !strings.Contains(err.Error(), "is not valid JSON") {
		t.Fatalf("malformed JSON must fail typed, got %v", err)
	}
}

func TestExtractJSONSecretsNestedObject(t *testing.T) {
	sink := newMemSink()
	value := []byte(`{"provider":"tavily","apiHost":"https://api.tavily.com","apiKey":"tvly-9"}`)
	out, receipts, err := ExtractJSONSecrets("netcatty_ai_web_search_v1", value, sink, testNow)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(receipts) != 1 || receipts[0].Reference == "" {
		t.Fatalf("receipts = %+v", receipts)
	}
	if err := VerifyNoRawSecrets(out); err != nil {
		t.Fatalf("nested apiKey must be removed: %v", err)
	}
	if !strings.Contains(string(out), "secret_0001") {
		t.Errorf("rewritten value must carry the reference: %s", out)
	}
	if sink.values["secret_0001"] != "tvly-9" {
		t.Errorf("sink = %v", sink.values)
	}
}

func TestResealOpaqueSecretOriginAware(t *testing.T) {
	codec := fakeCodec{openOrigin: OriginElectronBroker, plain: "the-key"}
	envelope := []byte("enc:v1:electron:payload")

	sealed, receipt, err := ResealOpaqueSecret("netcatty_ai_providers_v1", envelope, OriginElectronBroker, codec, testNow)
	if err != nil {
		t.Fatalf("reseal: %v", err)
	}
	if string(sealed) != "sealed:the-key" {
		t.Errorf("sealed = %q", sealed)
	}
	if receipt.Origin != OriginElectronBroker || receipt.SourceFingerprint == "" || receipt.SealedFingerprint == "" {
		t.Errorf("receipt = %+v", receipt)
	}
	if strings.Contains(receipt.SealedFingerprint, "the-key") {
		t.Errorf("receipt fingerprint must not be plaintext")
	}

	// Unknown origin blocks without a speculative open.
	_, _, err = ResealOpaqueSecret("netcatty_ai_providers_v1", envelope, SecretOrigin("unknown"), codec, testNow)
	var blocked *BlockedOriginError
	if err == nil {
		t.Fatal("unknown origin must block")
	}
	if _, ok := err.(*BlockedOriginError); !ok {
		t.Fatalf("error must be BlockedOriginError, got %v", err)
	}
	_ = blocked

	// Open failure at the known origin also aborts (never swallow).
	failing := failingCodec{}
	_, _, err = ResealOpaqueSecret("k", envelope, OriginGoCredential, failing, testNow)
	if err == nil || !strings.Contains(err.Error(), "open via go-credential-provider failed") {
		t.Fatalf("open failure must abort with the origin named, got %v", err)
	}
}

type failingCodec struct{}

func (failingCodec) Open(SecretOrigin, []byte) ([]byte, error) {
	return nil, errors.New("keyring locked")
}

func (failingCodec) Seal([]byte) ([]byte, error) { return nil, errors.New("unreachable") }
