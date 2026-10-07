// Package sshdebug owns the opt-in SSH diagnostic log (ssh-debug.log). It
// records connection, auth, handshake, disconnect and error reasons — never
// terminal output or credentials. Enabled state is process-global so the
// settings toggle applies to every dial immediately.
package sshdebug

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	enabled bool
	path    string
)

// SetPath points the logger at the diagnostics log directory. Safe to call
// repeatedly; the next write uses the new path.
func SetPath(value string) {
	mu.Lock()
	defer mu.Unlock()
	path = value
}

// Path reports the current log file path ("" when unset).
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return path
}

// SetEnabled flips the toggle (settings switch or LEMONSSH_SSH_DEBUG=1).
func SetEnabled(value bool) {
	mu.Lock()
	defer mu.Unlock()
	enabled = value
}

// Enabled reports whether debug events are being recorded.
func Enabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return enabled
}

// Logf appends one timestamped line when enabled. Writes are open-append-close
// so the file never needs lifecycle management and log rotation is safe.
// Logging failures are silently swallowed: diagnostics must never break a dial.
func Logf(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if !enabled || path == "" {
		return
	}
	directory := filepath.Dir(path)
	if directory != "" {
		_ = os.MkdirAll(directory, 0o755)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	stamp := time.Now().Format("2006-01-02 15:04:05.000")
	fmt.Fprintf(file, "%s %s\n", stamp, fmt.Sprintf(format, args...))
}

// LogError appends one line prefixed with ERROR when enabled.
func LogError(format string, args ...any) {
	Logf("ERROR "+format, args...)
}
