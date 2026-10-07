package terminaluse

import "testing"

func TestSSHConnectToInputMapsJumpAndMFA(t *testing.T) {
	input := sshConnectToInput(SSHConnectRequest{
		Hostname:  "target",
		Port:      22,
		Username:  "root",
		Password:  "pw",
		EnableMFA: true,
		ProxyURL:  "socks5://127.0.0.1:1080",
		JumpHosts: []SSHConnectRequest{{
			Hostname: "jump",
			Port:     2222,
			Username: "bastion",
			Password: "jpw",
		}},
	})
	if !input.EnableMFA || input.ProxyURL != "socks5://127.0.0.1:1080" {
		t.Fatalf("target input: %+v", input)
	}
	if len(input.JumpHosts) != 1 || input.JumpHosts[0].Hostname != "jump" || input.JumpHosts[0].Port != 2222 {
		t.Fatalf("jump: %+v", input.JumpHosts)
	}
}
