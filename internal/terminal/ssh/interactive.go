package ssh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrInteractiveUnknown   = errors.New("keyboard-interactive request not found")
	ErrInteractiveCancelled = errors.New("keyboard-interactive cancelled")
	ErrInteractiveTimeout   = errors.New("keyboard-interactive timed out")
	ErrUnsupportedProxy     = errors.New("unsupported ssh proxy type")
)

// KeyboardPrompt is one keyboard-interactive challenge line.
type KeyboardPrompt struct {
	Prompt string `json:"prompt"`
	Echo   bool   `json:"echo"`
}

// KeyboardChallenge is one MFA prompt set surfaced to the renderer.
type KeyboardChallenge struct {
	RequestID    string           `json:"requestId"`
	Hostname     string           `json:"hostname"`
	Name         string           `json:"name"`
	Instructions string           `json:"instructions"`
	Prompts      []KeyboardPrompt `json:"prompts"`
}

type interactiveReply struct {
	answers   []string
	cancelled bool
}

// InteractiveBroker correlates one in-flight keyboard-interactive challenge
// with a renderer response. Challenges time out fail-closed.
type InteractiveBroker struct {
	emit    func(KeyboardChallenge)
	timeout time.Duration
	seq     atomic.Uint64
	mu      sync.Mutex
	pending map[string]chan interactiveReply
}

func NewInteractiveBroker(emit func(KeyboardChallenge), timeout time.Duration) *InteractiveBroker {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return &InteractiveBroker{
		emit:    emit,
		timeout: timeout,
		pending: make(map[string]chan interactiveReply),
	}
}

// Handler returns an x/crypto keyboard-interactive callback bound to hostname.
func (b *InteractiveBroker) Handler(hostname string) func(name, instruction string, questions []string, echoes []bool) ([]string, error) {
	return b.HandlerContext(context.Background(), hostname)
}

func (b *InteractiveBroker) HandlerContext(ctx context.Context, hostname string) func(name, instruction string, questions []string, echoes []bool) ([]string, error) {
	return func(name, instruction string, questions []string, echoes []bool) ([]string, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		prompts := make([]KeyboardPrompt, len(questions))
		for index, question := range questions {
			echo := false
			if index < len(echoes) {
				echo = echoes[index]
			}
			prompts[index] = KeyboardPrompt{Prompt: question, Echo: echo}
		}
		requestID := fmt.Sprintf("kbd-%d", b.seq.Add(1))
		reply := make(chan interactiveReply, 1)
		b.mu.Lock()
		b.pending[requestID] = reply
		b.mu.Unlock()
		defer func() { b.mu.Lock(); delete(b.pending, requestID); b.mu.Unlock() }()
		if b.emit != nil {
			b.emit(KeyboardChallenge{
				RequestID:    requestID,
				Hostname:     hostname,
				Name:         name,
				Instructions: instruction,
				Prompts:      prompts,
			})
		}
		timer := time.NewTimer(b.timeout)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-reply:
			if result.cancelled {
				return nil, ErrInteractiveCancelled
			}
			return result.answers, nil
		case <-timer.C:
			b.mu.Lock()
			delete(b.pending, requestID)
			b.mu.Unlock()
			return nil, ErrInteractiveTimeout
		}
	}
}

// Respond completes or cancels a pending challenge.
func (b *InteractiveBroker) Respond(requestID string, responses []string, cancelled bool) error {
	b.mu.Lock()
	reply, ok := b.pending[requestID]
	if ok {
		delete(b.pending, requestID)
	}
	b.mu.Unlock()
	if !ok {
		return ErrInteractiveUnknown
	}
	reply <- interactiveReply{answers: responses, cancelled: cancelled}
	return nil
}

// PendingRequestID is a test helper for the in-flight challenge.
func (b *InteractiveBroker) PendingRequestID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id := range b.pending {
		return id
	}
	return ""
}

// ConnectInput is the shell-neutral dial request used by Wails facades.
type ConnectInput struct {
	Hostname   string
	Port       uint16
	Username   string
	Password   string
	PrivateKey string
	Passphrase string
	// KeyPath labels the private key in passphrase prompts (file path or the
	// caller's key identifier). Display only.
	KeyPath      string
	Certificate  string
	ProxyURL     string
	ProxyCommand string
	EnableMFA    bool
	UseAgent     bool
	// AgentForwarding marks the transport as exposing the local SSH agent to
	// this hop (OpenSSH ForwardAgent yes). DialConfig.ForwardAgent makes the
	// pool treat such transports as single-use.
	AgentForwarding   bool
	IdentityFilePaths []string
	JumpHosts         []ConnectInput
}

