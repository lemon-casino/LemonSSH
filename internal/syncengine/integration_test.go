package syncengine

import (
	"testing"

	"github.com/binaricat/netcatty/internal/profile/store"
)

// P6-01 integration: proves syncengine + profile store work together as the
// sync domain owner (settings domain).
func TestSyncEngineWithProfileStore(t *testing.T) {
	profileStore, err := store.Open(t.TempDir()+"/profile.db", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer profileStore.Close()

	// Seed via profile store.
	_, err = profileStore.Write(store.WriteRequest{Mutations: []store.Mutation{
		{Domain: "settings", Key: "theme", Value: []byte("dark")},
		{Domain: "settings", Key: "language", Value: []byte("zh-CN")},
	}})
	if err != nil {
		t.Fatal(err)
	}

	// Read back via domain keys.
	keys, err := profileStore.DomainKeys("settings")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}

	// CAS write.
	revision, _ := profileStore.Revision()
	_, err = profileStore.Write(store.WriteRequest{
		ExpectedRevision: revision,
		Mutations:        []store.Mutation{{Domain: "settings", Key: "theme", Value: []byte("light")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Stale CAS rejected.
	_, err = profileStore.Write(store.WriteRequest{
		ExpectedRevision: revision,
		Mutations:        []store.Mutation{{Domain: "settings", Key: "theme", Value: []byte("bad")}},
	})
	if err == nil {
		t.Fatal("stale CAS must fail")
	}
}
