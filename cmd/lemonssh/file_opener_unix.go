//go:build !windows

package main

import (
	"os/exec"
	"runtime"
)

func configureFileOpenProcess(cmd *exec.Cmd) {}

func openSystemFile(filePath string) error {
	command := "xdg-open"
	if runtime.GOOS == "darwin" {
		command = "open"
	}
	// These launcher commands exit after submitting the open request, so report
	// their failure instead of claiming that a missing handler opened the file.
	return exec.Command(command, filePath).Run()
}
