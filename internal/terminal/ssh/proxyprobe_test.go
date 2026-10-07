package ssh

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestProbeProxyRejectsEmptyAndUnknownKinds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ProbeProxy(ctx, "command", "", 0, "", "", "", "", 0); err == nil {
		t.Fatal("empty command must fail")
	}
	if err := ProbeProxy(ctx, "ftp", "127.0.0.1", 1080, "", "", "", "", 0); err == nil {
		t.Fatal("unknown proxy kind must fail")
	}
}

func TestProbeProxyUnreachableListenerFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ProbeProxy(ctx, "socks5", "127.0.0.1", 1, "", "", "", "", 0); err == nil {
		t.Fatal("closed proxy port must fail")
	}
}

func TestProbeProxyCommandStart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	command := "sleep 1"
	if runtime.GOOS == "windows" {
		command = "ping -n 2 127.0.0.1"
	}
	if err := ProbeProxy(ctx, "command", "", 0, "", "", command, "127.0.0.1", 1); err != nil {
		t.Fatalf("command proxy should start: %v", err)
	}
}
