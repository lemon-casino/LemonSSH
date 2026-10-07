// Package applog owns the LemonSSH runtime log (SYS-01): an append-only
// file under the logs directory next to the executable (profile dir as
// fallback), shared by Go services and the renderer's forwarded errors.
// Writes are best-effort: logging must never break the app.
package applog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	file    *os.File
	logsDir string
)

// Init creates the logs directory and opens the log file. The directory is
// created even when nothing is written yet so users can find it.
func Init(dir string) error {
	mu.Lock()
	defer mu.Unlock()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	opened, err := os.OpenFile(filepath.Join(dir, "lemonssh.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if file != nil {
		_ = file.Close()
	}
	file = opened
	logsDir = dir
	return nil
}

// Dir reports the active logs directory (empty before Init).
func Dir() string {
	mu.Lock()
	defer mu.Unlock()
	return logsDir
}

// Writer exposes the open log file for std log redirection.
func Writer() *os.File {
	mu.Lock()
	defer mu.Unlock()
	return file
}

// Infof appends one informational line.
func Infof(format string, args ...any) {
	write("INFO", fmt.Sprintf(format, args...))
}

// Errorf appends one error line.
func Errorf(format string, args ...any) {
	write("ERROR", fmt.Sprintf(format, args...))
}

func write(level, line string) {
	mu.Lock()
	defer mu.Unlock()
	if file == nil {
		return
	}
	stamped := fmt.Sprintf("%s [%s] %s\n", time.Now().Local().Format("2006-01-02 15:04:05.000"), level, line)
	_, _ = file.WriteString(stamped)
}

// Close flushes and closes the log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Close()
		file = nil
	}
}
