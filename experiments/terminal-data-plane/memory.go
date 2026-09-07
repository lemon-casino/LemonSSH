package main

import "runtime"

type MemorySnapshot struct {
	SampledAtUnixMillis                        int64   `json:"sampledAtUnixMillis"`
	GoHeapAllocBytes                           uint64  `json:"goHeapAllocBytes"`
	GoHeapInuseBytes                           uint64  `json:"goHeapInuseBytes"`
	ServiceLifetimeObservedMaxGoHeapAllocBytes uint64  `json:"serviceLifetimeObservedMaxGoHeapAllocBytes"`
	ProcessRSSBytes                            *uint64 `json:"processRssBytes"`
	ProcessLifetimePeakRSSBytes                *uint64 `json:"processLifetimePeakRssBytes"`
	ProcessRSSAvailable                        bool    `json:"processRssAvailable"`
	ProcessRSSError                            string  `json:"processRssError"`
}

func (p *ProbeService) memorySnapshotLocked() MemorySnapshot {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	if stats.HeapAlloc > p.serviceLifetimeObservedMaxGoHeapAllocBytes {
		p.serviceLifetimeObservedMaxGoHeapAllocBytes = stats.HeapAlloc
	}
	rss, peakRSS, available, rssError := platformProcessMemory()
	result := MemorySnapshot{
		SampledAtUnixMillis:                        p.now().UnixMilli(),
		GoHeapAllocBytes:                           stats.HeapAlloc,
		GoHeapInuseBytes:                           stats.HeapInuse,
		ServiceLifetimeObservedMaxGoHeapAllocBytes: p.serviceLifetimeObservedMaxGoHeapAllocBytes,
		ProcessRSSAvailable:                        available,
		ProcessRSSError:                            rssError,
	}
	if available {
		result.ProcessRSSBytes = &rss
		result.ProcessLifetimePeakRSSBytes = &peakRSS
	}
	return result
}
