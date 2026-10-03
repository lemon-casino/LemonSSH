package main

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	gossh "golang.org/x/crypto/ssh"
)

type KeyPairOptions struct {
	Type    string `json:"type"`
	Bits    int    `json:"bits,omitempty"`
	Comment string `json:"comment,omitempty"`
}

type KeyPairResult struct {
	Success    bool   `json:"success"`
	PrivateKey string `json:"privateKey,omitempty"`
	PublicKey  string `json:"publicKey,omitempty"`
	Error      string `json:"error,omitempty"`
}

type SSHAgentOptions struct {
	IdentityAgent   string `json:"identityAgent,omitempty"`
	AgentForwarding bool   `json:"agentForwarding,omitempty"`
	Hostname        string `json:"hostname,omitempty"`
	Port            int    `json:"port,omitempty"`
	Username        string `json:"username,omitempty"`
}

type SSHAgentStatus struct {
	Running     bool    `json:"running"`
	StartupType *string `json:"startupType"`
	Error       *string `json:"error"`
}

type DefaultSSHKey struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func encodeGeneratedKey(private any, comment string) (KeyPairResult, error) {
	block, err := gossh.MarshalPrivateKey(private, comment)
	if err != nil {
		return KeyPairResult{}, err
	}
	public, err := gossh.NewPublicKey(publicKeyOf(private))
	if err != nil {
		return KeyPairResult{}, err
	}
	publicText := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(public)))
	if strings.TrimSpace(comment) != "" {
		publicText += " " + strings.ReplaceAll(strings.TrimSpace(comment), "\n", " ")
	}
	return KeyPairResult{Success: true, PrivateKey: string(pem.EncodeToMemory(block)), PublicKey: publicText}, nil
}

func publicKeyOf(private any) any {
	switch key := private.(type) {
	case *rsa.PrivateKey:
		return &key.PublicKey
	case *ecdsa.PrivateKey:
		return &key.PublicKey
	case ed25519.PrivateKey:
		return key.Public()
	default:
		return nil
	}
}

func (s *TerminalService) GenerateKeyPair(options KeyPairOptions) KeyPairResult {
	keyType := strings.ToUpper(strings.TrimSpace(options.Type))
	var private any
	var err error
	switch keyType {
	case "RSA":
		bits := options.Bits
		if bits == 0 {
			bits = 3072
		}
		if bits != 2048 && bits != 3072 && bits != 4096 {
			return KeyPairResult{Error: "RSA bits must be 2048, 3072, or 4096"}
		}
		private, err = rsa.GenerateKey(rand.Reader, bits)
	case "ECDSA":
		bits := options.Bits
		if bits == 0 {
			bits = 256
		}
		var curve elliptic.Curve
		switch bits {
		case 256:
			curve = elliptic.P256()
		case 384:
			curve = elliptic.P384()
		case 521:
			curve = elliptic.P521()
		default:
			return KeyPairResult{Error: "ECDSA bits must be 256, 384, or 521"}
		}
		private, err = ecdsa.GenerateKey(curve, rand.Reader)
	case "ED25519":
		_, private, err = ed25519.GenerateKey(rand.Reader)
	default:
		return KeyPairResult{Error: "unsupported key type"}
	}
	if err != nil {
		return KeyPairResult{Error: err.Error()}
	}
	result, err := encodeGeneratedKey(private, options.Comment)
	if err != nil {
		return KeyPairResult{Error: err.Error()}
	}
	return result
}

func stringPointer(value string) *string { return &value }

func (s *TerminalService) CheckSshAgent(options SSHAgentOptions) SSHAgentStatus {
	identityAgent := strings.TrimSpace(options.IdentityAgent)
	if identityAgent == "" {
		identityAgent = strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK"))
	}
	if identityAgent != "" {
		if runtime.GOOS == "windows" && strings.HasPrefix(identityAgent, `\\.\pipe\`) {
			return SSHAgentStatus{Running: true, StartupType: stringPointer("named-pipe")}
		}
		if info, err := os.Stat(identityAgent); err == nil && !info.IsDir() {
			return SSHAgentStatus{Running: true, StartupType: stringPointer("socket")}
		}
	}
	if runtime.GOOS != "windows" {
		message := "SSH_AUTH_SOCK is not available"
		return SSHAgentStatus{Error: &message}
	}
	output, err := exec.Command("sc.exe", "query", "ssh-agent").CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return SSHAgentStatus{Error: &message}
	}
	running := strings.Contains(strings.ToUpper(string(output)), "RUNNING")
	startup := "unknown"
	if config, configErr := exec.Command("sc.exe", "qc", "ssh-agent").CombinedOutput(); configErr == nil {
		upper := strings.ToUpper(string(config))
		switch {
		case strings.Contains(upper, "AUTO_START"):
			startup = "automatic"
		case strings.Contains(upper, "DEMAND_START"):
			startup = "manual"
		case strings.Contains(upper, "DISABLED"):
			startup = "disabled"
		}
	}
	return SSHAgentStatus{Running: running, StartupType: &startup}
}

func (s *TerminalService) GetDefaultKeys() []DefaultSSHKey {
	home, err := os.UserHomeDir()
	if err != nil {
		return []DefaultSSHKey{}
	}
	sshDirectory := filepath.Join(home, ".ssh")
	keyNames := []string{"id_ed25519", "id_ecdsa", "id_rsa", "id_dsa"}
	result := make([]DefaultSSHKey, 0, len(keyNames))
	for _, name := range keyNames {
		path := filepath.Join(sshDirectory, name)
		info, statErr := os.Stat(path)
		if statErr == nil && info.Mode().IsRegular() {
			result = append(result, DefaultSSHKey{Name: fmt.Sprintf("%s (%s)", strings.TrimPrefix(name, "id_"), path), Path: path})
		}
	}
	return result
}
