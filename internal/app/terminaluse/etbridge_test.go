package terminaluse

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/platform/filesystem"
	"github.com/binaricat/netcatty/internal/terminal/dataplane"
	terminalssh "github.com/binaricat/netcatty/internal/terminal/ssh"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type etBootstrapObservation struct{ command, input string }

func etRemoteFixture(t *testing.T) (*terminalssh.Transport, <-chan etBootstrapObservation, <-chan uint32) {
	t.Helper()
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(private)
	config := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if string(password) != "remote-password" {
			return nil, errors.New("denied")
		}
		return nil, nil
	}}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	observed := make(chan etBootstrapObservation, 5)
	ports := make(chan uint32, 5)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		server, channels, requests, err := ssh.NewServerConn(conn, config)
		if err != nil {
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(requests)
		var wg sync.WaitGroup
		defer wg.Wait()
		for channel := range channels {
			stream, requests, err := channel.Accept()
			if err != nil {
				continue
			}
			wg.Add(1)
			go func(channel ssh.NewChannel) {
				defer wg.Done()
				defer stream.Close()
				if channel.ChannelType() == "direct-tcpip" {
					var destination struct {
						Host       string
						Port       uint32
						Origin     string
						OriginPort uint32
					}
					ssh.Unmarshal(channel.ExtraData(), &destination)
					ports <- destination.Port
					go ssh.DiscardRequests(requests)
					io.Copy(stream, stream)
					return
				}
				for request := range requests {
					if request.Type != "exec" {
						request.Reply(false, nil)
						continue
					}
					var command struct{ Value string }
					ssh.Unmarshal(request.Payload, &command)
					request.Reply(true, nil)
					input, _ := io.ReadAll(stream)
					observed <- etBootstrapObservation{command.Value, string(input)}
					io.WriteString(stream, "IDPASSKEY:"+strings.Repeat("I", 16)+"/"+strings.Repeat("K", 32)+"\n")
					stream.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
					return
				}
			}(channel)
		}
	}()
	client, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{User: "remote-user", Auth: []ssh.AuthMethod{ssh.Password("remote-password")}, HostKeyCallback: ssh.FixedHostKey(signer.PublicKey())})
	if err != nil {
		t.Fatal(err)
	}
	transport := &terminalssh.Transport{Client: client}
	t.Cleanup(func() {
		transport.Close()
		listener.Close()
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
			t.Error("remote fixture did not close")
		}
	})
	return transport, observed, ports
}

