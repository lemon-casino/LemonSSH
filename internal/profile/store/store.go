// Package store implements the Netcatty transactional profile store (P2-02).
//
// Engine: bbolt (pure Go, single-writer mmap B+tree, fully ACID file
// transactions with crash recovery via free-list rebuild). Chosen over
// SQLite/cgo and LMDB for cross-platform crash consistency without build
// toolchain requirements; go.etcd.io/bbolt v1.4.3.
//
// Invariants:
//   - every mutation runs inside one bbolt transaction (atomic, durable on
//     return);
//   - the store carries a monotonic revision; writers may use compare-and-swap
//     and lose with ErrRevisionConflict instead of clobbering;
//   - raw values are opaque bytes (localStorage compatibility); typed records
//     live in the meta bucket only;
//   - durable notifications fire after commit with the new revision.
package store

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// SchemaVersion is the profile store schema version.
const SchemaVersion = 1

// MaxRawValueBytes bounds one raw value (4 MiB) so a runaway renderer payload
// cannot bloat the profile file.
const MaxRawValueBytes = 4 << 20

// ErrValueTooLarge marks raw values beyond MaxRawValueBytes.
var ErrValueTooLarge = errors.New("profile value exceeds bound")

var (
	// ErrClosed is returned after Close.
	ErrClosed = errors.New("profile store is closed")
	// ErrRevisionConflict is returned by CAS writes with a stale revision.
	ErrRevisionConflict = errors.New("profile revision conflict")
	// ErrNoSuchKey marks a missing raw value.
	ErrNoSuchKey = errors.New("profile key not found")
	// ErrInvalidKey marks keys outside the allowed shape.
	ErrInvalidKey = errors.New("invalid profile key")
	// ErrStagingNotComplete marks promotion attempts on incomplete staging stores.
	ErrStagingNotComplete = errors.New("staging profile is not complete")
	// ErrUnknownDomain rejects domains that are not declared.
	ErrUnknownDomain = errors.New("unknown profile domain")
)

var (
	bucketMeta  = []byte("meta")
	bucketRaw   = []byte("raw")
	keySchema   = []byte("schema_version")
	keyRevision = []byte("revision")
)

// Store is the host-owned profile store. Safe for concurrent use.
type Store struct {
	mu     sync.RWMutex
	closed bool

	path     string
	revision uint64

	// notify is invoked after each committed mutation, on the writer's
	// goroutine, with the post-commit revision.
	notify func(revision uint64)
}

// Domain names bound the raw namespace. One bbolt sub-bucket per domain keeps
// domains isolated and makes P2-07 domain-by-domain migration explicit.
var Domains = []string{"settings", "vault", "sessions", "logs", "plugin-v1", "device"}

func validDomain(domain string) bool {
	for _, candidate := range Domains {
		if candidate == domain {
			return true
		}
	}
	return false
}

func validKey(key string) bool {
	if key == "" || len(key) > 256 {
		return false
	}
	for _, r := range key {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// Open opens (creating if needed) the profile store at path.
func Open(path string, notify func(revision uint64)) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if _, err := initBolt(path); err != nil {
		return nil, err
	}
	revision, err := initMeta(path)
	if err != nil {
		_ = closeBolt(path)
		return nil, err
	}
	return &Store{path: path, revision: revision, notify: notify}, nil
}

// Path reports the store file path.
func (s *Store) Path() string { return s.path }

// Revision returns the current committed revision.
func (s *Store) Revision() (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return 0, ErrClosed
	}
	return s.revision, nil
}

// GetRaw returns the raw value bytes for domain/key.
func (s *Store) GetRaw(domain, key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, ErrClosed
	}
	if !validDomain(domain) {
		return nil, fmt.Errorf("%w: %q", ErrUnknownDomain, domain)
	}
	if !validKey(key) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	return getRaw(s.path, domain, key)
}

// SetRaw writes one raw value inside a single transaction.
func (s *Store) SetRaw(domain, key string, value []byte) error {
	_, err := s.Write(WriteRequest{
		Mutations: []Mutation{{Domain: domain, Key: key, Value: value}},
	})
	return err
}

// DeleteRaw removes one raw value.
func (s *Store) DeleteRaw(domain, key string) error {
	_, err := s.Write(WriteRequest{
		Mutations: []Mutation{{Domain: domain, Key: key, Delete: true}},
	})
	return err
}

// Mutation is one key operation inside a transaction.
type Mutation struct {
	Domain string
	Key    string
	Value  []byte
	Delete bool
}

// WriteRequest describes one all-or-nothing transaction. ExpectedRevision
// enables compare-and-swap: 0 means "no CAS check".
type WriteRequest struct {
	ExpectedRevision uint64
	Mutations        []Mutation
}

// WriteResult reports the post-commit state.
type WriteResult struct {
	Revision uint64
}

// Write executes the transaction atomically and durably (bbolt fsyncs on
// commit). On any error the store is unchanged.
func (s *Store) Write(request WriteRequest) (WriteResult, error) {
	if len(request.Mutations) == 0 {
		return WriteResult{}, errors.New("profile write requires mutations")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return WriteResult{}, ErrClosed
	}
	newRevision, err := applyWrite(s.path, request, s.revision)
	if err != nil {
		return WriteResult{}, err
	}
	s.revision = newRevision
	if s.notify != nil {
		s.notify(newRevision)
	}
	return WriteResult{Revision: newRevision}, nil
}

// Close closes the store; further calls return ErrClosed.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return closeBolt(s.path)
}

func putU64(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}
