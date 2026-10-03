//go:build !windows

package terminaluse

import (
	"os"
	"strconv"
)

// platformChildProcesses enumerates direct children via procfs (Linux). On
// other Unix platforms without procfs it degrades to an empty list — the
// renderer then skips the busy confirmation exactly like today.
func platformChildProcesses(parentPID int) []ChildProcessInfo {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return []ChildProcessInfo{}
	}
	children := []ChildProcessInfo{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, convErr := strconv.Atoi(entry.Name())
		if convErr != nil || pid == parentPID {
			continue
		}
		ppid, ok := procPpid(pid)
		if !ok || ppid != parentPID {
			continue
		}
		command := procCommand(pid)
		if command == "" {
			continue
		}
		children = append(children, ChildProcessInfo{PID: pid, Command: command})
	}
	return children
}
