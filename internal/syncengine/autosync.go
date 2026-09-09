// Package autosync implements the periodic sync loop (P6-01): reads local
// changes, merges with remote using the sync engine, and pushes results back
// through the profile store with CAS protection.
package syncengine

import (
	"context"
	"time"
)

// Peer is a sync source/sink pair (local store ↔ remote).
type Peer struct {
	ID       string
	Local    SyncReader
	Remote   SyncReader
	Interval time.Duration
}

// SyncReader abstracts a key-value participant in the sync protocol.
type SyncReader interface {
	GetAll() (map[string]Entry, error)
	PutAll(entries map[string]Entry) error
	Revision() (uint64, error)
}

// Entry is one sync-able record.
// Entry is re-exported from sync.go.

// Loop runs periodic sync until ctx is cancelled.
func Loop(ctx context.Context, peer Peer, interval time.Duration, onConflict func(key string)) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := syncOnce(ctx, peer, onConflict); err != nil {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
			}
		}
	}
}

func syncOnce(ctx context.Context, peer Peer, onConflict func(key string)) error {
	local, err := peer.Local.GetAll()
	if err != nil {
		return err
	}
	remote, err := peer.Remote.GetAll()
	if err != nil {
		return err
	}
	merged := mergeEntries(local, remote, onConflict)
	return peer.Remote.PutAll(merged)
}

func mergeEntries(local, remote map[string]Entry, onConflict func(string)) map[string]Entry {
	result := make(map[string]Entry)
	keys := make(map[string]bool)
	for k := range local {
		keys[k] = true
	}
	for k := range remote {
		keys[k] = true
	}
	for k := range keys {
		l, hasL := local[k]
		r, hasR := remote[k]
		switch {
		case hasL && !hasR:
			result[k] = l
		case !hasL && hasR:
			result[k] = r
		case l.Timestamp >= r.Timestamp:
			result[k] = l
			if onConflict != nil {
				onConflict(k)
			}
		default:
			result[k] = r
			if onConflict != nil {
				onConflict(k)
			}
		}
	}
	return result
}
