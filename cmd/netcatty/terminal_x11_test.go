package main

import (
	"context"
	"testing"
)

func TestTerminalX11Disabled(t *testing.T) {
	f, err := startTerminalX11(context.Background(), nil, nil, false, "invalid")
	if f != nil || err != nil {
		t.Fatalf("disabled X11 performed setup: %v %v", f, err)
	}
}
func TestTerminalX11RejectsNonlocalDisplay(t *testing.T) {
	f, err := startTerminalX11(context.Background(), nil, nil, true, "example.com:0")
	if f != nil || err == nil {
		t.Fatal("accepted remote display")
	}
}
func TestTerminalX11Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f, err := startTerminalX11(ctx, nil, nil, true, ":0")
	if f != nil || err == nil {
		t.Fatal("ignored cancelled setup")
	}
}
