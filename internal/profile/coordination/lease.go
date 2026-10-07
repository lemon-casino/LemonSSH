// Package coordination implements the cross-shell profile writer lease
// (P2-03). At any moment at most one shell may write a profile.
//
// Protocol:
//   - Liveness is OS-native: the live holder keeps an exclusive flock on
//     `<profile>.writer.lock`. Process death releases the lock, so takeover
//     never relies on timeouts to detect a dead owner.
//   - Authority is a persistent epoch: `<profile>.writer.epoch` holds a
//     monotonically increasing fencing token bumped on every acquisition.
//     Writers embed the epoch into profile writes so a stale writer (e.g. a
//     partitioned shell that still holds a heartbeat) can be fenced.
//   - The lease file `<profile>.writer.lease` records holder, epoch and
//     expiry for diagnostics and for rejecting "expired but still live"
//     holders instead of silently stealing the lease.
package coordination

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

var (
	// ErrLeaseHeld marks a conflicting live lease.
	ErrLeaseHeld = errors.New("profile writer lease held by another owner")
	// ErrLeaseNotHeld marks lease operations without ownership.
	ErrLeaseNotHeld = errors.New("profile writer lease not held")
	// ErrLeaseExpired marks an owned lease whose TTL elapsed; renew first.
	ErrLeaseExpired = errors.New("profile writer lease expired")
)

// Lease describes the current lease state.
type Lease struct {
	HolderID    string `json:"holderId"`
	Epoch       uint64 `json:"epoch"`
	ExpiresAtMS int64  `json:"expiresAtMs"`
}

// Manager owns one profile's writer lease.
type Manager struct {
	profilePath string
	lockPath    string
	epochPath   string
	leasePath   string
	holderID    string

	flock *flock.Flock
	lease Lease
}

// NewManager builds a lease manager for a profile path.
func NewManager(profilePath, holderID string) *Manager {
	return &Manager{
		profilePath: profilePath,
		lockPath:    profilePath + ".writer.lock",
		epochPath:   profilePath + ".writer.epoch",
		leasePath:   profilePath + ".writer.lease",
		holderID:    holderID,
	}
}

// HolderID reports the manager's identity.
func (m *Manager) HolderID() string { return m.holderID }

// Acquire takes the writer lease for ttl. It fails with ErrLeaseHeld while
// another live process holds the flock, and otherwise bumps the persistent
// epoch (crash takeover included).
func (m *Manager) Acquire(ttl time.Duration) (Lease, error) {
	if m.flock != nil {
		return Lease{}, fmt.Errorf("already holding or acquiring a lease: %s", m.holderID)
	}
	if err := os.MkdirAll(filepath.Dir(m.lockPath), 0o700); err != nil {
		return Lease{}, err
	}
	fileLock := flock.New(m.lockPath)
	locked, err := fileLock.TryLock()
	if err != nil {
		return Lease{}, err
	}
	if !locked {
		return Lease{}, ErrLeaseHeld
	}
	m.flock = fileLock

	epoch, err := m.bumpEpoch()
	if err != nil {
		m.abort()
		return Lease{}, err
	}
	m.lease = Lease{HolderID: m.holderID, Epoch: epoch, ExpiresAtMS: time.Now().Add(ttl).UnixMilli()}
	if err := m.writeLease(); err != nil {
		m.abort()
		return Lease{}, err
	}
	return m.lease, nil
}

// Renew extends the held lease's expiry without changing the epoch.
func (m *Manager) Renew(ttl time.Duration) (Lease, error) {
	if m.flock == nil {
		return Lease{}, ErrLeaseNotHeld
	}
	if time.Now().UnixMilli() > m.lease.ExpiresAtMS {
		return Lease{}, ErrLeaseExpired
	}
	m.lease.ExpiresAtMS = time.Now().Add(ttl).UnixMilli()
	if err := m.writeLease(); err != nil {
		return Lease{}, err
	}
	return m.lease, nil
}

// Release drops the lease and the flock. The epoch persists so fencing
// tokens remain monotonic across owners.
func (m *Manager) Release() error {
	if m.flock == nil {
		return ErrLeaseNotHeld
	}
	_ = os.Remove(m.leasePath)
	err := m.flock.Unlock()
	m.flock = nil
	return err
}

// Current reads the persisted lease, if any. It reports the file contents
// only; liveness is governed by the flock, not by this record.
func (m *Manager) Current() (Lease, bool) {
	data, err := os.ReadFile(m.leasePath)
	if err != nil {
		return Lease{}, false
	}
	var lease Lease
	if json.Unmarshal(data, &lease) != nil {
		return Lease{}, false
	}
	return lease, true
}

func (m *Manager) bumpEpoch() (uint64, error) {
	epoch := uint64(0)
	if data, err := os.ReadFile(m.epochPath); err == nil {
		if _, parseErr := fmt.Sscanf(string(data), "%d", &epoch); parseErr != nil {
			return 0, fmt.Errorf("corrupt epoch file: %w", parseErr)
		}
	} else if !os.IsNotExist(err) {
		return 0, err
	}
	epoch++
	if err := os.WriteFile(m.epochPath, []byte(fmt.Sprintf("%d", epoch)), 0o600); err != nil {
		return 0, err
	}
	return epoch, nil
}

func (m *Manager) writeLease() error {
	data, err := json.Marshal(m.lease)
	if err != nil {
		return err
	}
	return os.WriteFile(m.leasePath, data, 0o600)
}

func (m *Manager) abort() {
	if m.flock != nil {
		_ = m.flock.Unlock()
		m.flock = nil
	}
}
