package contracts

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpaqueIDsShapeAndValidation(t *testing.T) {
	instance := NewInstanceID()
	if !instance.Valid() {
		t.Fatalf("fresh instance id invalid: %s", instance)
	}
	if !NewWindowID().Valid() || !NewSessionID().Valid() || !NewRequestID().Valid() {
		t.Fatal("fresh ids must validate")
	}
	if NewInstanceID() == NewInstanceID() {
		t.Fatal("ids must be unique")
	}
	tampered := InstanceID(strings.Replace(string(instance), "inst_", "win_", 1))
	if tampered.Valid() {
		t.Fatal("wrong prefix must not validate")
	}
	if InstanceID("inst_short").Valid() {
		t.Fatal("short body must not validate")
	}
	if InstanceID(string(instance[:len(instance)-1]) + "g").Valid() {
		t.Fatal("non-hex body must not validate")
	}
}

func TestAsErrorMapsSentinelsToStableCodes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want ErrorCode
	}{
		{"deadline", context.DeadlineExceeded, CodeDeadlineExceeded},
		{"cancelled", context.Canceled, CodeCancelled},
		{"too large", ErrPayloadTooLarge, CodeInvalidRequest},
		{"depth", ErrDepthExceeded, CodeInvalidRequest},
		{"unsafe int", ErrUnsafeInteger, CodeInvalidRequest},
		{"unknown field", ErrUnknownField, CodeInvalidRequest},
		{"arbitrary", errors.New("boom"), CodeInternal},
	}
	for _, testCase := range cases {
		envelope := AsError(testCase.err)
		if envelope.Code != testCase.want {
			t.Fatalf("%s: got %s want %s", testCase.name, envelope.Code, testCase.want)
		}
	}
	deadline := AsError(context.DeadlineExceeded)
	if !deadline.Retryable {
		t.Fatal("deadline errors should be retryable")
	}
	original := NewError(CodeNotFound, "missing").WithDetail("id", "x")
	if AsError(original) != original {
		t.Fatal("envelopes must pass through unwrapped")
	}
}

type samplePayload struct {
	Name   string            `json:"name"`
	Count  int               `json:"count"`
	Big    int64             `json:"big,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
	Hidden string            `json:"-"`
	Extra  string            `json:"extra,omitempty"`
}

func TestEncodeDecodePolicy(t *testing.T) {
	encoded, err := Encode(samplePayload{Name: "ok", Count: 3, Labels: map[string]string{"a": "b"}})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var decoded samplePayload
	if err := Decode(encoded, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Name != "ok" || decoded.Count != 3 || decoded.Labels["a"] != "b" || decoded.Hidden != "" {
		t.Fatalf("round trip mismatch: %+v", decoded)
	}

	if _, err := Encode(samplePayload{Name: "x", Count: 1, Big: SafeIntegerMax + 1}); !errors.Is(err, ErrUnsafeInteger) {
		t.Fatalf("unsafe int64 must be rejected, got %v", err)
	}

	deep := strings.Repeat("[", MaxDepth+1) + strings.Repeat("]", MaxDepth+1)
	if err := Decode([]byte(deep), &json.RawMessage{}); !errors.Is(err, ErrDepthExceeded) {
		t.Fatalf("depth bound must hold, got %v", err)
	}

	oversized := make([]byte, MaxPayloadBytes+1)
	for i := range oversized {
		oversized[i] = ' '
	}
	if err := Decode(oversized, &json.RawMessage{}); !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatal("size bound must hold on decode")
	}

	unknownField := []byte(`{"name":"x","count":1,"undeclared":true}`)
	if err := Decode(unknownField, &samplePayload{}); err == nil {
		t.Fatal("unknown fields must be rejected")
	}
}

type goldenInstanceIDEnvelope struct {
	ID      InstanceID `json:"id"`
	Owner   WindowID   `json:"owner"`
	Request Request    `json:"request"`
	Error   *Error     `json:"error,omitempty"`
}

func TestGoldenFixturesStable(t *testing.T) {
	fixtures := map[string]any{
		"error.json":   NewError(CodeUnavailable, "shell offline").WithDetail("shell", "wails"),
		"request.json": Request{RequestID: RequestID(idPrefixRequest + fixtureIDBody("abc"))},
		"subscription.json": SubscriptionEvent{
			SubscriptionID: "sub_fixed",
			Topic:          "terminal.output",
			Sequence:       42,
			Payload:        []byte("chunk"),
			Final:          false,
		},
		"instance.json": goldenInstanceIDEnvelope{
			ID:      InstanceID(idPrefixInstance + fixtureIDBody("f1")),
			Owner:   WindowID(idPrefixWindow + fixtureIDBody("ab")),
			Request: Request{RequestID: RequestID(idPrefixRequest + fixtureIDBody("abc"))},
		},
	}
	dir := filepath.Join("..", "..", "..", "testdata", "migration", "contracts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for name, value := range fixtures {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("%s: marshal: %v", name, err)
		}
		target := filepath.Join(dir, name)
		existing, readErr := os.ReadFile(target)
		if readErr == nil && string(existing) == string(encoded)+"\n" {
			continue
		}
		if readErr != nil && !os.IsNotExist(readErr) {
			t.Fatalf("%s: read: %v", name, readErr)
		}
		if readErr == nil {
			t.Fatalf("%s: golden fixture drifted; regenerate via the contracts test", name)
		}
		if err := os.WriteFile(target, append(encoded, '\n'), 0o644); err != nil {
			t.Fatalf("%s: write: %v", name, err)
		}
	}
}

// fixtureIDBody builds a valid id body of exactly idBodyHex hex characters.
func fixtureIDBody(suffix string) string {
	return strings.Repeat("0", idBodyHex-len(suffix)) + suffix
}
