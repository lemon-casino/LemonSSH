package main

import (
	"log"

	"github.com/binaricat/netcatty/internal/platform/filesystem"
)

// sweepTempOrphans removes staging leftovers from a previous session at boot.
// A fresh process holds no leases, so every staged-upload file and
// active-transfer directory in the managed temp root is an orphan; files
// outside those prefixes (external-edit downloads) are preserved. Failures
// are logged and never block boot.
func sweepTempOrphans(temp *filesystem.TempService) int {
	removed, err := temp.CleanupOrphans()
	if err != nil {
		log.Printf("temp orphan cleanup failed: %v", err)
		return removed
	}
	if removed > 0 {
		log.Printf("removed %d orphan temp entr(y/ies) from a previous session", removed)
	}
	return removed
}
