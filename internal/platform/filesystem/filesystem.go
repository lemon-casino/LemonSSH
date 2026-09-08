// Package filesystem owns local filesystem operations for the Go runtime
// (P4-01, SYS-01): the dedicated temp directory service, symlink-safe path
// resolution within managed roots, and archive extraction hardened against
// zip-slip. Native dialogs and platform UI stay in the Wails adapter; this
// package is headless and fully testable.
package filesystem

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

var (
	ErrPathEscapesRoot   = errors.New("filesystem: path escapes managed root")
	ErrTempRootMissing   = errors.New("filesystem: temp root missing")
	ErrEntryStillExists  = errors.New("filesystem: entry still exists after delete")
	ErrArchiveTooLarge   = errors.New("filesystem: archive exceeds total size cap")
	ErrPathRequired      = errors.New("filesystem: path is required")
	ErrUnsafeArchiveName = errors.New("filesystem: archive entry name unsafe")
)

// MaxArchiveBytes bounds the uncompressed total of one archive (1 GiB).
const MaxArchiveBytes = 1 << 30

// TempService manages the Netcatty dedicated temp directory. Everything the
// app writes temporarily must live under this root (AGENTS.md contract) so
// Settings > System can show and clear it.
type TempService struct {
	mu   sync.Mutex
	root string
}

// NewTempService creates the dedicated temp root (Netcatty temp dir) if
// missing and returns the service.
func NewTempService(root string) (*TempService, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrTempRootMissing
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("temp root create: %w", err)
	}
	return &TempService{root: root}, nil
}

// Root reports the dedicated temp root.
func (t *TempService) Root() string { return t.root }

// FilePath resolves one managed file under the temp root, refusing escapes.
func (t *TempService) FilePath(name string) (string, error) {
	return resolveUnderRoot(t.root, name)
}

// Mkdir makes a directory under the temp root.
func (t *TempService) Mkdir(name string) error {
	target, err := t.FilePath(name)
	if err != nil {
		return err
	}
	return os.MkdirAll(target, 0o700)
}

// WriteFile writes content to a managed temp file (0600).
func (t *TempService) WriteFile(name string, data []byte) error {
	target, err := t.FilePath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o600)
}

// ReadFile reads a managed temp file.
func (t *TempService) ReadFile(name string) ([]byte, error) {
	target, err := t.FilePath(name)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(target)
}

// Remove deletes one managed entry and verifies it is actually gone.
func (t *TempService) Remove(name string) error {
	target, err := t.FilePath(name)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		return ErrEntryStillExists
	}
	return nil
}

// Clear removes everything under the temp root but keeps the root itself.
func (t *TempService) Clear() error {
	entries, err := os.ReadDir(t.root)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrTempRootMissing
		}
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(t.root, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// Usage reports the number of entries and total bytes under the temp root
// (Settings > System display).
func (t *TempService) Usage(ctx context.Context) (entries int64, bytes int64, err error) {
	err = filepath.WalkDir(t.root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if path == t.root {
			return nil
		}
		entries++
		if !d.IsDir() {
			info, infoErr := d.Info()
			if infoErr == nil {
				bytes += info.Size()
			}
		}
		return nil
	})
	return entries, bytes, err
}

// resolveUnderRoot joins base+name and refuses any lexical or symlink escape.
func resolveUnderRoot(base, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", ErrPathRequired
	}
	// Lexical check first: reject absolute names and parent traversal.
	if filepath.IsAbs(name) || strings.HasPrefix(name, `\\`) || strings.Contains(name, ":") && filepath.VolumeName(name) != "" {
		return "", fmt.Errorf("%w: %q", ErrPathEscapesRoot, name)
	}
	joined := filepath.Join(base, filepath.FromSlash(name))
	relative, err := filepath.Rel(base, joined)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: %q", ErrPathEscapesRoot, name)
	}
	// Symlink check: every existing component must resolve inside base.
	probe := base
	for _, segment := range strings.Split(relative, string(os.PathSeparator)) {
		if segment == "" || segment == "." {
			continue
		}
		probe = filepath.Join(probe, segment)
		info, err := os.Lstat(probe)
		if err != nil {
			if os.IsNotExist(err) {
				break // nothing more to verify for not-yet-created segments
			}
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, linkErr := filepath.EvalSymlinks(probe)
			if linkErr != nil {
				// Broken symlink: allow only if it is the final component and
				// the caller intends to create/replace it.
				if probe == joined {
					break
				}
				return "", fmt.Errorf("%w: broken symlink %q", ErrPathEscapesRoot, segment)
			}
			if !withinBase(base, resolved) {
				return "", fmt.Errorf("%w: symlink %q", ErrPathEscapesRoot, segment)
			}
		}
	}
	return joined, nil
}

func withinBase(base, target string) bool {
	relative, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return relative == "." || (!strings.HasPrefix(relative, "..") && !filepath.IsAbs(relative))
}

// ExtractArchive extracts a zip archive into destinationRoot with zip-slip
// protection and a total uncompressed size cap. Pre-existing files at entry
// paths are overwritten; traversal entries abort the whole extraction.
func ExtractArchive(archivePath, destinationRoot string) (extracted int, err error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return 0, err
	}
	defer reader.Close()

	var total int64
	for _, file := range reader.File {
		total += int64(file.UncompressedSize64)
		if total > MaxArchiveBytes {
			return extracted, ErrArchiveTooLarge
		}
	}
	for _, file := range reader.File {
		cleanName := path.Clean(file.Name)
		if strings.HasPrefix(cleanName, "../") || path.IsAbs(cleanName) || strings.Contains(cleanName, "..\\") {
			return extracted, fmt.Errorf("%w: %q", ErrUnsafeArchiveName, file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filepath.Join(destinationRoot, filepath.FromSlash(cleanName)), 0o700); err != nil {
				return extracted, err
			}
			continue
		}
		if err := extractFile(file, destinationRoot); err != nil {
			return extracted, err
		}
		extracted++
	}
	return extracted, nil
}

func extractFile(file *zip.File, destinationRoot string) error {
	target := filepath.Join(destinationRoot, filepath.FromSlash(path.Clean(file.Name)))
	if !withinBase(destinationRoot, target) {
		return fmt.Errorf("%w: %q", ErrUnsafeArchiveName, file.Name)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	source, err := file.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	destination, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer destination.Close()
	_, err = io.Copy(destination, source)
	return err
}
