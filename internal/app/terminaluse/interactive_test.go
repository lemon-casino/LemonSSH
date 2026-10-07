package terminaluse

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"strconv"

	"github.com/lemon-casino/lemonssh/internal/terminal/dataplane"
	terminalssh "github.com/lemon-casino/lemonssh/internal/terminal/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// newInteractiveService builds a Service with temp known-hosts and captures
// interactive prompt events.
type interactiveCapture struct {
	passphrases []terminalssh.PassphraseRequest
	hostKeys    []terminalssh.HostKeyVerificationRequest
}

func newInteractiveService(t *testing.T) (*Service, *interactiveCapture, *terminalssh.KnownHosts) {
	t.Helper()
	controller := dataplane.NewRouteController()
	hosts := terminalssh.NewKnownHosts(filepath.Join(t.TempDir(), "known_hosts"))
	service := New(controller, dataplane.NewServer(controller, "127.0.0.1:0"), hosts)
	capture := &interactiveCapture{}
	service.SetPassphraseEmitter(func(request terminalssh.PassphraseRequest) {
		capture.passphrases = append(capture.passphrases, request)
	})
	service.SetHostKeyVerificationEmitter(func(request terminalssh.HostKeyVerificationRequest) {
		capture.hostKeys = append(capture.hostKeys, request)
	})
	return service, capture, hosts
}

func encryptedKeyPEM(t *testing.T, passphrase string) ([]byte, gossh.PublicKey) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := gossh.MarshalPrivateKeyWithPassphrase(private, "", []byte(passphrase))
	if err != nil {
		t.Fatal(err)
	}
	public, err := gossh.NewPublicKey(private.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(block), public
}

func hostKeySigner(t *testing.T) gossh.PublicKey {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return signer.PublicKey()
}

// newSSHServerFixture runs an in-memory SSH server with the given config;
// every accepted session channel is handed to handleSession.
func newSSHServerFixture(t *testing.T, config *gossh.ServerConfig, handleSession func(channel gossh.Channel, requests <-chan *gossh.Request)) uint16 {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				server, channels, requests, err := gossh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				defer server.Close()
				go gossh.DiscardRequests(requests)
				for incoming := range channels {
					channel, reqs, err := incoming.Accept()
					if err != nil {
						continue
					}
					go handleSession(channel, reqs)
				}
			}()
		}
	}()
	return uint16(listener.Addr().(*net.TCPAddr).Port)
}

// sshKeyAuthServer listens with key-only auth restricted to expected; the
// session phase answers every request so Connect can open its shell.
func sshKeyAuthServer(t *testing.T, expected gossh.PublicKey) uint16 {
	t.Helper()
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := gossh.NewSignerFromKey(private)
	config := &gossh.ServerConfig{PublicKeyCallback: func(_ gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
		if string(key.Marshal()) != string(expected.Marshal()) {
			return nil, io.EOF
		}
		return &gossh.Permissions{}, nil
	}}
	config.AddHostKey(signer)
	return newSSHServerFixture(t, config, func(channel gossh.Channel, requests <-chan *gossh.Request) {
		defer channel.Close()
		for request := range requests {
			_ = request.Reply(true, nil)
		}
	})
}

// execServer handles one session channel running a command: it echoes a line
// to stdout, one to stderr and reports the given exit status.
func execServer(t *testing.T, expected gossh.PublicKey, password string, exitStatus uint32) uint16 {
	t.Helper()
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := gossh.NewSignerFromKey(private)
	config := &gossh.ServerConfig{PasswordCallback: func(_ gossh.ConnMetadata, got []byte) (*gossh.Permissions, error) {
		if string(got) != password {
			return nil, io.EOF
		}
		return &gossh.Permissions{}, nil
	}}
	if expected != nil {
		config = &gossh.ServerConfig{PublicKeyCallback: func(_ gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			if string(key.Marshal()) != string(expected.Marshal()) {
				return nil, io.EOF
			}
			return &gossh.Permissions{}, nil
		}}
	}
	config.AddHostKey(signer)
	return newSSHServerFixture(t, config, func(channel gossh.Channel, requests <-chan *gossh.Request) {
		defer channel.Close()
		for request := range requests {
			if request.Type != "exec" {
				request.Reply(false, nil)
				continue
			}
			request.Reply(true, nil)
			_, _ = channel.Write([]byte("stdout-line\n"))
			_, _ = channel.Stderr().Write([]byte("stderr-line\n"))
			_, _ = channel.SendRequest("exit-status", false, gossh.Marshal(struct{ Status uint32 }{exitStatus}))
			return
		}
	})
}

