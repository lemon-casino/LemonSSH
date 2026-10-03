package terminaluse

import (
	"os"
	"strconv"
	"strings"
)

// ChildProcessInfo is one direct child of a local terminal's shell process.
// The renderer uses it to warn before closing a terminal that is still running
// a command ("terminal is running: make").
type ChildProcessInfo struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
}

// maxBusyChildCommands bounds the response; the confirm dialog shows one
// primary command plus a count.
const maxBusyChildCommands = 16

// PtyGetChildProcesses lists the direct children of a local session's shell
// (empty for remote/serial sessions — only local shells are killable here).
func (s *Service) PtyGetChildProcesses(sessionID string) []ChildProcessInfo {
	_, term, err := s.attachTerm(sessionID)
	if err != nil || term.local == nil {
		return []ChildProcessInfo{}
	}
	pid := term.local.PID()
	if pid <= 0 {
		return []ChildProcessInfo{}
	}
	children := listChildProcesses(pid)
	if len(children) > maxBusyChildCommands {
		children = children[:maxBusyChildCommands]
	}
	return children
}

// listChildProcesses is the platform seam (procfs vs Toolhelp32).
func listChildProcesses(parentPID int) []ChildProcessInfo {
	return platformChildProcesses(parentPID)
}

// sanitizeChildCommand strips control characters so hostile process titles
// cannot smuggle terminal escape sequences into the confirm dialog.
func sanitizeChildCommand(command string) string {
	var builder strings.Builder
	for _, r := range command {
		if r < 32 || r == 127 {
			builder.WriteRune(' ')
			continue
		}
		builder.WriteRune(r)
	}
	return strings.TrimSpace(builder.String())
}

// procPpid reads "PPid:" from /proc/<pid>/status (Linux).
func procPpid(pid int) (int, bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if value, ok := strings.CutPrefix(line, "PPid:"); ok {
			ppid, convErr := strconv.Atoi(strings.TrimSpace(value))
			if convErr == nil {
				return ppid, true
			}
			return 0, false
		}
	}
	return 0, false
}

// procCommand reads /proc/<pid>/cmdline, falling back to the comm name from
// /proc/<pid>/stat.
func procCommand(pid int) string {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err == nil {
		command := strings.ReplaceAll(string(raw), "\x00", " ")
		if sanitized := sanitizeChildCommand(command); sanitized != "" {
			return sanitized
		}
	}
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return ""
	}
	// comm sits between the first "(" and the last ")".
	stat := string(data)
	open := strings.IndexByte(stat, '(')
	close := strings.LastIndexByte(stat, ')')
	if open < 0 || close <= open {
		return ""
	}
	return sanitizeChildCommand(stat[open+1 : close])
}
