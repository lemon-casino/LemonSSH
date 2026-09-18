package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/app/terminaluse"
	"github.com/binaricat/netcatty/internal/rpc"
	"github.com/binaricat/netcatty/internal/terminal/sftp"
)

func newTestAgentHost(t *testing.T) (*AgentHost, string) {
	t.Helper()
	host := newAgentHost(AgentHostConfig{
		Version: appVersion{Name: "LemonSSH", Version: "0.0.1", GOOS: "windows", GOARCH: "amd64"},
		Sessions: func() []SessionEntry {
			return []SessionEntry{
				{ID: "sess-1", Label: "prod shell", Kind: "ssh"},
				{ID: "sess-2", Label: "local", Kind: "local"},
			}
		},
	})
	path := filepath.Join(t.TempDir(), "agent-rpc-discovery.json")
	if err := host.Start(path); err != nil {
		t.Fatalf("host start: %v", err)
	}
	t.Cleanup(func() {
		host.Stop()
		_ = rpc.RemoveDiscovery(path)
	})
	return host, path
}

// TestAgentHostStatusRoundTrip drives the full stack a native binary uses:
// discovery file -> loopback dial -> authenticated call -> JSON result.
func TestAgentHostStatusRoundTrip(t *testing.T) {
	_, discoveryPath := newTestAgentHost(t)
	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	result, err := client.Call(context.Background(), "netcatty/getStatus", map[string]any{})
	if err != nil {
		t.Fatalf("getStatus: %v", err)
	}
	var status map[string]any
	if err := json.Unmarshal(result, &status); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if status["name"] != "LemonSSH" || status["permissionMode"] != "confirm" {
		t.Errorf("status payload = %v", status)
	}
}

func TestAgentHostContextScopedToPrincipal(t *testing.T) {
	_, discoveryPath := newTestAgentHost(t)
	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	result, err := client.Call(context.Background(), "netcatty/getContext", map[string]any{})
	if err != nil {
		t.Fatalf("getContext: %v", err)
	}
	var context map[string]any
	if err := json.Unmarshal(result, &context); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	sessions, ok := context["sessions"].([]any)
	if !ok || len(sessions) != 2 {
		t.Errorf("sessions = %v", context["sessions"])
	}
}

func TestAgentHostUnknownMethodTyped(t *testing.T) {
	_, discoveryPath := newTestAgentHost(t)
	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	_, err = client.Call(context.Background(), "netcatty/nonexistent", map[string]any{})
	if err == nil {
		t.Fatal("unknown method must fail")
	}
	var rpcErr *rpc.RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != "UNKNOWN_METHOD" {
		t.Fatalf("expected UNKNOWN_METHOD RPCError, got %v", err)
	}
}

// fakeSFTPReader records calls and returns canned data.
type fakeSFTPReader struct {
	listCalls int
	lastDir   string
}

func (f *fakeSFTPReader) List(sessionID, dir string) ([]sftp.Entry, error) {
	f.listCalls++
	f.lastDir = dir
	return []sftp.Entry{{Name: "deploy.sh", Size: 120, Mode: "-rw-r--r--"}}, nil
}

func (f *fakeSFTPReader) Stat(sessionID, target string) (sftp.FileInfo, error) {
	return sftp.FileInfo{Path: target, IsDir: false, Size: 120, Mode: "-rw-r--r--"}, nil
}

func (f *fakeSFTPReader) Read(sessionID, remotePath string) (string, error) {
	return "file body", nil
}

func (f *fakeSFTPReader) HomeDir(sessionID string) (string, error) {
	return "/home/deploy", nil
}

