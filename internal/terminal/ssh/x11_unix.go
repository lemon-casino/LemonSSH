//go:build !windows

package ssh

import "os/exec"

func hideXauthWindow(cmd *exec.Cmd) {}
