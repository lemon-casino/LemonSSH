package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/binaricat/netcatty/internal/platform/applog"
)

// DiagnosticLogService receives renderer-side diagnostic/error lines and
// stores them in the shared logs directory.
type DiagnosticLogService struct{}

func newDiagnosticLogService() *DiagnosticLogService { return &DiagnosticLogService{} }

func (s *DiagnosticLogService) Append(line string) (bool, error) {
	if applog.Dir() == "" {
		return false, nil
	}
	applog.Infof("renderer: %s", line)
	return true, nil
}

type CrashLogFile struct {
	FileName   string `json:"fileName"`
	Date       string `json:"date"`
	Size       int64  `json:"size"`
	EntryCount int    `json:"entryCount"`
}

type CrashLogEntry struct {
	Timestamp string `json:"timestamp"`
	Source    string `json:"source"`
	Message   string `json:"message"`
	Stack     string `json:"stack,omitempty"`
	PID       int    `json:"pid,omitempty"`
	Platform  string `json:"platform,omitempty"`
	Arch      string `json:"arch,omitempty"`
	Version   string `json:"version,omitempty"`
}

type DeletedLogsResult struct {
	DeletedCount int `json:"deletedCount"`
}
type OpenLogsResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}
type SSHDebugLogInfo struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Size    int64  `json:"size"`
}

var applicationLogLine = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?) \[([^]]+)] (.*)$`)

func diagnosticLogsDir() string {
	if directory := applog.Dir(); directory != "" {
		return directory
	}
	return filepath.Join(baseProfileDir(), "logs")
}

func safeLogPath(fileName string) (string, error) {
	if fileName == "" || filepath.Base(fileName) != fileName || strings.ContainsAny(fileName, `/\\`) {
		return "", fmt.Errorf("invalid log file name")
	}
	return filepath.Join(diagnosticLogsDir(), fileName), nil
}

func countLogLines(path string) int {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()
	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		count++
	}
	return count
}

func (s *DiagnosticLogService) GetCrashLogs() ([]CrashLogFile, error) {
	directory := diagnosticLogsDir()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	result := []CrashLogFile{}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || strings.ToLower(filepath.Ext(entry.Name())) != ".log" {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		result = append(result, CrashLogFile{FileName: entry.Name(), Date: info.ModTime().Format(time.RFC3339), Size: info.Size(), EntryCount: countLogLines(filepath.Join(directory, entry.Name()))})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Date > result[j].Date })
	return result, nil
}

func (s *DiagnosticLogService) ReadCrashLog(fileName string) ([]CrashLogEntry, error) {
	path, err := safeLogPath(fileName)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	result := []CrashLogEntry{}
	scanner := bufio.NewScanner(file)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		entry := CrashLogEntry{Timestamp: "", Source: "application", Message: line}
		if match := applicationLogLine.FindStringSubmatch(line); len(match) == 4 {
			entry.Timestamp, entry.Source, entry.Message = match[1], strings.ToLower(match[2]), match[3]
		}
		result = append(result, entry)
	}
	return result, scanner.Err()
}

func (s *DiagnosticLogService) ClearCrashLogs() (DeletedLogsResult, error) {
	logs, err := s.GetCrashLogs()
	if err != nil {
		return DeletedLogsResult{}, err
	}
	deleted := 0
	active := filepath.Clean(filepath.Join(diagnosticLogsDir(), "lemonssh.log"))
	for _, logFile := range logs {
		path, pathErr := safeLogPath(logFile.FileName)
		if pathErr != nil || filepath.Clean(path) == active {
			continue
		}
		if os.Remove(path) == nil {
			deleted++
		}
	}
	return DeletedLogsResult{DeletedCount: deleted}, nil
}

func (s *DiagnosticLogService) OpenCrashLogsDir() OpenLogsResult {
	directory := diagnosticLogsDir()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return OpenLogsResult{Error: err.Error()}
	}
	if err := openSystemFile(directory); err != nil {
		return OpenLogsResult{Error: err.Error()}
	}
	return OpenLogsResult{Success: true}
}

func sshDebugLogPath() string { return filepath.Join(diagnosticLogsDir(), "ssh-debug.log") }

func (s *DiagnosticLogService) GetSshDebugLogInfo() SSHDebugLogInfo {
	path := sshDebugLogPath()
	info, err := os.Stat(path)
	return SSHDebugLogInfo{Enabled: os.Getenv("LEMONSSH_SSH_DEBUG") == "1" || os.Getenv("NETCATTY_SSH_DEBUG") == "1", Path: path, Exists: err == nil && info.Mode().IsRegular(), Size: func() int64 {
		if err == nil {
			return info.Size()
		}
		return 0
	}()}
}

func (s *DiagnosticLogService) OpenSshDebugLogDir() OpenLogsResult { return s.OpenCrashLogsDir() }
