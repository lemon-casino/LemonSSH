package main

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// DiagnosticLogService appends timestamped lines to a diagnostics file in the
// profile directory. It exists to debug machine-specific drag-drop behavior
// where renderer toasts lose the details.
type DiagnosticLogService struct {
	mu  sync.Mutex
	dir string
}

func newDiagnosticLogService(dataDir string) *DiagnosticLogService {
	return &DiagnosticLogService{dir: dataDir}
}

// Append writes one timestamped line to diagnostics.log.
func (s *DiagnosticLogService) Append(line string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == "" {
		return false, nil
	}
	path := s.dir + "/diagnostics.log"
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return false, err
	}
	defer file.Close()
	stamped := fmt.Sprintf("%s %s\n", time.Now().UTC().Format(time.RFC3339Nano), line)
	if _, err := file.WriteString(stamped); err != nil {
		return false, err
	}
	return true, nil
}
