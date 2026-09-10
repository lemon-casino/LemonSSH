package main

import (
	"github.com/binaricat/netcatty/internal/platform/applog"
)

// DiagnosticLogService receives renderer-side diagnostic/error lines and
// stores them in the shared logs directory.
type DiagnosticLogService struct{}

func newDiagnosticLogService() *DiagnosticLogService {
	return &DiagnosticLogService{}
}

// Append writes one timestamped line to the logs directory.
func (s *DiagnosticLogService) Append(line string) (bool, error) {
	if applog.Dir() == "" {
		return false, nil
	}
	applog.Infof("renderer: %s", line)
	return true, nil
}
