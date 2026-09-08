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
	"sync"
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

// Session bounds concurrent SFTP clients per terminal session.
type Session struct {
	mu      sync.Mutex
	max     int
	clients int
}

// NewSession constructs a session with a bounded client limit.
func NewSession(maxClients int) *Session {
	if maxClients <= 0 {
		maxClients = 4
	}
	return &Session{max: maxClients}
}

// Acquire reserves an SFTP client slot. The caller must invoke the returned
// release exactly once.
func (s *Session) Acquire() (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients >= s.max {
		return nil, ErrTooManyClients
	}
	s.clients++
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.clients--
	}, nil
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
	cleaned := path.Clean(path.Join(base, relative))
	baseCleaned := path.Clean(base)
	if cleaned != baseCleaned && !strings.HasPrefix(cleaned, baseCleaned+"/") {
		return "", fmt.Errorf("%w: %q escapes the base", ErrNameInvalid, relative)
	}
	return cleaned, nil
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
