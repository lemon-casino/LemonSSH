// Package sync implements the Go-side convergent sync merge engine (P6-01):
// last-writer-wins per key with tombstone support and semantic fingerprinting
// for differential verification. The CRDT lineage is preserved: merged
// results are deterministic regardless of arrival order.
package syncengine

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// Entry is one sync-able key-value pair with a modification timestamp.
type Entry struct {
	Value     string `json:"value"`
	Deleted   bool   `json:"deleted,omitempty"`
	Timestamp int64  `json:"timestamp"` // Unix millis
}

// Merge combines local and remote entries using last-writer-wins per key.
// Deleted entries become tombstones (preserved to prevent resurrection).
func Merge(local, remote map[string]Entry) map[string]Entry {
	result := make(map[string]Entry)
	keys := make(map[string]bool)
	for key := range local {
		keys[key] = true
	}
	for key := range remote {
		keys[key] = true
	}
	for key := range keys {
		l, hasLocal := local[key]
		r, hasRemote := remote[key]
		switch {
		case hasLocal && !hasRemote:
			result[key] = l
		case !hasLocal && hasRemote:
			result[key] = r
		case l.Timestamp >= r.Timestamp:
			result[key] = l
		default:
			result[key] = r
		}
	}
	return result
}

// Fingerprint computes a deterministic semantic hash over a set of entries.
// The hash is order-independent: entries are sorted by key before hashing.
func Fingerprint(entries map[string]Entry) string {
	if len(entries) == 0 {
		return hex.EncodeToString(sha256Sum([]byte("empty")))
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hasher := sha256.New()
	for _, key := range keys {
		entry := entries[key]
		hasher.Write([]byte(key))
		hasher.Write([]byte{0})
		hasher.Write([]byte(entry.Value))
		hasher.Write([]byte{0})
		if entry.Deleted {
			hasher.Write([]byte{1})
		} else {
			hasher.Write([]byte{0})
		}
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func sha256Sum(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}
