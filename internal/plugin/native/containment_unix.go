//go:build !windows && !js

package native

import (
	"os/exec"
	"syscall"
)

// applyContainment puts the child in its own process group so descendants
// can be signalled together.
func applyContainment(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func signalGraceful(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
}

func killTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}

func assignJob(*exec.Cmd) (func() error, error) {
	return func() error { return nil }, nil
}
