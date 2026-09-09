package wasm

import (
	"context"
	"errors"
	"testing"
)

// minimalWASM is the smallest valid WASM binary (magic + version, no exports).
var minimalWASM = []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

func TestRuntimeInstantiateAndClose(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Instantiate(ctx, "test-plugin", minimalWASM); err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	if err := runtime.CloseModule("test-plugin"); err != nil {
		t.Fatalf("close module: %v", err)
	}
	if err := runtime.CloseModule("test-plugin"); err == nil {
		t.Fatal("double close must fail")
	}
	if err := runtime.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeDoubleInstantiateRejected(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(ctx)
	if err := runtime.Instantiate(ctx, "p1", minimalWASM); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Instantiate(ctx, "p1", minimalWASM); !errors.Is(err, ErrAlreadyInstant) {
		t.Fatalf("double instantiate must fail: %v", err)
	}
}

func TestRuntimeCloseAll(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		if err := runtime.Instantiate(ctx, id, minimalWASM); err != nil {
			t.Fatal(err)
		}
	}
	if err := runtime.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Instantiate(ctx, "c", minimalWASM); !errors.Is(err, ErrRuntimeClosed) {
		t.Fatalf("closed runtime must reject: %v", err)
	}
}
