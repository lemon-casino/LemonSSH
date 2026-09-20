package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/binaricat/netcatty/internal/terminal/sessionlog"
)

type ManualSessionLogResult struct {
	Success   bool   `json:"success"`
	Started   bool   `json:"started,omitempty"`
	Stopped   bool   `json:"stopped,omitempty"`
	IsLogging bool   `json:"isLogging,omitempty"`
	FilePath  string `json:"filePath,omitempty"`
	Error     string `json:"error,omitempty"`
}

type SessionLogsClearResult struct {
	Success      bool   `json:"success"`
	DeletedCount int    `json:"deletedCount"`
	FailedCount  int    `json:"failedCount"`
	Error        string `json:"error,omitempty"`
}

type SessionLogService struct{ manager *sessionlog.Manager }

func newSessionLogService(manager *sessionlog.Manager) *SessionLogService {
	return &SessionLogService{manager: manager}
}

func (s *SessionLogService) Start(sessionID, filePath, initialLine string) ManualSessionLogResult {
	resolved, err := s.manager.Start(strings.TrimSpace(sessionID), strings.TrimSpace(filePath))
	if err != nil {
		return ManualSessionLogResult{Error: err.Error()}
	}
	if initialLine != "" {
		s.manager.Append(sessionID, []byte(initialLine))
		if !strings.HasSuffix(initialLine, "\n") {
			s.manager.Append(sessionID, []byte("\n"))
		}
	}
	return ManualSessionLogResult{Success: true, Started: true, IsLogging: true, FilePath: resolved}
}

func (s *SessionLogService) Stop(sessionID string) ManualSessionLogResult {
	_, path := s.manager.Status(sessionID)
	if err := s.manager.Stop(sessionID); err != nil {
		return ManualSessionLogResult{Error: err.Error(), FilePath: path}
	}
	return ManualSessionLogResult{Success: true, Stopped: true, FilePath: path}
}

func (s *SessionLogService) Status(sessionID string) ManualSessionLogResult {
	active, path := s.manager.Status(sessionID)
	return ManualSessionLogResult{Success: true, IsLogging: active, FilePath: path}
}

func (s *SessionLogService) OpenDirectory(directory string) ManualSessionLogResult {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return ManualSessionLogResult{Error: "session log directory is required"}
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return ManualSessionLogResult{Error: err.Error()}
	}
	if err := openSystemFile(directory); err != nil {
		return ManualSessionLogResult{Error: err.Error()}
	}
	return ManualSessionLogResult{Success: true, FilePath: directory}
}

func (s *SessionLogService) ClearDirectory(directory string) SessionLogsClearResult {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return SessionLogsClearResult{Error: "session log directory is required"}
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return SessionLogsClearResult{Error: err.Error()}
	}
	if absolute == filepath.VolumeName(absolute)+string(os.PathSeparator) {
		return SessionLogsClearResult{Error: "refusing to clear a filesystem root"}
	}
	entries, err := os.ReadDir(absolute)
	if os.IsNotExist(err) {
		return SessionLogsClearResult{Success: true}
	}
	if err != nil {
		return SessionLogsClearResult{Error: err.Error()}
	}
	result := SessionLogsClearResult{Success: true}
	for _, entry := range entries {
		path := filepath.Join(absolute, entry.Name())
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if extension != ".log" && extension != ".txt" && extension != ".raw" && extension != ".html" {
			continue
		}
		if removeErr := os.Remove(path); removeErr != nil {
			result.FailedCount++
			result.Success = false
			if result.Error == "" {
				result.Error = fmt.Sprintf("remove %s: %v", entry.Name(), removeErr)
			}
		} else {
			result.DeletedCount++
		}
	}
	return result
}
