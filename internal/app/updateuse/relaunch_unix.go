//go:build !windows

package updateuse

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// relaunchCommand builds the detached helper that restarts the app once the
// quitting process has exited: a shell that waits, then execs the new image
// (keeping the exit status of the successor).
func relaunchCommand(executable string) (string, []string) {
	escaped := strings.ReplaceAll(executable, "'", `'\''`)
	return "/bin/sh", []string{"-c", fmt.Sprintf("sleep 2; exec '%s'", escaped)}
}

// startDetachedProcess spawns the helper in its own session so it survives
// this process exiting.
func startDetachedProcess(name string, argv []string) error {
	cmd := exec.Command(name, argv...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}
