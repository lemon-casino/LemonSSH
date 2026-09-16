package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"testing"
	"time"

	pkgsftp "github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
)

func TestSFTPOpenForTerminalKeepsAuthenticatedConnection(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := gossh.NewSignerFromKey(key)
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
			channel, requests, err := incoming.Accept()
			if err != nil {
				continue
			}
			go func() {
				defer channel.Close()
				for request := range requests {
					var subsystem struct{ Name string }
					_ = gossh.Unmarshal(request.Payload, &subsystem)
					if request.Type != "subsystem" || subsystem.Name != "sftp" {
						_ = request.Reply(false, nil)
						continue
					}
					_ = request.Reply(true, nil)
					server, err := pkgsftp.NewServer(channel)
					if err != nil {
						return
					}
					defer server.Close()
					_ = server.Serve()
					return
				}
			}()
		}
	}()
	client, err := gossh.Dial("tcp", listener.Addr().String(), &gossh.ClientConfig{User: "authenticated", HostKeyCallback: gossh.InsecureIgnoreHostKey(), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	// Closing the listener proves no new SSH connection can be authenticated.
	_ = listener.Close()
	terminals := newSeamTerminalService(t, []string{"other"}, map[string]*gossh.Client{"exact-native": client})
	service := NewSFTPService(nil, nil)
	service.setTerminalService(terminals)
	first, err := service.OpenForTerminal("exact-native")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.HomeDir(first); err != nil {
		t.Fatal(err)
	}
	if err = service.Close(first); err != nil {
		t.Fatal(err)
	}
	second, err := service.OpenForTerminal("exact-native")
	if err != nil {
		t.Fatalf("closing SFTP closed terminal transport: %v", err)
	}
	defer service.Close(second)
	if first == second {
		t.Fatal("subsystems must have independent identities")
	}
	if _, err = service.OpenForTerminal("other"); err == nil {
		t.Fatal("selected another terminal transport")
	}
}