func TestETGoBootstrapNativeSSHandoffAndTunnel(t *testing.T) {
	transport, observed, ports := etRemoteFixture(t)
	temp, err := filesystem.NewTempService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := newEtBridge(context.Background(), transport, MoshStartRequest{ServerPath: "/opt/et tools/etterminal", ServerFifo: "/run/custom fifo", EtPort: 4044}, temp, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()
	observation := <-observed
	if observation.command != "'/opt/et tools/etterminal' --serverfifo='/run/custom fifo'" || !strings.HasSuffix(observation.input, "_xterm-256color\n") {
		t.Fatalf("incorrect remote bootstrap command %q", observation.command)
	}
	args := strings.Join(bridge.args, " ")
	for _, secret := range []string{"remote-password", strings.Repeat("K", 32), observation.input[:16]} {
		if strings.Contains(args, secret) {
			t.Fatal("remote secret in argv")
		}
	}
	identityBytes, err := os.ReadFile(filepath.Join(bridge.directory, "identity"))
	if err != nil {
		t.Fatal(err)
	}
	identity, err := ssh.ParsePrivateKey(identityBytes)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := knownhosts.New(filepath.Join(bridge.directory, "known_hosts"))
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ClientConfig{User: "netcatty", Auth: []ssh.AuthMethod{ssh.Password("unauthorized")}, HostKeyCallback: policy, Timeout: time.Second}
	if client, err := ssh.Dial("tcp", bridge.bootstrap.Addr().String(), config); err == nil {
		client.Close()
		t.Fatal("bootstrap allowed wrong identity")
	}
	// Exercise the exact -o values consumed by native ET against real OpenSSH.
	if sshPath, findErr := exec.LookPath("ssh"); findErr == nil {
		var sshArgs []string
		for i := 0; i+1 < len(bridge.args); i++ {
			if bridge.args[i] == "--ssh-option" {
				sshArgs = append(sshArgs, "-o", bridge.args[i+1])
				i++
			}
		}
		sshArgs = append(sshArgs, "netcatty@127.0.0.1", "ignored-bootstrap-command")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, sshPath, sshArgs...)
		cmd.Dir = bridge.directory
		cmd.Env = helperEnvironment(bridge.env)
		response, err := cmd.CombinedOutput()
		expected := "IDPASSKEY:" + strings.Repeat("I", 16) + "/" + strings.Repeat("K", 32)
		if !strings.Contains(string(response), expected) {
			t.Fatalf("native SSH handoff failed: %v %s", err, response)
		}
	} else {
		config.Auth = []ssh.AuthMethod{ssh.PublicKeys(identity)}
		client, err := ssh.Dial("tcp", bridge.bootstrap.Addr().String(), config)
		if err != nil {
			t.Fatal(err)
		}
		session, err := client.NewSession()
		if err != nil {
			t.Fatal(err)
		}
		response, err := session.Output("bootstrap")
		session.Close()
		client.Close()
		if err != nil || !strings.HasPrefix(string(response), "IDPASSKEY:") {
			t.Fatal("Go SSH handoff failed", err)
		}
		t.Log("OpenSSH unavailable; only Go SSH handoff exercised")
	}
	config.Auth = []ssh.AuthMethod{ssh.PublicKeys(identity)}
	client, err := ssh.Dial("tcp", bridge.bootstrap.Addr().String(), config)
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = session.Output("second"); err == nil {
		t.Fatal("bootstrap reply consumed twice")
	}
	session.Close()
	client.Close()
	conn, err := net.Dial("tcp", bridge.tcp.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	conn.SetDeadline(time.Now().Add(time.Second))
	conn.Write([]byte("hello"))
	buf := make([]byte, 5)
	if _, err = io.ReadFull(conn, buf); err != nil || string(buf) != "hello" {
		t.Fatal("ET tunnel failed", err)
	}
	if <-ports != 4044 {
		t.Fatal("wrong ET port")
	}
	conn.Close()
	bridge.Close()
	if _, err = os.Stat(bridge.directory); !os.IsNotExist(err) {
		t.Fatal("temporary identity was not removed")
	}
	if conn, err = net.DialTimeout("tcp", bridge.tcp.Addr().String(), time.Second); err == nil {
		conn.Close()
		t.Fatal("tunnel survived close")
	}
}

func TestETAuthMappingAndNativeFallback(t *testing.T) {
	var request MoshStartRequest
	if err := json.Unmarshal([]byte(`{"hostname":"target","username":"user","password":"pw","privateKey":"pem","passphrase":"phrase","certificate":"cert","useAgent":true,"enableMfa":true,"identityFilePaths":["/key"],"proxyUrl":"socks5://p:1080","proxyCommand":"connect %h %p","etPort":4044,"sessionId":"s","bootEpoch":9,"jumpHosts":[{"hostname":"jump","username":"hop","password":"hop-pw","certificate":"hop-cert","useAgent":true}]}`), &request); err != nil {
		t.Fatal(err)
	}
	input := sshConnectToInput(request.SSHConnectRequest)
	if input.Password != "pw" || input.PrivateKey != "pem" || input.Passphrase != "phrase" || input.Certificate != "cert" || !input.UseAgent || !input.EnableMFA || input.ProxyURL != "socks5://p:1080" || input.ProxyCommand != "connect %h %p" || len(input.JumpHosts) != 1 || input.JumpHosts[0].Password != "hop-pw" || input.JumpHosts[0].Certificate != "hop-cert" || !input.JumpHosts[0].UseAgent || request.EtPort != 4044 || request.BootEpoch != 9 {
		t.Fatal("bridge auth input lost")
	}
	for _, field := range []string{"password", "privateKey", "passphrase", "certificate", "proxyUrl", "proxyCommand", "identityFilePaths", "useAgent", "enableMfa", "jumpHosts"} {
		value := "\"configured\""
		switch field {
		case "identityFilePaths":
			value = `["/key"]`
		case "jumpHosts":
			value = `[{"hostname":"hop"}]`
		case "useAgent", "enableMfa":
			value = "true"
		}
		var configured MoshStartRequest
		json.Unmarshal([]byte(`{"`+field+`":`+value+`}`), &configured)
		if !etUsesGoSSH(configured) {
			t.Errorf("%s silently fell back to native SSH", field)
		}
	}
	if etUsesGoSSH(MoshStartRequest{SSHConnectRequest: SSHConnectRequest{Hostname: "ssh-config-alias", Username: "user"}}) {
		t.Fatal("native SSH config fallback lost")
	}
	controller := dataplane.NewRouteController()
	service := New(controller, dataplane.NewServer(controller, ""), terminalssh.NewKnownHosts(filepath.Join(t.TempDir(), "known_hosts")))
	no := false
	config, err := sshBootstrapConfig(context.Background(), service, MoshStartRequest{SSHConnectRequest: SSHConnectRequest{Hostname: "target", Username: "user", Password: "pw", VerifyHostKeys: &no, JumpHosts: []SSHConnectRequest{{Hostname: "jump", Username: "hop", Password: "pw", VerifyHostKeys: &no}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(key)
	_, changedKey, _ := ed25519.GenerateKey(rand.Reader)
	changed, _ := ssh.NewSignerFromKey(changedKey)
	for _, hop := range []terminalssh.DialConfig{config, config.JumpHosts[0]} {
		if err = hop.HostKeyPolicy(hop.Hostname, &net.TCPAddr{}, signer.PublicKey()); err != nil {
			t.Fatal(err)
		}
		if err = hop.HostKeyPolicy(hop.Hostname, &net.TCPAddr{}, changed.PublicKey()); err == nil {
			t.Fatal("bootstrap allowed changed host key")
		}
	}
}

func TestETTunnelRedialDoesNotBootstrapAnotherShell(t *testing.T) {
	first, firstBootstrap, _ := etRemoteFixture(t)
	second, secondBootstrap, secondPorts := etRemoteFixture(t)
	temp, err := filesystem.NewTempService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	redials := 0
	bridge, err := newEtBridge(context.Background(), first, MoshStartRequest{EtPort: 4045}, temp,
		func(context.Context) (*terminalssh.Transport, error) { redials++; return second, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()
	<-firstBootstrap
	first.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := bridge.dialRemote(ctx, "127.0.0.1:4045")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Write([]byte("same-native-client"))
	buf := make([]byte, 18)
	if _, err = io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if redials != 1 || <-secondPorts != 4045 || len(secondBootstrap) != 0 {
		t.Fatal("tunnel recovery changed bootstrap")
	}
}
