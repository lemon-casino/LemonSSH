package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/terminal/dataplane"
	gossh "golang.org/x/crypto/ssh"
)

func TestTerminalActualSSHExit(t *testing.T) {
	for _, code := range []int{0, 9, -1} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			_, key, _ := ed25519.GenerateKey(rand.Reader)
			signer, _ := gossh.NewSignerFromKey(key)
			config := &gossh.ServerConfig{NoClientAuth: true}
			config.AddHostKey(signer)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			release := make(chan struct{})
			serverDone := make(chan struct{})
			go func() {
				defer close(serverDone)
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
				incoming, ok := <-channels
				if !ok {
					return
				}
				channel, reqs, err := incoming.Accept()
				if err != nil {
					return
				}
				defer channel.Close()
				for request := range reqs {
					_ = request.Reply(true, nil)
					if request.Type == "shell" {
						<-release
						if code >= 0 {
							_, _ = channel.SendRequest("exit-status", false, gossh.Marshal(struct{ Status uint32 }{uint32(code)}))
						}
						return
					}
				}
			}()
			controller := dataplane.NewRouteController()
			service := NewTerminalService(controller, dataplane.NewServer(controller, "127.0.0.1:0"), nil)
			verify := false
			id, err := service.Connect(SSHConnectRequest{Hostname: "127.0.0.1", Port: uint16(listener.Addr().(*net.TCPAddr).Port), Username: "test", Password: "test", VerifyHostKeys: &verify})
			if err != nil {
				close(release)
				t.Fatal(err)
			}
			defer service.Close(id)
			close(release)
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				if status := service.GetExitStatus(id); status != nil {
					if code >= 0 && (status.Reason != "exited" || status.ExitCode == nil || *status.ExitCode != code) {
						t.Fatalf("SSH exit mismatch: %+v", status)
					}
					if code < 0 && (status.Reason != "error" || status.ExitCode != nil) {
						t.Fatalf("missing SSH status guessed a code: %+v", status)
					}
					<-serverDone
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatal("SSH exit not observed")
		})
	}
}