// answerInteractive responds to each prompt as its event lands. passphrase
// maps a request to the answer (empty = cancel, false = do not answer);
// hostKey maps a request to (accept, store) plus whether to answer at all.
func answerInteractive(service *Service, capture *interactiveCapture, passphrase func(terminalssh.PassphraseRequest) (string, bool), hostKey func(terminalssh.HostKeyVerificationRequest) (bool, bool, bool)) {
	answeredPassphrase, answeredHostKey := false, false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !answeredPassphrase && len(capture.passphrases) > 0 {
			request := capture.passphrases[0]
			if answer, send := passphrase(request); send {
				_ = service.RespondPassphrase(request.RequestID, answer, answer == "")
			}
			answeredPassphrase = true
		}
		if !answeredHostKey && len(capture.hostKeys) > 0 {
			request := capture.hostKeys[0]
			if accept, store, send := hostKey(request); send {
				_ = service.RespondHostKeyVerification(request.RequestID, accept, store)
			}
			answeredHostKey = true
		}
		if answeredPassphrase && answeredHostKey {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

func TestServiceConnectPromptsAndAnswersPassphrase(t *testing.T) {
	pemBytes, public := encryptedKeyPEM(t, "phrase")
	port := sshKeyAuthServer(t, public)
	service, capture, _ := newInteractiveService(t)

	request := SSHConnectRequest{
		Hostname:   "127.0.0.1",
		Port:       port,
		Username:   "user",
		PrivateKey: string(pemBytes),
		SessionID:  "term-1",
		BootEpoch:  7,
	}
	// The passphrase prompt blocks the dial until the broker gets an answer.
	go answerInteractive(service, capture, func(terminalssh.PassphraseRequest) (string, bool) {
		return "phrase", true
	}, nil)
	sessionID, err := service.Connect(request)
	if err != nil {
		t.Fatalf("connect with prompted passphrase: %v", err)
	}
	_ = service.Close(sessionID)
	if len(capture.passphrases) != 1 {
		t.Fatalf("passphrase events: %+v", capture.passphrases)
	}
	requested := capture.passphrases[0]
	if requested.SessionID != "term-1" || requested.BootEpoch != 7 || requested.Hostname != "127.0.0.1" {
		t.Fatalf("session correlation missing: %+v", requested)
	}
	if requested.PassphraseInvalid {
		t.Fatal("first prompt must not be flagged invalid")
	}
}

func TestServiceConnectPassphraseCancelFails(t *testing.T) {
	pemBytes, _ := encryptedKeyPEM(t, "phrase")
	port := sshKeyAuthServer(t, hostKeySigner(t))
	service, capture, _ := newInteractiveService(t)

	go answerInteractive(service, capture, func(terminalssh.PassphraseRequest) (string, bool) {
		return "", true
	}, nil)
	if _, err := service.Connect(SSHConnectRequest{Hostname: "127.0.0.1", Port: port, Username: "user", PrivateKey: string(pemBytes)}); err == nil {
		t.Fatal("cancelled passphrase must fail the dial")
	}
}

func TestServiceConnectHostKeyChangeConfirmRotates(t *testing.T) {
	pemBytes, public := encryptedKeyPEM(t, "phrase")
	port := sshKeyAuthServer(t, public)
	service, capture, hosts := newInteractiveService(t)

	// Pin a different key first so the handshake reads as a rotation. The
	// host-key callback receives the dial address ("host:port") as hostname.
	if err := hosts.Add(net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port))), "ssh-ed25519", "QUFBQU9MRA=="); err != nil {
		t.Fatal(err)
	}

	request := SSHConnectRequest{
		Hostname:   "127.0.0.1",
		Port:       port,
		Username:   "user",
		PrivateKey: string(pemBytes),
		SessionID:  "term-2",
		BootEpoch:  3,
	}
	go answerInteractive(service, capture, func(terminalssh.PassphraseRequest) (string, bool) {
		return "phrase", true
	}, func(terminalssh.HostKeyVerificationRequest) (bool, bool, bool) {
		return true, true, true
	})
	sessionID, err := service.Connect(request)
	if err != nil {
		t.Fatalf("connect with confirmed rotation: %v", err)
	}
	_ = service.Close(sessionID)
	if len(capture.hostKeys) != 1 {
		t.Fatalf("host key events: %+v", capture.hostKeys)
	}
	event := capture.hostKeys[0]
	if event.Status != "changed" || event.Hostname != "127.0.0.1" || event.Port != port || event.SessionID != "term-2" || event.BootEpoch != 3 {
		t.Fatalf("host key payload: %+v", event)
	}
	if event.Fingerprint == "" || event.KnownFingerprint == "" {
		t.Fatalf("fingerprints missing: %+v", event)
	}
	if !strings.Contains(event.Fingerprint, "SHA256:") {
		t.Fatalf("fingerprint format %q", event.Fingerprint)
	}
	// The stored key must now be the presented one (keyed by dial address).
	stored, _ := hosts.Lookup(net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port))), "ssh-ed25519")
	if stored == "QUFBQU9MRA==" || stored == "" {
		t.Fatalf("rotation not stored: %q", stored)
	}
}

