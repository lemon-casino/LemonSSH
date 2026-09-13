package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/binaricat/netcatty/internal/profile/store"
)

func TestSyncRotationProfileTransactionAtomicity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.db")
	profile, err := store.Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	service := newProfileService(profile)
	keys := []string{"netcatty_master_key_config_v1", "netcatty_convergent_sync_replica_v2", "netcatty_convergent_sync_provider_baseline_v2_github", "netcatty_sync_base_payload_v1_github", "netcatty_sync_snapshots_v1_github"}
	for _, key := range keys {
		if err := service.SetRaw("settings", key, []byte("old:"+key)); err != nil {
			t.Fatal(err)
		}
	}
	revision, _ := service.Revision()
	var mutations []store.Mutation
	for _, key := range keys {
		mutations = append(mutations, store.Mutation{Domain: "settings", Key: key, Value: []byte("new:" + key)})
	}
	invalid := append(append([]store.Mutation{}, mutations...), store.Mutation{Domain: "invalid", Key: "failure", Value: []byte("fail")})
	if _, err := service.Write(revision, invalid); err == nil {
		t.Fatal("invalid transaction succeeded")
	}
	for _, key := range keys {
		value, _ := service.GetRaw("settings", key)
		if !bytes.Equal(value, []byte("old:"+key)) {
			t.Fatal("partially rotated profile")
		}
	}
	if actual, _ := service.Revision(); actual != revision {
		t.Fatal("failed transaction changed revision")
	}
	if _, err := service.Write(revision, mutations); err != nil {
		t.Fatal(err)
	}
	profile.Close()
	profile, err = store.Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer profile.Close()
	service = newProfileService(profile)
	for _, key := range keys {
		value, _ := service.GetRaw("settings", key)
		if !bytes.Equal(value, []byte("new:"+key)) {
			t.Fatal("rotation not durable across reopen")
		}
	}
}
