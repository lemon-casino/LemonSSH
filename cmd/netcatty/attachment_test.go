package main

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/binaricat/netcatty/internal/rpc"
)

func base64Of(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}

// TestAttachmentDomainThroughDispatch drives the W13 attachment surface
// end to end: renderer registration -> list (summaries only) -> read
// (content + text detection) -> scope isolation.
func TestAttachmentDomainThroughDispatch(t *testing.T) {
	dir := t.TempDir()
	registry := newAttachmentRegistry()
	host := newAgentHost(AgentHostConfig{
		Version:     appVersion{Name: "LemonSSH", Version: "0.0.1"},
		Attachments: registry,
	})
	discoveryPath := filepath.Join(dir, "discovery.json")
	if err := host.Start(discoveryPath); err != nil {
		t.Fatalf("host start: %v", err)
	}
	t.Cleanup(func() {
		host.Stop()
		_ = rpc.RemoveDiscovery(discoveryPath)
	})

	// The renderer registers one inline text attachment and one
	// path-backed binary attachment for chat-1.
	service := newAgentService(nil, false, registry, nil, nil)
	if err := service.AgentRegisterChatAttachments("chat-1", []Attachment{
		{Filename: "hosts.csv", MediaType: "text/csv", Base64: base64Of("hostname,port\nh1,22")},
		{Filename: "capture.bin", MediaType: "application/octet-stream", Base64: base64Of("\x00\x01")},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	// List: summaries without content.
	list, err := client.Call(context.Background(), "netcatty/listAttachments", map[string]any{"chatSessionId": "chat-1"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if strings.Contains(string(list), "aG9zdG5hbWU=") {
		t.Errorf("list must not carry inline content: %s", list)
	}
	if !strings.Contains(string(list), "hosts.csv") || !strings.Contains(string(list), "capture.bin") {
		t.Errorf("list missing entries: %s", list)
	}

	// Read the text attachment: content plus text projection.
	read, err := client.Call(context.Background(), "netcatty/readAttachment", map[string]any{
		"chatSessionId": "chat-1",
		"filename":      "hosts.csv",
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(read), `"text":"hostname,port\nh1,22"`) {
		t.Errorf("text projection missing: %s", read)
	}

	// Scope isolation: another chat sees nothing.
	if _, err := client.Call(context.Background(), "netcatty/listAttachments", map[string]any{"chatSessionId": "chat-other"}); err != nil {
		t.Fatalf("other chat list: %v", err)
	}
	if _, err := client.Call(context.Background(), "netcatty/readAttachment", map[string]any{
		"chatSessionId": "chat-other",
		"filename":      "hosts.csv",
	}); err == nil {
		t.Fatal("cross-chat read must fail")
	}

	// Missing params fail with the ported messages.
	if _, err := client.Call(context.Background(), "netcatty/readAttachment", map[string]any{"chatSessionId": "chat-1"}); err == nil {
		t.Fatal("read without filePath/filename must fail")
	}
}

// TestAttachmentReadFromFilePath covers the path-backed variant: content
// loads from the registered host-readable path.
func TestAttachmentReadFromFilePath(t *testing.T) {
	dir := t.TempDir()
	bodyPath := filepath.Join(dir, "report.md")
	if err := writeFile(bodyPath, []byte("# report")); err != nil {
		t.Fatalf("write body: %v", err)
	}
	registry := newAttachmentRegistry()
	registry.Register("chat-9", []Attachment{
		{Filename: "report.md", MediaType: "text/markdown", FilePath: bodyPath},
	})

	attachment, err := registry.Find("chat-9", "", "report.md")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	content, err := loadAttachmentContent(attachment)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if string(content) != "# report" {
		t.Errorf("content = %q", content)
	}
	if !isLikelyTextAttachment("text/markdown", "report.md") {
		t.Errorf("markdown must be text-detected")
	}
}
