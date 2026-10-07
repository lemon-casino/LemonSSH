//go:build windows

package updateuse

import (
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
)

// relaunchCommand builds the detached helper that restarts the app once the
// quitting process has exited: a hidden cmd that waits, then starts the new
// image detached from this process tree.
func relaunchCommand(executable string) (string, []string) {
	command := fmt.Sprintf(`timeout /t 2 /nobreak >nul & start "" %s`, strconv.Quote(executable))
	return "cmd", []string{"/c", command}
}

// startDetachedProcess spawns the helper without a console window and
// without tying its lifetime to this process.
func startDetachedProcess(name string, argv []string) error {
	cmd := exec.Command(name, argv...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	return cmd.Start()
}
