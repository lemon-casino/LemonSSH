//go:build windows

package terminaluse

import (
	"sort"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// platformChildProcesses enumerates direct children via the Toolhelp32
// snapshot. The snapshot's ExeFile is the image name (no full command line),
// which is exactly what the close-confirmation needs ("python.exe").
func platformChildProcesses(parentPID int) []ChildProcessInfo {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return []ChildProcessInfo{}
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	children := []ChildProcessInfo{}
	for err := windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if int(entry.ParentProcessID) != parentPID || int(entry.ProcessID) == parentPID {
			continue
		}
		command := sanitizeChildCommand(strings.TrimSpace(windows.UTF16ToString(entry.ExeFile[:])))
		if command == "" {
			continue
		}
		children = append(children, ChildProcessInfo{PID: int(entry.ProcessID), Command: command})
	}
	sort.Slice(children, func(i, j int) bool { return children[i].PID < children[j].PID })
	return children
}
