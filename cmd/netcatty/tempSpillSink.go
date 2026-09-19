package main

import (
	"fmt"

	"github.com/binaricat/netcatty/internal/platform/filesystem"
)

// tempSpillSink persists oversized tool-output handles into the Netcatty
// dedicated temp directory via the TempService lease registry — never
// os.TempDir (workspace + design §6.3 rule). Staging entries are leased,
// so the boot sweep in later sessions cleans them while this session's
// remain protected.
type tempSpillSink struct {
	temp *filesystem.TempService
}

func (s tempSpillSink) Spill(handleID, content string) (string, error) {
	staged, err := s.temp.CreateStagingFile(fmt.Sprintf("tool-output-%s.txt", handleID))
	if err != nil {
		return "", err
	}
	if _, err := staged.WriteString(content); err != nil {
		staged.Close()
		return "", err
	}
	if err := staged.Close(); err != nil {
		return "", err
	}
	return staged.Name(), nil
}
