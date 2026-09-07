package store

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile.db")
	s, err := Open(path, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestRawValueRoundTripAndBounds(t *testing.T) {
	s := openTestStore(t)
	if err := s.SetRaw("vault", "netcatty_hosts_v1", []byte(`{"hosts":[]}`)); err != nil {
		t.Fatalf("set: %v", err)
	}
	value, err := s.GetRaw("vault", "netcatty_hosts_v1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(value) != `{"hosts":[]}` {
		t.Fatalf("round trip mismatch: %s", value)
	}

	if _, err := s.GetRaw("vault", "missing"); !errors.Is(err, ErrNoSuchKey) {
		t.Fatalf("missing key must be ErrNoSuchKey, got %v", err)
	}
	if err := s.SetRaw("not-a-domain", "k", []byte("v")); !errors.Is(err, ErrUnknownDomain) {
		t.Fatalf("unknown domain must fail, got %v", err)
	}
	if err := s.SetRaw("vault", "", []byte("v")); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("empty key must fail, got %v", err)
	}
	if err := s.SetRaw("vault", "bad\x01key", []byte("v")); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("control chars must fail, got %v", err)
	}
	if err := s.DeleteRaw("vault", "netcatty_hosts_v1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetRaw("vault", "netcatty_hosts_v1"); !errors.Is(err, ErrNoSuchKey) {
		t.Fatal("deleted key must be gone")
	}
}

func TestRevisionCASAndConflict(t *testing.T) {
	s := openTestStore(t)
	revision, err := s.Revision()
	if err != nil {
		t.Fatalf("revision: %v", err)
	}
	if revision != 0 {
		t.Fatalf("fresh store revision must be 0, got %d", revision)
	}
	result, err := s.Write(WriteRequest{
		ExpectedRevision: 0, // 0 = no CAS check
		Mutations:        []Mutation{{Domain: "settings", Key: "theme", Value: []byte("dark")}},
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if result.Revision != 1 {
		t.Fatalf("revision must advance to 1, got %d", result.Revision)
	}
	// A current CAS succeeds and advances the revision...
	if _, err := s.Write(WriteRequest{
		ExpectedRevision: 1,
		Mutations:        []Mutation{{Domain: "settings", Key: "theme", Value: []byte("darker")}},
	}); err != nil {
		t.Fatalf("current CAS must succeed: %v", err)
	}
	// ...and a stale CAS (1, while the store is at 2) must conflict.
	if _, err := s.Write(WriteRequest{
		ExpectedRevision: 1,
		Mutations:        []Mutation{{Domain: "settings", Key: "theme", Value: []byte("light")}},
	}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale CAS must conflict, got %v", err)
	}
	value, err := s.GetRaw("settings", "theme")
	if err != nil || string(value) != "darker" {
		t.Fatalf("conflicting write must not land, got %s (%v)", value, err)
	}
}

func TestTransactionAtomicity(t *testing.T) {
	s := openTestStore(t)
	if err := s.SetRaw("vault", "good", []byte("v1")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Second mutation is invalid, so the whole transaction must roll back.
	_, err := s.Write(WriteRequest{Mutations: []Mutation{
		{Domain: "vault", Key: "good", Value: []byte("v2")},
		{Domain: "nope", Key: "bad", Value: []byte("x")},
	}})
	if err == nil {
		t.Fatal("invalid transaction must fail")
	}
	value, err := s.GetRaw("vault", "good")
	if err != nil || string(value) != "v1" {
		t.Fatalf("rollback violated, got %s (%v)", value, err)
	}
}

func TestNotificationsFireAfterCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.db")
	var notified []uint64
	var mu sync.Mutex
	s, err := Open(path, func(revision uint64) {
		mu.Lock()
		notified = append(notified, revision)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	if err := s.SetRaw("vault", "k", []byte("v")); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := s.Write(WriteRequest{Mutations: []Mutation{{Domain: "vault", Key: "k", Delete: true}}}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(notified) != 2 || notified[0] != 1 || notified[1] != 2 {
		t.Fatalf("notifications mismatch: %v", notified)
	}
}

func TestReopenPreservesDataAndRevision(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.db")
	s, err := Open(path, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := s.Write(WriteRequest{Mutations: []Mutation{{Domain: "vault", Key: "k", Value: []byte{byte(i)}}}}); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	reopened, err := Open(path, nil)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	revision, err := reopened.Revision()
	if err != nil {
		t.Fatalf("revision: %v", err)
	}
	if revision != 3 {
		t.Fatalf("revision must survive reopen, got %d", revision)
	}
	value, err := reopened.GetRaw("vault", "k")
	if err != nil || len(value) != 1 || value[0] != 2 {
		t.Fatalf("data must survive reopen, got %v (%v)", value, err)
	}
}

func TestConcurrentWritesSerialize(t *testing.T) {
	s := openTestStore(t)
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				if _, err := s.Write(WriteRequest{Mutations: []Mutation{
					{Domain: "vault", Key: "counter", Value: []byte("x")},
				}}); err != nil {
					t.Errorf("concurrent write: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	revision, err := s.Revision()
	if err != nil {
		t.Fatalf("revision: %v", err)
	}
	if revision != 200 {
		t.Fatalf("expected exactly 200 committed revisions, got %d", revision)
	}
}

func TestMalformedAndOversizedReject(t *testing.T) {
	s := openTestStore(t)
	huge := make([]byte, 8<<20) // 8 MiB raw value
	if err := s.SetRaw("vault", "huge", huge); err == nil {
		t.Fatal("oversized raw value must be rejected")
	}
	if err := s.SetRaw("vault", "k", nil); err == nil {
		t.Fatal("nil value must be rejected")
	}
}
