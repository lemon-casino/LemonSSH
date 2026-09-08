package migration

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/x509"
	"testing"

	"github.com/binaricat/netcatty/internal/platform/credentials"
	"github.com/binaricat/netcatty/internal/profile/store"
)

func seedStore(t *testing.T, path string) *store.Store {
	t.Helper()
	profileStore, err := store.Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := profileStore.Write(store.WriteRequest{Mutations: []store.Mutation{
		{Domain: "settings", Key: "netcatty_theme_v1", Value: []byte("dark")},
		{Domain: "vault", Key: "netcatty_hosts_v1", Value: []byte(`{"hosts":[]}`)},
	}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = profileStore.Close() })
	return profileStore
}

func TestRollbackBundleRoundTripsThroughImport(t *testing.T) {
	dir := t.TempDir()
	source := seedStore(t, dir+"/source.db")

	target, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	targetSPKI, err := x509.MarshalPKIXPublicKey(target.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	bundleBytes, err := BuildRollbackBundle(source, targetSPKI, "rollback-fingerprint-1")
	if err != nil {
		t.Fatalf("build rollback bundle: %v", err)
	}

	mutations, err := Import(bundleBytes, target, "rollback-fingerprint-1", fakeProvider{available: true})
	if err != nil {
		t.Fatalf("import rollback bundle: %v", err)
	}
	if len(mutations) != 2 {
		t.Fatalf("expected 2 mutations, got %d", len(mutations))
	}
}

func TestRollbackBundleCrashMatrixFailClosed(t *testing.T) {
	dir := t.TempDir()
	source := seedStore(t, dir+"/source.db")
	target, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	targetSPKI, err := x509.MarshalPKIXPublicKey(target.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	bundleBytes, err := BuildRollbackBundle(source, targetSPKI, "rollback-fingerprint-1")
	if err != nil {
		t.Fatal(err)
	}

	// Tamper every sampled byte position; the import must never succeed on a
	// corrupted bundle.
	corrupted := append([]byte(nil), bundleBytes...)
	step := len(corrupted) / 16
	if step == 0 {
		step = 1
	}
	for offset := 0; offset < len(corrupted); offset += step {
		damaged := append([]byte(nil), bundleBytes...)
		damaged[offset] ^= 0xFF
		if _, err := Import(damaged, target, "rollback-fingerprint-1", fakeProvider{available: true}); err == nil {
			t.Fatalf("corrupted bundle accepted at offset %d", offset)
		}
	}

	// Wrong fingerprint fails closed even on an intact bundle.
	if _, err := Import(bundleBytes, target, "wrong-fingerprint", fakeProvider{available: true}); err == nil {
		t.Fatal("wrong fingerprint accepted")
	}
}

func TestRollbackOfEmptyStoreFails(t *testing.T) {
	dir := t.TempDir()
	empty, err := store.Open(dir+"/empty.db", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer empty.Close()
	target, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	targetSPKI, err := x509.MarshalPKIXPublicKey(target.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildRollbackBundle(empty, targetSPKI, "fp-empty-store"); err == nil {
		t.Fatal("empty store rollback must fail")
	}
	_ = credentials.ErrUnavailable
}
