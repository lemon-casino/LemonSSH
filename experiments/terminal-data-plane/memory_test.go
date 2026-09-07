package main

import (
	"runtime"
	"testing"
)

func TestSnapshotReportsRealMemoryAvailability(t *testing.T) {
	service := newTestService(t)
	snapshot := service.Snapshot()
	if snapshot.Memory.GoHeapAllocBytes == 0 || snapshot.Memory.GoHeapInuseBytes == 0 ||
		snapshot.Memory.ServiceLifetimeObservedMaxGoHeapAllocBytes == 0 {
		t.Fatalf("Go heap sample is empty: %#v", snapshot.Memory)
	}
	if runtime.GOOS == "windows" {
		if !snapshot.Memory.ProcessRSSAvailable || snapshot.Memory.ProcessRSSBytes == nil || *snapshot.Memory.ProcessRSSBytes == 0 {
			t.Fatalf("Windows process RSS sample is unavailable: %#v", snapshot.Memory)
		}
	}
}
