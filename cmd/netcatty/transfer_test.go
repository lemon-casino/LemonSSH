package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/binaricat/netcatty/internal/app/terminaluse"
	"github.com/binaricat/netcatty/internal/rpc"
)

// TestTransferDomainThroughDispatch drives sftp.download and sftp.upload
// over the loopback stack with the fake transfer-capable reader.
func TestTransferDomainThroughDispatch(t *testing.T) {
	dir := t.TempDir()
	reader := &fakeSFTPReader{}
	host := newAgentHost(AgentHostConfig{
		Version:        appVersion{Name: "LemonSSH", Version: "0.0.1"},
		PermissionMode: "auto",
		Sessions: func() []SessionEntry {
			return []SessionEntry{{ID: "sess-1", Kind: "ssh"}}
		},
		SFTP: reader,
	})
	discoveryPath := filepath.Join(dir, "discovery.json")
	if err := host.Start(discoveryPath); err != nil {
		t.Fatalf("host start: %v", err)
	}
	t.Cleanup(func() {
		host.Stop()
		_ = rpc.RemoveDiscovery(discoveryPath)
	})
	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	// Download: local file materializes with the simulated remote content.
	localPath := filepath.Join(dir, "downloaded.txt")
	result, err := client.Call(context.Background(), "netcatty/sftp/download", map[string]any{
		"chatSessionId": "chat-1",
		"sessionId":     "sess-1",
		"remotePath":    "/var/log/app.log",
		"localPath":     localPath,
	})
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if !strings.Contains(string(result), `"bytes":16`) {
		t.Errorf("download result = %s", result)
	}
	content, err := os.ReadFile(localPath)
	if err != nil || string(content) != "remote file body" {
		t.Errorf("downloaded content = %q (%v)", content, err)
	}

	// Upload: local bytes move to the simulated remote path.
	localSource := filepath.Join(dir, "upload-me.txt")
	if err := os.WriteFile(localSource, []byte("upload payload"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	result, err = client.Call(context.Background(), "netcatty/sftp/upload", map[string]any{
		"chatSessionId": "chat-1",
		"sessionId":     "sess-1",
		"localPath":     localSource,
		"remotePath":    "/tmp/upload-me.txt",
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if !strings.Contains(string(result), `"bytes":14`) {
		t.Errorf("upload result = %s", result)
	}
	if reader.lastUpload != "upload payload" {
		t.Errorf("uploaded bytes = %q", reader.lastUpload)
	}

	// Missing params fail before touching the reader.
	before := reader.listCalls
	if _, err := client.Call(context.Background(), "netcatty/sftp/download", map[string]any{
		"chatSessionId": "chat-1",
		"sessionId":     "sess-1",
	}); err == nil {
		t.Fatal("download without remotePath must fail")
	}
	if reader.listCalls != before {
		t.Errorf("failed call must not invoke the reader")
	}
}

// TestTransferConfirmModeFailsClosed pins the write rule: transfers are
// writes and confirm mode without a gate refuses them (T10 family).
func TestTransferConfirmModeFailsClosed(t *testing.T) {
	dir := t.TempDir()
	queue := terminaluse.NewJobQueue(func(sessionID string) (terminaluse.CommandRunner, error) {
		return scriptRunner{}, nil
	})
	host := newAgentHost(AgentHostConfig{
		Version:        appVersion{Name: "LemonSSH", Version: "0.0.1"},
		PermissionMode: "confirm",
		Jobs:           queue,
		Sessions: func() []SessionEntry {
			return []SessionEntry{{ID: "sess-1", Kind: "local"}}
		},
		SFTP: &fakeSFTPReader{},
	})
	discoveryPath := filepath.Join(dir, "discovery.json")
	if err := host.Start(discoveryPath); err != nil {
		t.Fatalf("host start: %v", err)
	}
	t.Cleanup(func() {
		host.Stop()
		_ = rpc.RemoveDiscovery(discoveryPath)
	})
	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	_, err = client.Call(context.Background(), "netcatty/sftp/upload", map[string]any{
		"chatSessionId": "chat-1",
		"sessionId":     "sess-1",
		"localPath":     filepath.Join(dir, "src.txt"),
		"remotePath":    "/tmp/dst.txt",
	})
	var rpcErr *rpc.RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != "APPROVAL_GATE_UNAVAILABLE" {
		t.Fatalf("confirm upload must fail APPROVAL_GATE_UNAVAILABLE, got %v", err)
	}
}
