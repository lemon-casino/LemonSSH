package terminaluse

import (
	"crypto/ed25519"
	"crypto/rand"
	"github.com/binaricat/lemonssh/internal/terminal/ssh"
	gossh "golang.org/x/crypto/ssh"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestTerminalNativeOptions(t *testing.T) {
	interval, count, disabled := 7, 5, false
	request := SSHConnectRequest{Term: "vt100", VerifyHostKeys: &disabled, KeepaliveInterval: &interval, KeepaliveCountMax: &count}
	config, err := terminalSSHDialConfig(request, nil, ssh.DialInteractive{})
	if err != nil {
		t.Fatal(err)
	}
	if config.KeepaliveInterval != 7*time.Second {
		t.Fatalf("interval %v", config.KeepaliveInterval)
	}
	if config.KeepaliveCountMax != 5 {
		t.Fatalf("count %v", config.KeepaliveCountMax)
	}
	if terminalTerm(request) != "vt100" {
		t.Fatal("term discarded")
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	key, _ := gossh.NewPublicKey(pub)
	if err := config.HostKeyPolicy("h", nil, key); err != nil {
		t.Fatal(err)
	}
	hosts := ssh.NewKnownHosts(filepath.Join(t.TempDir(), "known_hosts"))
	secure, err := terminalSSHDialConfig(SSHConnectRequest{}, hosts, ssh.DialInteractive{})
	if err != nil {
		t.Fatal(err)
	}
	if err := secure.HostKeyPolicy("h", nil, key); err != nil {
		t.Fatal(err)
	}
	pub, _, _ = ed25519.GenerateKey(rand.Reader)
	changed, _ := gossh.NewPublicKey(pub)
	if err := secure.HostKeyPolicy("h", nil, changed); err == nil {
		t.Fatal("default accepted changed key")
	}
	zero := 0
	off, err := terminalSSHDialConfig(SSHConnectRequest{KeepaliveInterval: &zero}, hosts, ssh.DialInteractive{})
	if err != nil || off.KeepaliveInterval != 0 {
		t.Fatalf("disable: %v %v", off.KeepaliveInterval, err)
	}
}

func TestSSHConnectRequestMapsAgentForwarding(t *testing.T) {
	request := SSHConnectRequest{
		Hostname:        "target",
		Username:        "root",
		AgentForwarding: true,
		JumpHosts: []SSHConnectRequest{
			{Hostname: "jump-on", Username: "bastion", AgentForwarding: true},
			{Hostname: "jump-off", Username: "bastion"},
		},
	}
	input := ConnectInputFromRequest(request)
	if !input.AgentForwarding {
		t.Fatal("ConnectInputFromRequest dropped target AgentForwarding")
	}
	config, err := terminalSSHDialConfig(request, nil, ssh.DialInteractive{})
	if err != nil {
		t.Fatal(err)
	}
	if !config.ForwardAgent {
		t.Fatalf("target hop must carry ForwardAgent: %+v", config)
	}
	if len(config.JumpHosts) != 2 {
		t.Fatalf("jump hops: %+v", config.JumpHosts)
	}
	if !config.JumpHosts[0].ForwardAgent {
		t.Fatalf("enabled hop must carry ForwardAgent: %+v", config.JumpHosts[0])
	}
	if config.JumpHosts[1].ForwardAgent {
		t.Fatalf("disabled hop must not carry ForwardAgent: %+v", config.JumpHosts[1])
	}
	plain, err := terminalSSHDialConfig(SSHConnectRequest{Hostname: "target", Username: "root"}, nil, ssh.DialInteractive{})
	if err != nil {
		t.Fatal(err)
	}
	if plain.ForwardAgent {
		t.Fatalf("default must leave ForwardAgent off: %+v", plain)
	}
}

func TestLocalNativeArgvAndValidation(t *testing.T) {
	dir := t.TempDir()
	args := []string{"--login", "a b", "$(touch injected); & echo nope"}
	config := localStartConfig("test", LocalStartRequest{Shell: "custom-shell", ShellArgs: args, CWD: dir, Cols: 90, Rows: 30, Env: map[string]string{"TERM": "vt100"}})
	if config.Shell != "custom-shell" || !reflect.DeepEqual(config.Args, args) || config.CWD != dir || config.Cols != 90 || config.Rows != 30 {
		t.Fatalf("launch %#v", config)
	}
	args[0] = "changed"
	if config.Args[0] != "--login" {
		t.Fatal("argv alias")
	}
	file := filepath.Join(dir, "shell.exe")
	if err := os.WriteFile(file, []byte("test"), 0700); err != nil {
		t.Fatal(err)
	}
	if got := ValidatePath(file, "file"); !got.Exists || !got.IsFile || !got.IsExecutable {
		t.Fatalf("file %#v", got)
	}
	if got := ValidatePath(dir, "directory"); !got.Exists || !got.IsDirectory || got.IsFile {
		t.Fatalf("directory %#v", got)
	}
	if got := ValidatePath(filepath.Join(dir, "missing"), "any"); got.Exists {
		t.Fatalf("missing %#v", got)
	}
	shells := DiscoverShells()
	if len(shells) == 0 {
		t.Fatal("no default shell")
	}
	found := false
	for _, shell := range shells {
		if shell.IsDefault && shell.Command == DefaultShell() {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing default %#v", shells)
	}
}
