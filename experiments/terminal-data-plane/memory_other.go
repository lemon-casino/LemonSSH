//go:build !windows

package main

func platformProcessMemory() (uint64, uint64, bool, string) {
	return 0, 0, false, "process RSS sampling is not implemented for this probe platform"
}
