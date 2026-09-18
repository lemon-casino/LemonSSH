package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/binaricat/netcatty/internal/app/terminaluse"
	"github.com/binaricat/netcatty/internal/profile/store"
	"github.com/binaricat/netcatty/internal/rpc"
)

// seedVaultStore opens a real temp profile store and seeds the vault
// domain with the records the read handlers serve.
func seedVaultStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	profileStore, err := store.Open(filepath.Join(dir, "profile.db"), nil)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = profileStore.Close() })

	if _, err := profileStore.Write(store.WriteRequest{Mutations: []store.Mutation{
		{Domain: "vault", Key: "netcatty_hosts_v1", Value: []byte(`[
			{"id":"host-1","label":"prod","hostname":"10.0.0.5","username":"deploy","password":"SUPER-SECRET","telnetPassword":"TELNET-SECRET","proxyConfig":{"host":"127.0.0.1","password":"PROXY-SECRET"}},
			{"id":"host-2","label":"staging","hostname":"10.0.0.6","username":"deploy","notes":"read me"}
		]`)},
		{Domain: "vault", Key: "netcatty_notes_v1", Value: []byte(`[
			{"id":"note-1","title":"Runbook","content":"# steps"}
		]`)},
		{Domain: "vault", Key: "netcatty_identities_v1", Value: []byte(`[
			{"id":"id-1","name":"deploy-key","passphrase":"PHRASE-SECRET"}
		]`)},
		{Domain: "vault", Key: "netcatty_snippets_v1", Value: []byte(`[
			{"id":"snip-1","label":"ls","content":"ls -la"},
			{"id":"script-1","label":"deploy","kind":"script","content":"nct.screen"}
		]`)},
	}}); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	return profileStore
}

func newFullTestHost(t *testing.T) (string, *rpc.Client) {
	t.Helper()
	profileStore := seedVaultStore(t)
	queue := terminaluse.NewJobQueue(func(sessionID string) (terminaluse.CommandRunner, error) {
		return scriptRunner{}, nil
	})
	host := newAgentHost(AgentHostConfig{
		Version: appVersion{Name: "LemonSSH", Version: "0.0.1"},
		Sessions: func() []SessionEntry {
			return []SessionEntry{{ID: "sess-1", Kind: "ssh"}}
		},
		SFTP:           &fakeSFTPReader{},
		Jobs:           queue,
		Vault:          newVaultReader(profileStore),
		PermissionMode: "auto",
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
	t.Cleanup(func() { client.Close() })
	return path, client
}

// TestVaultReadDomainThroughDispatch pins the vault read surface end to
// end: lists, id lookups and the redaction contract over the real store.
func TestVaultReadDomainThroughDispatch(t *testing.T) {
	_, client := newFullTestHost(t)

	// Host list: metadata only, every secret field redacted at any depth.
	result, err := client.Call(context.Background(), "vault/hosts/list", map[string]any{})
	if err != nil {
		t.Fatalf("host list: %v", err)
	}
	if strings.Contains(string(result), "SUPER-SECRET") ||
		strings.Contains(string(result), "TELNET-SECRET") ||
		strings.Contains(string(result), "PROXY-SECRET") {
		t.Fatalf("host list leaked secrets: %s", result)
	}
	if !strings.Contains(string(result), `"hostname":"10.0.0.5"`) {
		t.Errorf("host metadata must survive redaction: %s", result)
	}

	// Host get by id.
	host, err := client.Call(context.Background(), "vault/host/get", map[string]any{"hostId": "host-1"})
	if err != nil {
		t.Fatalf("host get: %v", err)
	}
	if !strings.Contains(string(host), `"label":"prod"`) || strings.Contains(string(host), "SUPER-SECRET") {
		t.Errorf("host get = %s", host)
	}

	// Host notes come from the same record.
	notes, err := client.Call(context.Background(), "vault/host/notes/get", map[string]any{"hostId": "host-2"})
	if err != nil {
		t.Fatalf("notes get: %v", err)
	}
	if !strings.Contains(string(notes), "read me") {
		t.Errorf("notes = %s", notes)
	}

	// Note list/get.
	noteList, err := client.Call(context.Background(), "vault/notes/list", map[string]any{})
	if err != nil {
		t.Fatalf("note list: %v", err)
	}
	if !strings.Contains(string(noteList), "Runbook") {
		t.Errorf("note list = %s", noteList)
	}
	noteGet, err := client.Call(context.Background(), "vault/notes/get", map[string]any{"noteId": "note-1"})
	if err != nil {
		t.Fatalf("note get: %v", err)
	}
	if !strings.Contains(string(noteGet), "Runbook") {
		t.Errorf("note get = %s", noteGet)
	}

	// Identity list must not leak the passphrase.
	identities, err := client.Call(context.Background(), "vault/identities/list", map[string]any{})
	if err != nil {
		t.Fatalf("identity list: %v", err)
	}
	if strings.Contains(string(identities), "PHRASE-SECRET") {
		t.Fatalf("identity list leaked passphrase: %s", identities)
	}

	// Snippets vs scripts filtering.
	snippets, err := client.Call(context.Background(), "vault/snippets/list", map[string]any{})
	if err != nil {
		t.Fatalf("snippets list: %v", err)
	}
	scripts, err := client.Call(context.Background(), "vault/scripts/list", map[string]any{})
	if err != nil {
		t.Fatalf("scripts list: %v", err)
	}
	if !strings.Contains(string(snippets), `"ls"`) || strings.Contains(string(snippets), `"deploy"`) {
		t.Errorf("snippet list must exclude scripts: %s", snippets)
	}
	if !strings.Contains(string(scripts), `"deploy"`) {
		t.Errorf("script list must include the script: %s", scripts)
	}

	// Script get by id.
	scriptGet, err := client.Call(context.Background(), "vault/scripts/get", map[string]any{"scriptId": "script-1"})
	if err != nil {
		t.Fatalf("script get: %v", err)
	}
	if !strings.Contains(string(scriptGet), "nct.screen") {
		t.Errorf("script get = %s", scriptGet)
	}

	// Unknown id fails with a clear message (generic code, host-mapped).
	if _, err := client.Call(context.Background(), "vault/host/get", map[string]any{"hostId": "nope"}); err == nil {
		t.Fatal("unknown host must fail")
	}
}

// TestVaultWriteFailsClosed pins that vault writes stay unavailable: no
// handlers are registered for the write methods, so they never dispatch.
func TestVaultWriteFailsClosed(t *testing.T) {
	_, client := newFullTestHost(t)
	_, err := client.Call(context.Background(), "vault/hosts/create", map[string]any{"hosts": "[]"})
	if err == nil {
		t.Fatal("vault write must fail closed (no handler registered)")
	}
}