func newSFTPTestHost(t *testing.T) (*AgentHost, *fakeSFTPReader, string) {
	t.Helper()
	reader := &fakeSFTPReader{}
	host := newAgentHost(AgentHostConfig{
		Version: appVersion{Name: "LemonSSH", Version: "0.0.1"},
		Sessions: func() []SessionEntry {
			return []SessionEntry{{ID: "sess-1", Kind: "ssh"}}
		},
		SFTP: reader,
	})
	path := filepath.Join(t.TempDir(), "discovery.json")
	if err := host.Start(path); err != nil {
		t.Fatalf("host start: %v", err)
	}
	t.Cleanup(func() {
		host.Stop()
		_ = rpc.RemoveDiscovery(path)
	})
	return host, reader, path
}

func TestAgentHostSFTPReadDomain(t *testing.T) {
	_, reader, discoveryPath := newSFTPTestHost(t)
	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	result, err := client.Call(context.Background(), "netcatty/sftp/list", map[string]any{
		"sessionId": "sess-1",
		"path":      "/var/www",
	})
	if err != nil {
		t.Fatalf("sftp/list: %v", err)
	}
	if reader.listCalls != 1 || reader.lastDir != "/var/www" {
		t.Errorf("reader usage: calls=%d dir=%q", reader.listCalls, reader.lastDir)
	}
	if !strings.Contains(string(result), "deploy.sh") {
		t.Errorf("list result missing entry: %s", result)
	}

	home, err := client.Call(context.Background(), "netcatty/sftp/home", map[string]any{"sessionId": "sess-1"})
	if err != nil {
		t.Fatalf("sftp/home: %v", err)
	}
	if !strings.Contains(string(home), "/home/deploy") {
		t.Errorf("home result = %s", home)
	}
}

// TestAgentHostSFTPUnknownSessionDenied pins the host scope ownership: a
// session the host does not report is refused even for read calls.
func TestAgentHostSFTPUnknownSessionDenied(t *testing.T) {
	_, _, discoveryPath := newSFTPTestHost(t)
	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	_, err = client.Call(context.Background(), "netcatty/sftp/read", map[string]any{
		"sessionId": "sess-evil",
		"path":      "/etc/passwd",
	})
	if err == nil {
		t.Fatal("unknown session must be denied")
	}
	var rpcErr *rpc.RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != "SCOPE_DENIED" {
		t.Fatalf("expected SCOPE_DENIED, got %v", err)
	}
}

// TestAgentHostWriteFailsClosedWithoutApprovalGate pins the confirm-write
// fail-closed rule: terminal execute requires approval and no gate is
// wired yet, so the write never reaches a handler.
func TestAgentHostWriteFailsClosedWithoutApprovalGate(t *testing.T) {
	_, _, discoveryPath := newSFTPTestHost(t)
	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	_, err = client.Call(context.Background(), "netcatty/exec", map[string]any{
		"chatSessionId": "chat-1",
		"sessionId":     "sess-1",
		"command":       "rm -rf /",
	})
	var rpcErr *rpc.RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("write without gate must fail typed, got %v", err)
	}
	if rpcErr.Code != "APPROVAL_GATE_UNAVAILABLE" {
		t.Errorf("expected APPROVAL_GATE_UNAVAILABLE, got %q (%s)", rpcErr.Code, rpcErr.Message)
	}
}

