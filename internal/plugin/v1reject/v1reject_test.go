package v1reject

import (
	"strings"
	"testing"
)

func TestDetectV1(t *testing.T) {
	v1 := []byte(`{"name":"legacy","main":{"browser":"index.js","node":"index.js"}}`)
	detected, err := DetectV1(v1)
	if err != nil || !detected {
		t.Fatalf("v1 must be detected: detected=%v err=%v", detected, err)
	}
	v2 := []byte(`{"apiVersion":2,"name":"modern","entrypoint":{"wasm":"main.wasm"}}`)
	detected, err = DetectV1(v2)
	if err != nil || detected {
		t.Fatalf("v2 must not be detected as v1: detected=%v err=%v", detected, err)
	}
	malformed := []byte(`{invalid`)
	_, err = DetectV1(malformed)
	if err == nil {
		t.Fatal("malformed JSON must error")
	}
}

func TestRejectMessage(t *testing.T) {
	err := Reject("legacy-plugin")
	if err == nil || !strings.Contains(err.Error(), "legacy-plugin") {
		t.Fatalf("reject message must include plugin ID: %v", err)
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Fatal("reject message must say not supported")
	}
}
