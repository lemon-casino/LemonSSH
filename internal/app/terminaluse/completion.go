package terminaluse

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	pkgsftp "github.com/pkg/sftp"
)

// AutocompleteDirectoryEntry is one filtered directory listing row.
type AutocompleteDirectoryEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// AutocompleteDirectoryResult is the directory completion payload.
type AutocompleteDirectoryResult struct {
	Success bool                         `json:"success"`
	Entries []AutocompleteDirectoryEntry `json:"entries"`
	Error   string                       `json:"error,omitempty"`
}

// ListAutocompleteDirectory uses a separate SFTP channel on the terminal's
// authenticated transport for remote sessions, and never writes to the
// interactive shell. Local PTY sessions list the local filesystem.
func (s *Service) ListAutocompleteDirectory(ctx context.Context, sessionID, directory string, foldersOnly bool, prefix string, limit int) AutocompleteDirectoryResult {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	failure := func(err error) AutocompleteDirectoryResult {
		return AutocompleteDirectoryResult{Entries: []AutocompleteDirectoryEntry{}, Error: err.Error()}
	}
	if err := ctx.Err(); err != nil {
		return failure(err)
	}
	if directory == "" || strings.ContainsAny(directory, "\x00\r\n\x1b") {
		return failure(fmt.Errorf("invalid directory"))
	}
	s.mu.Lock()
	term := s.sessions[sessionID]
	cwd := ""
	if term != nil {
		if term.completionQueries >= 2 {
			s.mu.Unlock()
			return failure(fmt.Errorf("terminal completion busy"))
		}
		term.completionQueries++
		cwd = term.cwd.cwd
	}
	s.mu.Unlock()
	if sessionID != "" && term == nil {
		return failure(fmt.Errorf("terminal session is not connected"))
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	type outcome struct {
		entries []AutocompleteDirectoryEntry
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		if term != nil {
			defer func() { s.mu.Lock(); term.completionQueries--; s.mu.Unlock() }()
		}
		var readDir func(string) ([]os.FileInfo, error)
		var stat func(string) (os.FileInfo, error)
		join := filepath.Join
		isAbs := filepath.IsAbs
		home := ""
		remote := term != nil && term.transport != nil
		if remote {
			channel, err := term.transport.Client.NewSession()
			if err != nil {
				done <- outcome{err: err}
				return
			}
			defer channel.Close()
			if ctx.Err() != nil {
				return
			}
			stopped := make(chan struct{})
			defer close(stopped)
			go func() {
				select {
				case <-ctx.Done():
					_ = channel.Close()
				case <-stopped:
				}
			}()
			reader, err := channel.StdoutPipe()
			if err != nil {
				done <- outcome{err: err}
				return
			}
			writer, err := channel.StdinPipe()
			if err != nil {
				done <- outcome{err: err}
				return
			}
			if err = channel.RequestSubsystem("sftp"); err != nil {
				done <- outcome{err: err}
				return
			}
			client, err := pkgsftp.NewClientPipe(reader, writer)
			if err != nil {
				done <- outcome{err: err}
				return
			}
			defer client.Close()
			readDir = func(p string) ([]os.FileInfo, error) { return client.ReadDirContext(ctx, p) }
			stat = client.Stat
			join = path.Join
			isAbs = path.IsAbs
			if directory == "~" || strings.HasPrefix(directory, "~/") {
				home, err = client.RealPath(".")
				if err != nil {
					done <- outcome{err: err}
					return
				}
			}
			if !isAbs(directory) && directory != "~" && !strings.HasPrefix(directory, "~/") && cwd == "" {
				pwd := s.GetSessionPwd(sessionID, TerminalPwdOptions{TimeoutMs: 1500})
				if !pwd.Success {
					done <- outcome{err: fmt.Errorf("terminal cwd unavailable")}
					return
				}
				cwd = pwd.Cwd
			}
		} else {
			if term != nil && (term.telnet != nil || term.serial != nil || term.runner != nil || term.helperState.Kind != "") {
				done <- outcome{err: fmt.Errorf("terminal does not support directory completion")}
				return
			}
			home, _ = os.UserHomeDir()
			readDir = func(p string) ([]os.FileInfo, error) {
				entries, err := os.ReadDir(p)
				if err != nil {
					return nil, err
				}
				result := make([]os.FileInfo, 0, len(entries))
				for _, entry := range entries {
					info, err := entry.Info()
					if err == nil {
						result = append(result, info)
					}
				}
				return result, nil
			}
			stat = os.Stat
		}
		lookup := directory
		if lookup == "~" || strings.HasPrefix(lookup, "~/") {
			if home == "" {
				done <- outcome{err: fmt.Errorf("home unavailable")}
				return
			}
			lookup = join(home, strings.TrimPrefix(strings.TrimPrefix(lookup, "~"), "/"))
		}
		if !isAbs(lookup) {
			if cwd == "" || !isAbs(cwd) {
				done <- outcome{err: fmt.Errorf("terminal cwd unavailable")}
				return
			}
			lookup = join(cwd, lookup)
		}
		entries, err := readDir(lookup)
		if err != nil {
			done <- outcome{err: err}
			return
		}
		result := make([]AutocompleteDirectoryEntry, 0)
		for _, entry := range entries {
			if ctx.Err() != nil {
				return
			}
			name := entry.Name()
			if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) || strings.ContainsAny(name, "\x00\r\n\x1b\t") {
				continue
			}
			kind := "file"
			if entry.IsDir() {
				kind = "directory"
			} else if entry.Mode()&os.ModeSymlink != 0 {
				kind = "symlink"
				if target, err := stat(join(lookup, name)); err == nil && target.IsDir() {
					kind = "directory"
				}
			}
			if foldersOnly && kind != "directory" {
				continue
			}
			result = append(result, AutocompleteDirectoryEntry{Name: name, Type: kind})
			if len(result) >= limit {
				break
			}
		}
		done <- outcome{entries: result}
	}()
	select {
	case <-ctx.Done():
		return failure(ctx.Err())
	case result := <-done:
		if err := ctx.Err(); err != nil {
			return failure(err)
		}
		if result.err != nil {
			return failure(result.err)
		}
		s.mu.Lock()
		current := s.sessions[sessionID]
		s.mu.Unlock()
		if sessionID != "" && current != term {
			return failure(fmt.Errorf("terminal session changed"))
		}
		return AutocompleteDirectoryResult{Success: true, Entries: result.entries}
	}
}
