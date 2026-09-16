package terminaluse

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	pkgsftp "github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	terminalssh "github.com/binaricat/netcatty/internal/terminal/ssh"
)

func TestAutocompleteLocalDirectories(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"Mihomo", "My files", "中文", "a'quote"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "Mihomo.yaml"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	service := &Service{sessions: map[string]*terminalSession{"local": {cwd: cwdOSC{cwd: root}}}}
	result := service.ListAutocompleteDirectory(context.Background(), "local", ".", true, "mi", 100)
	if !result.Success || len(result.Entries) != 1 || result.Entries[0].Name != "Mihomo" || result.Entries[0].Type != "directory" {
		t.Fatalf("unexpected listing: %+v", result)
	}
	all := service.ListAutocompleteDirectory(context.Background(), "", root, true, "", 100)
	if !all.Success || len(all.Entries) != 4 {
		t.Fatalf("unexpected full listing: %+v", all)
	}
	unknown := service.ListAutocompleteDirectory(context.Background(), "missing", root, true, "", 100)
	if unknown.Success {
		t.Fatal("missing remote session must never fall back to local filesystem")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if service.ListAutocompleteDirectory(ctx, "", root, true, "", 100).Success {
		t.Fatal("cancelled query succeeded")
	}
}

// A real SSH/SFTP server verifies the endpoint does not open an exec channel
// or fall back to a second authenticated connection or the active stdin.
func TestAutocompleteRemoteSameConnection(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Mihomo"), 0700); err != nil {
		t.Fatal(err)
	}
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := gossh.NewSignerFromKey(private)
	config := &gossh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		server, channels, requests, err := gossh.NewServerConn(conn, config)
		if err != nil {
			return
		}
		defer server.Close()
		go gossh.DiscardRequests(requests)
		for incoming := range channels {
			ch, reqs, err := incoming.Accept()
			if err != nil {
				continue
			}
			go func() {
				defer ch.Close()
				for req := range reqs {
					if req.Type != "subsystem" {
						_ = req.Reply(false, nil)
						continue
					}
					var subsystem struct{ Name string }
					_ = gossh.Unmarshal(req.Payload, &subsystem)
					if subsystem.Name != "sftp" {
						_ = req.Reply(false, nil)
						continue
					}
					_ = req.Reply(true, nil)
					sftp, err := pkgsftp.NewServer(ch, pkgsftp.WithServerWorkingDirectory(root))
					if err != nil {
						return
					}
					defer sftp.Close()
					if err := sftp.Serve(); err != nil && err != io.EOF {
						return
					}
					return
				}
			}()
		}
	}()
	client, err := gossh.Dial("tcp", listener.Addr().String(), &gossh.ClientConfig{User: "test", HostKeyCallback: gossh.InsecureIgnoreHostKey(), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	remoteRoot := filepath.ToSlash(root)
	if !strings.HasPrefix(remoteRoot, "/") {
		remoteRoot = "/" + remoteRoot
	}
	service := &Service{sessions: map[string]*terminalSession{"remote": {transport: &terminalssh.Transport{Client: client}, cwd: cwdOSC{cwd: remoteRoot}}}}
	for _, directory := range []string{remoteRoot, "."} {
		result := service.ListAutocompleteDirectory(context.Background(), "remote", directory, true, "Mi", 10)
		if !result.Success || len(result.Entries) != 1 || result.Entries[0].Name != "Mihomo" {
			t.Fatalf("%s: %+v", directory, result)
		}
	}
}

func TestAutocompleteRejectsUnknownRelativeLocalCwd(t *testing.T) {
	service := &Service{sessions: map[string]*terminalSession{}}
	if service.ListAutocompleteDirectory(context.Background(), "", ".", true, "", 100).Success {
		t.Fatal("must not list app cwd in place of terminal cwd")
	}
}
