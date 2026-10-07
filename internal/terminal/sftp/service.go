// Package sftp owns SFTP browsing semantics (P3-05): list, stat, read, write,
// rename, mkdir, delete over an authenticated SSH transport. The service never
// opens its own SSH connections — it borrows a client from the caller (the
// shared pool in P3-04A) and bounds concurrent clients per session.
package sftp

import (
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"
)

var (
	ErrClientMissing  = errors.New("sftp client not available")
	ErrTooManyClients = errors.New("sftp client limit exceeded for session")
	ErrPathRequired   = errors.New("sftp path is required")
	ErrNameInvalid    = errors.New("sftp name is invalid")
	ErrOffsetNegative = errors.New("sftp offset is negative")
)

// Entry is one directory listing row.
type Entry struct {
	Name    string    `json:"name"`
	IsDir   bool      `json:"isDir"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	ModTime time.Time `json:"modTime"`
	Symlink bool      `json:"symlink,omitempty"`
}

// FileInfo is the stat payload for one path.
type FileInfo struct {
	Path    string    `json:"path"`
	IsDir   bool      `json:"isDir"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	ModTime time.Time `json:"modTime"`
}

// RemoteFS is the transport-neutral SFTP surface satisfied by *sftp.Client
// (production) and by the in-process test server.
type RemoteFS interface {
	ReadDir(dir string) ([]Entry, error)
	Stat(path string) (FileInfo, error)
	Mkdir(dir string) error
	Remove(path string) error
	Rename(oldPath, newPath string) error
	Open(path string) (io.ReadCloser, error)
	Create(path string) (io.WriteCloser, error)
}

// acquireWait bounds how long an operation queues while the session is
// saturated. The UI legitimately bursts past the limit (parallel multi-select
// deletes) and saturated sessions drain in well under a second for ordinary
// operations, so waiting converts those bursts into brief queues instead of
// user-visible failures; a hung transport still fails after the window.
const acquireWait = 10 * time.Second

// Session bounds concurrent SFTP operations per terminal session.
type Session struct {
	slots chan struct{}
	wait  time.Duration
}

// NewSession constructs a session with a bounded client limit.
func NewSession(maxClients int) *Session {
	return newSessionWithWait(maxClients, acquireWait)
}

func newSessionWithWait(maxClients int, wait time.Duration) *Session {
	if maxClients <= 0 {
		maxClients = 4
	}
	return &Session{slots: make(chan struct{}, maxClients), wait: wait}
}

// Acquire reserves an SFTP client slot, queueing up to the session wait while
// the limit is saturated. The caller must invoke the returned release exactly
// once.
func (s *Session) Acquire() (func(), error) {
	timer := time.NewTimer(s.wait)
	defer timer.Stop()
	select {
	case s.slots <- struct{}{}:
		return func() { <-s.slots }, nil
	case <-timer.C:
		return nil, ErrTooManyClients
	}
}

// NormalizePath joins and cleans an SFTP path, refusing empty names and
// backslash separators at the lexical level (the server still enforces real
// access control).
func NormalizePath(base, relative string) (string, error) {
	if base == "" {
		return "", ErrPathRequired
	}
	if relative == "" {
		return base, nil
	}
	if strings.Contains(relative, "\\") {
		return "", fmt.Errorf("%w: %q", ErrNameInvalid, relative)
	}
	if path.IsAbs(relative) {
		if containsDotDot(relative) {
			return "", fmt.Errorf("%w: %q escapes the base", ErrNameInvalid, relative)
		}
		return path.Clean(relative), nil
	}
	cleaned := path.Clean(path.Join(base, relative))
	baseCleaned := path.Clean(base)
	if cleaned != baseCleaned && !strings.HasPrefix(cleaned, baseCleaned+"/") {
		return "", fmt.Errorf("%w: %q escapes the base", ErrNameInvalid, relative)
	}
	return cleaned, nil
}

func containsDotDot(value string) bool {
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

// SortEntries orders entries directories-first then by name (matches the
// renderer's SFTP ordering contract).
func SortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})
}
