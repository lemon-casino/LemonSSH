package main

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestCanonicalSustainedFixtureExactDigest(t *testing.T) {
	fixture, err := loadCanonicalFixture()
	if err != nil {
		t.Fatal(err)
	}
	workload, err := newWorkload(WorkloadSustained, WorkloadOptions{}, fixture)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	expectation := workload.Expectation()
	chunks := 0
	bytes := 0
	for {
		chunk, ok, err := workload.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		chunks++
		bytes += len(chunk.payload)
		if chunk.cost != uint32(len(chunk.payload)) {
			t.Fatalf("chunk %d credit cost mismatch", chunks)
		}
		_, _ = hash.Write(chunk.payload)
	}
	if chunks != fixture.Summary.ChunkCount || bytes != fixture.Summary.TotalBytes {
		t.Fatalf("summary mismatch: chunks=%d bytes=%d", chunks, bytes)
	}
	if expectation.payloadBytes != uint64(bytes) || expectation.creditBytes != uint64(bytes) || expectation.frames != uint64(chunks) {
		t.Fatalf("sustained expectation mismatch: %#v", expectation)
	}
	if digest := hex.EncodeToString(hash.Sum(nil)); digest != fixture.Summary.PayloadSHA256 || digest != canonicalPayloadSHA256 {
		t.Fatalf("digest mismatch: %s", digest)
	}
}

func TestDeterministicWorkloadBounds(t *testing.T) {
	fixture, err := loadCanonicalFixture()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		id      string
		options WorkloadOptions
	}{
		{id: WorkloadLongLine, options: WorkloadOptions{TotalBytes: maxPayloadBytes + 1}},
		{id: WorkloadMillionLines, options: WorkloadOptions{LineCount: 10_000}},
		{id: WorkloadMetadataOnly, options: WorkloadOptions{MetadataFrames: 3}},
	}
	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			workload, err := newWorkload(test.id, test.options, fixture)
			if err != nil {
				t.Fatal(err)
			}
			expectation := workload.Expectation()
			var payloadBytes, creditBytes, frames uint64
			for {
				chunk, ok, err := workload.Next()
				if err != nil {
					t.Fatal(err)
				}
				if !ok {
					break
				}
				if len(chunk.payload) > maxPayloadBytes || chunk.cost == 0 {
					t.Fatalf("unbounded or free chunk: payload=%d cost=%d", len(chunk.payload), chunk.cost)
				}
				payloadBytes += uint64(len(chunk.payload))
				creditBytes += uint64(chunk.cost)
				frames++
			}
			if payloadBytes != expectation.payloadBytes || creditBytes != expectation.creditBytes || frames != expectation.frames {
				t.Fatalf("workload expectation %#v, actual payload=%d credit=%d frames=%d", expectation, payloadBytes, creditBytes, frames)
			}
		})
	}
}
