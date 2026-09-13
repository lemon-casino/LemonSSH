package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	terminalssh "github.com/binaricat/netcatty/internal/terminal/ssh"
	gossh "golang.org/x/crypto/ssh"
)

func monitoringPeer(t *testing.T, name string, blocked bool) *terminalssh.Transport {
	t.Helper()
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := gossh.NewSignerFromKey(key)
	cfg := &gossh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, e := ln.Accept()
		if e != nil {
			return
		}
		server, channels, requests, e := gossh.NewServerConn(conn, cfg)
		if e != nil {
			return
		}
		defer server.Close()
		go gossh.DiscardRequests(requests)
		for incoming := range channels {
			ch, reqs, e := incoming.Accept()
			if e != nil {
				return
			}
			go func() {
				defer ch.Close()
				for req := range reqs {
					if req.Type != "exec" {
						req.Reply(false, nil)
						continue
					}
					req.Reply(true, nil)
					if blocked {
						for range reqs {
						}
						return
					}
					io.WriteString(ch, "{\"ID\":\""+name+"\",\"Repository\":\"repo\",\"Tag\":\"latest\",\"Size\":\"1MB\",\"CreatedAt\":\"today\"}\n")
					ch.SendRequest("exit-status", false, gossh.Marshal(struct{ Status uint32 }{0}))
					return
				}
			}()
		}
	}()
	client, e := gossh.Dial("tcp", ln.Addr().String(), &gossh.ClientConfig{User: "test", HostKeyCallback: gossh.InsecureIgnoreHostKey(), Timeout: time.Second})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { client.Close() })
	return &terminalssh.Transport{Client: client}
}
func TestMonitoringSSHIdentityCancellation(t *testing.T) {
	a := monitoringPeer(t, "host-a", true)
	b := monitoringPeer(t, "host-b", false)
	s := &TerminalService{sessions: map[string]*terminalSession{"a": {transport: a}, "b": {transport: b}}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	result := s.ListDockerImages(ctx, "a")
	if result.Success || !strings.Contains(result.Error, "deadline") {
		t.Fatal(result)
	}
	result = s.ListDockerImages(context.Background(), "b")
	rows, ok := result.Images.([]map[string]any)
	if !result.Success || !ok || len(rows) != 1 || rows[0]["id"] != "host-b" {
		t.Fatal(result)
	}
	// Cancellation must leave the shared SSH transport usable.
	ch, e := a.Client.NewSession()
	if e != nil {
		t.Fatal(e)
	}
	ch.Close()
	if r := s.ListDockerImages(context.Background(), "missing"); r.Success || r.Error == "" {
		t.Fatal(r)
	}
}
func TestMonitoringEmptyCollectionsSerialize(t *testing.T) {
	result := MonitoringResult{Success: true, Images: []map[string]any{}}
	data, e := json.Marshal(result)
	if e != nil || !strings.Contains(string(data), `"images":[]`) {
		t.Fatalf("%s %v", data, e)
	}
}
