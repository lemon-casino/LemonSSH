package main

import "github.com/binaricat/netcatty/internal/syncengine"

type SyncService struct{}

func newSyncService() *SyncService { return &SyncService{} }

func (s *SyncService) Merge(local, remote map[string]syncengine.Entry) map[string]syncengine.Entry {
	return syncengine.Merge(local, remote)
}

func (s *SyncService) Fingerprint(entries map[string]syncengine.Entry) string {
	return syncengine.Fingerprint(entries)
}