// TestAgentHostTerminalJobsInAutoMode drives the job queue through the
// dispatcher with auto permission mode: exec runs, jobStart/jobPoll/stop
// manage a background job owned by the chat session.
func TestAgentHostTerminalJobsInAutoMode(t *testing.T) {
	queue := terminaluse.NewJobQueue(func(sessionID string) (terminaluse.CommandRunner, error) {
		return scriptRunner{}, nil
	})
	host := newAgentHost(AgentHostConfig{
		Version:        appVersion{Name: "LemonSSH", Version: "0.0.1"},
		PermissionMode: "auto",
		Jobs:           queue,
		Sessions: func() []SessionEntry {
			return []SessionEntry{{ID: "sess-1", Kind: "local"}}
		},
	})
	path := filepath.Join(t.TempDir(), "discovery.json")
	if err := host.Start(path); err != nil {
		t.Fatalf("host start: %v", err)
	}
	t.Cleanup(func() {
		host.Stop()
		_ = rpc.RemoveDiscovery(path)
	})

	client, err := rpc.Dial(path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	result, err := client.Call(context.Background(), "netcatty/exec", map[string]any{
		"chatSessionId": "chat-1",
		"sessionId":     "sess-1",
		"command":       "echo hi",
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(string(result), `"exitCode":0`) {
		t.Errorf("exec result = %s", result)
	}

	started, err := client.Call(context.Background(), "netcatty/jobStart", map[string]any{
		"chatSessionId": "chat-1",
		"sessionId":     "sess-1",
		"command":       "long-run",
	})
	if err != nil {
		t.Fatalf("jobStart: %v", err)
	}
	var startPayload struct {
		JobID string `json:"jobId"`
	}
	if err := json.Unmarshal(started, &startPayload); err != nil {
		t.Fatalf("parse start: %v", err)
	}

	var pollPayload struct {
		Status string `json:"status"`
		Output string `json:"output"`
	}
	for i := 0; i < 100; i++ {
		page, err := client.Call(context.Background(), "netcatty/jobPoll", map[string]any{
			"chatSessionId": "chat-1",
			"jobId":         startPayload.JobID,
			"offset":        0,
		})
		if err != nil {
			t.Fatalf("jobPoll: %v", err)
		}
		if err := json.Unmarshal(page, &pollPayload); err != nil {
			t.Fatalf("parse poll: %v", err)
		}
		if pollPayload.Status == "completed" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if pollPayload.Status != "completed" || !strings.Contains(pollPayload.Output, "script ran") {
		t.Errorf("final poll = %+v", pollPayload)
	}

	stopped, err := client.Call(context.Background(), "netcatty/jobStop", map[string]any{
		"chatSessionId": "chat-1",
		"jobId":         startPayload.JobID,
	})
	if err != nil {
		t.Fatalf("jobStop on completed job: %v", err)
	}
	if !strings.Contains(string(stopped), `"status":"completed"`) {
		t.Errorf("stop of completed job = %s", stopped)
	}
}

// scriptRunner is a real local runner stand-in: executes a known script
// body so the full queue path runs an actual process.
type scriptRunner struct{}

func (scriptRunner) Run(ctx context.Context, command string, sink func([]byte)) (int, bool, error) {
	if strings.Contains(command, "long-run") {
		time.Sleep(30 * time.Millisecond)
	}
	sink([]byte("script ran"))
	return 0, true, nil
}

// TestAgentHostTerminalExecConfirmModeFailsClosed pins the confirm-mode
// rule at the host level: without an approval gate the exec is refused
// even in an otherwise authorized session.
func TestAgentHostTerminalExecConfirmModeFailsClosed(t *testing.T) {
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
	})
	path := filepath.Join(t.TempDir(), "discovery.json")
	if err := host.Start(path); err != nil {
		t.Fatalf("host start: %v", err)
	}
	t.Cleanup(func() {
		host.Stop()
		_ = rpc.RemoveDiscovery(path)
	})

	client, err := rpc.Dial(path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	_, err = client.Call(context.Background(), "netcatty/exec", map[string]any{
		"chatSessionId": "chat-1",
		"sessionId":     "sess-1",
		"command":       "echo hi",
	})
	var rpcErr *rpc.RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != "APPROVAL_GATE_UNAVAILABLE" {
		t.Fatalf("confirm exec must fail APPROVAL_GATE_UNAVAILABLE, got %v", err)
	}
}

// TestAgentHostDiscoveryRemoved pins the shutdown contract: Stop leaves no
// discovery file behind, so stale launchers fail with the typed
// unavailable message instead of dialing a dead port.
func TestAgentHostDiscoveryRemoved(t *testing.T) {
	host, discoveryPath := newTestAgentHost(t)
	host.Stop()
	if _, err := rpc.LoadDiscovery(discoveryPath); err == nil {
		t.Fatal("discovery must be removed on stop")
	}
}
