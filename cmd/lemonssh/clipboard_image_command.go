package main

import (
	"fmt"
	"io"
	"os/exec"
)

// Read bounded stdout instead of allowing a helper to allocate unbounded output.
func boundedClipboardOutput(cmd *exec.Cmd, limit int) ([]byte, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, int64(limit)+1))
	if readErr != nil || len(output) > limit {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if readErr != nil {
		return nil, readErr
	}
	if len(output) > limit {
		return nil, fmt.Errorf("clipboard output exceeds size limit")
	}
	if waitErr != nil {
		return nil, fmt.Errorf("clipboard helper failed: %w", waitErr)
	}
	return output, nil
}
