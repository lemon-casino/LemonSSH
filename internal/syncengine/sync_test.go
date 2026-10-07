package syncengine

import (
	"testing"
)

func TestMergeLastWriterWins(t *testing.T) {
	local := map[string]Entry{"key": {Value: "local", Timestamp: 100}}
	remote := map[string]Entry{"key": {Value: "remote", Timestamp: 200}}
	merged := Merge(local, remote)
	if merged["key"].Value != "remote" {
		t.Fatalf("newer remote should win: %+v", merged["key"])
	}
	reversed := Merge(remote, local)
	if reversed["key"].Value != "remote" {
		t.Fatal("merge must be order-independent")
	}
}

func TestMergeTombstonePreservation(t *testing.T) {
	local := map[string]Entry{"key": {Deleted: true, Timestamp: 300}}
	remote := map[string]Entry{"key": {Value: "old", Timestamp: 100}}
	merged := Merge(local, remote)
	if !merged["key"].Deleted {
		t.Fatal("tombstone must win over older value")
	}
}

func TestMergeDisjointKeys(t *testing.T) {
	local := map[string]Entry{"a": {Value: "1", Timestamp: 1}}
	remote := map[string]Entry{"b": {Value: "2", Timestamp: 1}}
	merged := Merge(local, remote)
	if len(merged) != 2 {
		t.Fatalf("disjoint keys must both survive, got %d", len(merged))
	}
}

func TestFingerprintDeterministic(t *testing.T) {
	a := map[string]Entry{"x": {Value: "1", Timestamp: 1}, "y": {Value: "2", Timestamp: 2}}
	b := map[string]Entry{"y": {Value: "2", Timestamp: 2}, "x": {Value: "1", Timestamp: 1}}
	if Fingerprint(a) != Fingerprint(b) {
		t.Fatal("fingerprint must be order-independent")
	}
	if Fingerprint(a) == Fingerprint(map[string]Entry{"x": {Value: "changed", Timestamp: 1}}) {
		t.Fatal("different content must produce different fingerprint")
	}
}
