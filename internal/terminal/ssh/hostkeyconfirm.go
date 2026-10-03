package ssh

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// EventHostKeyVerification is the renderer event name for a changed host-key
// confirmation prompt.
const EventHostKeyVerification = "ssh:host-key-verification"

var ErrHostKeyConfirmUnavailable = errors.New("host key verification is unavailable")

// HostKeyVerificationRequest is the renderer-facing changed-host-key prompt.
// It carries both the presented and the previously pinned fingerprints so the
// dialog can show the rotation.
type HostKeyVerificationRequest struct {
	RequestID        string `json:"requestId"`
	SessionID        string `json:"sessionId,omitempty"`
	BootEpoch        int    `json:"bootEpoch,omitempty"`
	Hostname         string `json:"hostname"`
	Port             uint16 `json:"port"`
	Status           string `json:"status"`
	KeyType          string `json:"keyType"`
	Fingerprint      string `json:"fingerprint"`
	PublicKey        string `json:"publicKey,omitempty"`
	KnownFingerprint string `json:"knownFingerprint,omitempty"`
}

type hostKeyReply struct {
	accept bool
	store  bool
}

// HostKeyBroker correlates one in-flight changed host-key confirmation with a
// renderer response, mirroring InteractiveBroker. Confirmations time out
// fail-closed so a hung renderer cannot stall the dial forever.
type HostKeyBroker struct {
	emit    func(HostKeyVerificationRequest)
	timeout time.Duration
	seq     atomic.Uint64
	mu      sync.Mutex
	pending map[string]chan hostKeyReply
}

// NewHostKeyBroker wires the prompt emitter (may be nil in tests; a nil
// emitter makes every confirmation fail fast instead of stalling).
func NewHostKeyBroker(emit func(HostKeyVerificationRequest), timeout time.Duration) *HostKeyBroker {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return &HostKeyBroker{
		emit:    emit,
		timeout: timeout,
		pending: make(map[string]chan hostKeyReply),
	}
}

// Confirmer returns the HostKeyConfirm bound to the renderer session
// correlation carried by the connection request.
func (b *HostKeyBroker) Confirmer(sessionID string, bootEpoch int) HostKeyConfirm {
	return func(change HostKeyChange) (bool, bool, error) {
		if b.emit == nil {
			return false, false, ErrHostKeyConfirmUnavailable
		}
		requestID := fmt.Sprintf("hk-%d", b.seq.Add(1))
		reply := make(chan hostKeyReply, 1)
		b.mu.Lock()
		b.pending[requestID] = reply
		b.mu.Unlock()
		defer func() { b.mu.Lock(); delete(b.pending, requestID); b.mu.Unlock() }()
		b.emit(HostKeyVerificationRequest{
			RequestID:        requestID,
			SessionID:        sessionID,
			BootEpoch:        bootEpoch,
			Hostname:         change.Hostname,
			Port:             change.Port,
			Status:           "changed",
			KeyType:          change.KeyType,
			Fingerprint:      change.Fingerprint,
			PublicKey:        change.PublicKey,
			KnownFingerprint: change.KnownFingerprint,
		})
		timer := time.NewTimer(b.timeout)
		defer timer.Stop()
		select {
		case result := <-reply:
			return result.accept, result.store, nil
		case <-timer.C:
			b.mu.Lock()
			delete(b.pending, requestID)
			b.mu.Unlock()
			return false, false, ErrInteractiveTimeout
		}
	}
}

// Respond completes or rejects a pending changed host-key confirmation.
func (b *HostKeyBroker) Respond(requestID string, accept, store bool) error {
	b.mu.Lock()
	reply, ok := b.pending[requestID]
	if ok {
		delete(b.pending, requestID)
	}
	b.mu.Unlock()
	if !ok {
		return ErrInteractiveUnknown
	}
	reply <- hostKeyReply{accept: accept, store: store}
	return nil
}

// PendingRequestID is a test helper for the in-flight confirmation.
func (b *HostKeyBroker) PendingRequestID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id := range b.pending {
		return id
	}
	return ""
}