func TestServiceConnectHostKeyRejectFails(t *testing.T) {
	pemBytes, public := encryptedKeyPEM(t, "phrase")
	port := sshKeyAuthServer(t, public)
	service, capture, hosts := newInteractiveService(t)
	if err := hosts.Add(net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port))), "ssh-ed25519", "QUFBQU9MRA=="); err != nil {
		t.Fatal(err)
	}

	go answerInteractive(service, capture, func(terminalssh.PassphraseRequest) (string, bool) {
		return "phrase", true
	}, func(terminalssh.HostKeyVerificationRequest) (bool, bool, bool) {
		return false, false, true
	})
	if _, err := service.Connect(SSHConnectRequest{Hostname: "127.0.0.1", Port: port, Username: "user", PrivateKey: string(pemBytes)}); err == nil {
		t.Fatal("rejected rotation must fail the dial")
	}
	if stored, _ := hosts.Lookup(net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port))), "ssh-ed25519"); stored != "QUFBQU9MRA==" {
		t.Fatalf("rejection must keep the pinned key: %q", stored)
	}
}

func TestServiceExecRunsCommandAndReportsStatus(t *testing.T) {
	for _, code := range []uint32{0, 3} {
		port := execServer(t, nil, "pw", code)
		service, _, _ := newInteractiveService(t)
		result := service.Exec(ExecRequest{
			SSHConnectRequest: SSHConnectRequest{Hostname: "127.0.0.1", Port: port, Username: "root", Password: "pw", SessionID: "export-key:x"},
			Command:           "mkdir -p ~/.ssh",
		})
		if result.Code == nil || *result.Code != int(code) {
			t.Fatalf("exit code: %+v", result)
		}
		if !strings.Contains(result.Stdout, "stdout-line") || !strings.Contains(result.Stderr, "stderr-line") {
			t.Fatalf("streams: %+v", result)
		}
	}
}

func TestServiceExecPromptsPassphrase(t *testing.T) {
	pemBytes, public := encryptedKeyPEM(t, "phrase")
	port := execServer(t, public, "", 0)
	service, capture, _ := newInteractiveService(t)
	go answerInteractive(service, capture, func(terminalssh.PassphraseRequest) (string, bool) {
		return "phrase", true
	}, nil)
	result := service.Exec(ExecRequest{
		SSHConnectRequest: SSHConnectRequest{Hostname: "127.0.0.1", Port: port, Username: "user", PrivateKey: string(pemBytes)},
		Command:           "true",
	})
	if result.Code == nil || *result.Code != 0 {
		t.Fatalf("exec after prompt: %+v", result)
	}
	if len(capture.passphrases) != 1 {
		t.Fatalf("passphrase events: %+v", capture.passphrases)
	}
}

func TestServiceExecRejectsEmptyCommand(t *testing.T) {
	service, _, _ := newInteractiveService(t)
	result := service.Exec(ExecRequest{Command: ""})
	if result.Code != nil || result.Stderr == "" {
		t.Fatalf("empty command: %+v", result)
	}
}

func TestExecRequestJSONRoundTrip(t *testing.T) {
	// The renderer sends flat camelCase options; the embedded dial contract
	// must keep its JSON names.
	encoded, err := json.Marshal(ExecRequest{
		SSHConnectRequest: SSHConnectRequest{Hostname: "h", Username: "u", SessionID: "s", BootEpoch: 2, KeyPath: "k"},
		Command:           "cmd",
		TimeoutMs:         1500,
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if !strings.Contains(text, `"command":"cmd"`) || !strings.Contains(text, `"timeoutMs":1500`) {
		t.Fatalf("encoded: %s", text)
	}
}