// DialInteractive bundles the renderer-backed callbacks one dial may need.
// Nil members keep the fail-closed defaults (no MFA prompts, no passphrase
// prompts, changed host keys rejected without confirmation).
type DialInteractive struct {
	// Challenge answers keyboard-interactive rounds (MFA). The factory binds
	// the hostname per hop so prompts carry the hop being dialed.
	Challenge func(hostname string) func(name, instruction string, questions []string, echoes []bool) ([]string, error)
	// Passphrase asks for an encrypted private key's passphrase.
	Passphrase PassphrasePrompt
	// ConfirmHostKey asks the renderer to accept or reject a changed key.
	ConfirmHostKey HostKeyConfirm
}

// FormatProxyURL builds a socks5:// or http:// URL. Command proxies fail closed.
func FormatProxyURL(kind, host string, port int, username, password string) (string, error) {
	switch kind {
	case "socks5", "http":
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedProxy, kind)
	}
	if host == "" || port <= 0 {
		return "", fmt.Errorf("%w: missing host or port", ErrUnsupportedProxy)
	}
	parsed := &url.URL{
		Scheme: kind,
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
	}
	if username != "" {
		parsed.User = url.UserPassword(username, password)
	}
	return parsed.String(), nil
}

// BuildDialConfig maps a ConnectInput onto DialConfig. Every hop receives the
// same host-key policy; renderer callbacks ride DialInteractive per hop.
func BuildDialConfig(input ConnectInput, policy HostKeyPolicy, interactive DialInteractive) DialConfig {
	config, err := BuildDialConfigErr(input, policy, interactive)
	if err != nil {
		return DialConfig{}
	}
	return config
}

func BuildDialConfigErr(input ConnectInput, policy HostKeyPolicy, interactive DialInteractive) (DialConfig, error) {
	privateKey := []byte(input.PrivateKey)
	if len(privateKey) == 0 && len(input.IdentityFilePaths) > 0 {
		loaded, err := LoadIdentityFilePEMs(input.IdentityFilePaths)
		if err != nil {
			return DialConfig{}, err
		}
		privateKey = loaded
	}
	auth := AuthMethod{
		Password:      input.Password,
		PrivateKeyPEM: privateKey,
		Passphrase:    input.Passphrase,
		KeyPath:       resolveKeyPath(input),
		UseAgent:      input.UseAgent,
		Certificate:   []byte(input.Certificate),
	}
	if interactive.Passphrase != nil && len(privateKey) > 0 {
		passphrase := interactive.Passphrase
		auth.RequestPassphrase = func(keyPath string, passphraseInvalid bool) (string, error) {
			return passphrase(input.Hostname, keyPath, passphraseInvalid)
		}
	}
	if input.EnableMFA && interactive.Challenge != nil {
		auth.Challenge = interactive.Challenge(input.Hostname)
	}
	config := DialConfig{
		Hostname:          input.Hostname,
		Port:              input.Port,
		Username:          input.Username,
		Auth:              auth,
		HostKeyPolicy:     policy,
		Timeout:           15 * time.Second,
		HandshakeTimeout:  15 * time.Second,
		KeepaliveInterval: 30 * time.Second,
		ProxyURL:          input.ProxyURL,
		ProxyCommand:      input.ProxyCommand,
		ForwardAgent:      input.AgentForwarding,
	}
	if len(input.JumpHosts) > 0 {
		config.JumpHosts = make([]DialConfig, 0, len(input.JumpHosts))
		for _, hop := range input.JumpHosts {
			hopInteractive := interactive
			if !hop.EnableMFA {
				hopInteractive.Challenge = nil
			}
			child, err := BuildDialConfigErr(hop, policy, hopInteractive)
			if err != nil {
				return DialConfig{}, err
			}
			config.JumpHosts = append(config.JumpHosts, child)
		}
	}
	return config, nil
}

// resolveKeyPath picks the passphrase-prompt label: the caller's explicit key
// path, else the first identity file, else empty.
func resolveKeyPath(input ConnectInput) string {
	if strings.TrimSpace(input.KeyPath) != "" {
		return input.KeyPath
	}
	for _, path := range input.IdentityFilePaths {
		if strings.TrimSpace(path) != "" {
			return path
		}
	}
	return ""
}
