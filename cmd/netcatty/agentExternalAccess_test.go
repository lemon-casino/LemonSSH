package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/rpc"
)

func TestExternalMcpEnableDisableRevokesOnlyExternalCredential(t *testing.T) {
	host, firstPartyPath := newTestAgentHost(t)
	status, err := host.setExternalEnabled(true)
	if err != nil {
		t.Fatal(err)
	}
	path := status["discoveryPath"].(string)
	external, err := rpc.Dial(path)
	if err != nil {
		t.Fatal(err)
	}
	defer external.Close()
	if _, err := external.Call(context.Background(), "public/getEnvironment", map[string]any{"chatSessionId": "forged"}); err != nil {
		t.Fatal(err)
	}
	if _, err := host.setExternalEnabled(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("disabled discovery remains: %v", err)
	}
	_, err = external.Call(context.Background(), "public/getEnvironment", nil)
	var denied *rpc.RPCError
	if !errors.As(err, &denied) || denied.Code != rpc.CodeAuthFailed {
		t.Fatalf("revoked token accepted: %v", err)
	}
	firstParty, err := rpc.Dial(firstPartyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer firstParty.Close()
	if _, err := firstParty.Call(context.Background(), "netcatty/getStatus", map[string]any{}); err != nil {
		t.Fatalf("first-party access was revoked: %v", err)
	}
}

func TestTemporaryExternalAccessExpires(t *testing.T) {
	host, _ := newTestAgentHost(t)
	if _, err := host.setExternalEnabled(true); err != nil {
		t.Fatal(err)
	}
	host.external.mu.Lock()
	host.external.lastActivity = time.Now().Add(-time.Hour)
	host.armExternalTimerLocked()
	host.external.mu.Unlock()
	deadline := time.Now().Add(time.Second)
	for host.externalStatus()["enabled"] == true {
		if time.Now().After(deadline) {
			t.Fatal("temporary access did not expire")
		}
		time.Sleep(time.Millisecond)
	}
}
