package ssh

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrPassphraseUnknown   = errors.New("passphrase request not found")
	ErrPassphraseCancelled = errors.New("passphrase prompt cancelled")
	ErrPassphraseTimeout   = errors.New("passphrase prompt timed out")
)

// PassphraseRejectedError reports that every prompted passphrase failed to
// decrypt the key. The shell facade surfaces it so the renderer can clear
// remembered passphrases for KeyPath.
type PassphraseRejectedError struct {
	KeyPath string
	Err     error
}

func (e *PassphraseRejectedError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("passphrase rejected for %s", e.KeyPath)
	}
	return fmt.Sprintf("passphrase rejected for %s: %v", e.KeyPath, e.Err)
}

func (e *PassphraseRejectedError) Unwrap() error { return e.Err }

// Renderer event names emitted alongside passphrase prompts. The shell facade
// maps them onto the renderer bridge contract (onPassphraseTimeout etc.).
const (
	EventPassphraseTimeout   = "ssh:passphrase-timeout"
	EventPassphraseCancelled = "ssh:passphrase-cancelled"
	EventPassphraseRejected  = "ssh:passphrase-auth-failed"
)

// PassphraseRequest is one encrypted-key passphrase prompt surfaced to the
// renderer. SessionID/BootEpoch correlate the prompt with the terminal boot
// that asked for it so the renderer can drop stale prompts.
type PassphraseRequest struct {
	RequestID         string `json:"requestId"`
	KeyPath           string `json:"keyPath"`
	KeyName           string `json:"keyName"`
	Hostname          string `json:"hostname,omitempty"`
	SessionID         string `json:"sessionId,omitempty"`
	BootEpoch         int    `json:"bootEpoch,omitempty"`
	PassphraseInvalid bool   `json:"passphraseInvalid,omitempty"`
}

type passphraseReply struct {
	passphrase string
	cancelled  bool
}

// PassphrasePrompt asks the renderer for an encrypted key's passphrase.
// hostname identifies the hop being dialed, keyPath the key file (or caller
// key label), and passphraseInvalid reports that a previously supplied
// passphrase failed to decrypt the key.
type PassphrasePrompt func(hostname, keyPath string, passphraseInvalid bool) (string, error)

// PassphraseBroker correlates one in-flight passphrase prompt with a renderer
// response, mirroring InteractiveBroker for encrypted private keys. Prompts
// time out fail-closed.
type PassphraseBroker struct {
	emit    func(PassphraseRequest)
	notify  func(name string, payload any)
	timeout time.Duration
	seq     atomic.Uint64
	mu      sync.Mutex
	pending map[string]chan passphraseReply
}

// NewPassphraseBroker wires the prompt emitter plus a notifier for
// timeout/cancelled/rejected events (both may be nil in tests).
func NewPassphraseBroker(emit func(PassphraseRequest), notify func(name string, payload any), timeout time.Duration) *PassphraseBroker {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return &PassphraseBroker{
		emit:    emit,
		notify:  notify,
		timeout: timeout,
		pending: make(map[string]chan passphraseReply),
	}
}

// Requester binds the renderer session correlation (alias session id and boot
// epoch) carried by the connection request; hostname varies per hop.
func (b *PassphraseBroker) Requester(sessionID string, bootEpoch int) PassphrasePrompt {
	return func(hostname, keyPath string, passphraseInvalid bool) (string, error) {
		return b.request(sessionID, bootEpoch, hostname, keyPath, passphraseInvalid)
	}
}

func (b *PassphraseBroker) request(sessionID string, bootEpoch int, hostname, keyPath string, passphraseInvalid bool) (string, error) {
	requestID := fmt.Sprintf("pp-%d", b.seq.Add(1))
	reply := make(chan passphraseReply, 1)
	b.mu.Lock()
	b.pending[requestID] = reply
	b.mu.Unlock()
	defer func() { b.mu.Lock(); delete(b.pending, requestID); b.mu.Unlock() }()
	if b.emit != nil {
		b.emit(PassphraseRequest{
			RequestID:         requestID,
			KeyPath:           keyPath,
			KeyName:           keyNameFor(keyPath),
			Hostname:          hostname,
			SessionID:         sessionID,
			BootEpoch:         bootEpoch,
			PassphraseInvalid: passphraseInvalid,
		})
	}
	timer := time.NewTimer(b.timeout)
	defer timer.Stop()
	select {
	case result := <-reply:
		if result.cancelled {
			return "", ErrPassphraseCancelled
		}
		return result.passphrase, nil
	case <-timer.C:
		b.mu.Lock()
		delete(b.pending, requestID)
		b.mu.Unlock()
		if b.notify != nil {
			b.notify(EventPassphraseTimeout, map[string]any{"requestId": requestID})
		}
		return "", ErrPassphraseTimeout
	}
}

// Respond completes or cancels a pending passphrase prompt.
func (b *PassphraseBroker) Respond(requestID, passphrase string, cancelled bool) error {
	b.mu.Lock()
	reply, ok := b.pending[requestID]
	if ok {
		delete(b.pending, requestID)
	}
	b.mu.Unlock()
	if !ok {
		return ErrPassphraseUnknown
	}
	reply <- passphraseReply{passphrase: passphrase, cancelled: cancelled}
	if cancelled && b.notify != nil {
		b.notify(EventPassphraseCancelled, map[string]any{"requestId": requestID})
	}
	return nil
}

// PendingRequestID is a test helper for the in-flight prompt.
func (b *PassphraseBroker) PendingRequestID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id := range b.pending {
		return id
	}
	return ""
}

// keyNameFor renders the short display name shown in the passphrase modal.
func keyNameFor(keyPath string) string {
	trimmed := strings.TrimRight(strings.ReplaceAll(keyPath, "\\", "/"), "/")
	if trimmed == "" {
		return "SSH key"
	}
	if index := strings.LastIndex(trimmed, "/"); index >= 0 {
		trimmed = trimmed[index+1:]
	}
	if trimmed == "" {
		return "SSH key"
	}
	return trimmed
}
