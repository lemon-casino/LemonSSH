// Package filesystem owns local filesystem operations for the Go runtime
// (P4-01, SYS-01): the dedicated temp directory service, symlink-safe path
// resolution within managed roots, and archive extraction hardened against
// zip-slip. Native dialogs and platform UI stay in the Wails adapter; this
// package is headless and fully testable.
package filesystem

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
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
	ErrNotStagedFile     = errors.New("filesystem: path is not an owned staging file")
)

// MaxArchiveBytes bounds the uncompressed total of one archive (1 GiB).
const MaxArchiveBytes = 1 << 30

const (
	StagedUploadPrefix = "lemonssh-stage-"
	TransferTempPrefix = "active-transfer-"
)

// TempService manages the Netcatty dedicated temp directory. Everything the
// app writes temporarily must live under this root (AGENTS.md contract) so
// Settings > System can show and clear it.
type TempService struct {
	mu     sync.Mutex
	root   string
	active map[string]*tempEntry
}

type tempEntry struct {
	path   string
	owned  bool
	users  int
	remove bool
	staged os.FileInfo
}

// NewTempService creates the dedicated temp root (Netcatty temp dir) if
// missing and returns the service.
func NewTempService(root string) (*TempService, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrTempRootMissing
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("temp root create: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	return &TempService{root: root, active: make(map[string]*tempEntry)}, nil
}

// Root reports the dedicated temp root.
func (t *TempService) Root() string { return t.root }

// FilePath resolves one managed file under the temp root, refusing escapes.
func (t *TempService) FilePath(name string) (string, error) {
	return resolveUnderRoot(t.root, name)
}

func tempKey(name string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(name)
	}
	return name
}

// ReserveFilePath leases a unique external-download path without creating a
// placeholder. Release keeps the downloaded file; Remove deletes it.
func (t *TempService) ReserveFilePath(name string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	target, err := t.FilePath(rand.Text() + "_" + filepath.Base(name))
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		return "", fmt.Errorf("temp reservation already exists or cannot be inspected: %s", target)
	}
	key := tempKey(filepath.Base(target))
	if t.active[key] != nil {
		return "", fmt.Errorf("temp path is already reserved")
	}
	t.active[key] = &tempEntry{path: target, owned: true}
	return target, nil
}

// CreateFile and CreateDir allocate and lease while holding the cleanup lock.
// Callers close file handles before Remove; Release instead keeps the artifact.
func (t *TempService) CreateFile(pattern string) (*os.File, error) {
	return t.createFile(pattern, false)
}

func (t *TempService) CreateStagingFile(name string) (*os.File, error) {
	return t.createFile(StagedUploadPrefix+filepath.Base(name), true)
}

func (t *TempService) createFile(pattern string, staged bool) (*os.File, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	file, err := os.CreateTemp(t.root, pattern)
	if err != nil {
		return nil, err
	}
	entry := &tempEntry{path: file.Name(), owned: true}
	if staged {
		entry.staged, err = file.Stat()
		if err != nil {
			_ = file.Close()
			_ = os.Remove(file.Name())
			return nil, err
		}
	}
	t.active[tempKey(filepath.Base(file.Name()))] = entry
	return file, nil
}

func (t *TempService) CreateDir(pattern string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	dir, err := os.MkdirTemp(t.root, pattern)
	if err != nil {
		return "", err
	}
	t.active[tempKey(filepath.Base(dir))] = &tempEntry{path: dir, owned: true}
	return dir, nil
}

// entryPath accepts only contained paths without symlink aliases, so a pin can
// never protect a link while cleanup deletes its target under a different key.
func (t *TempService) entryPath(filePath string) (target, key string, err error) {
	if !filepath.IsAbs(filePath) {
		return "", "", ErrPathEscapesRoot
	}
	rel, err := filepath.Rel(t.root, filePath)
	if err != nil || rel == "." {
		return "", "", ErrPathEscapesRoot
	}
	target, err = t.FilePath(rel)
	if err != nil {
		return "", "", err
	}
	segments := strings.Split(rel, string(filepath.Separator))
	probe := t.root
	for _, segment := range segments {
		probe = filepath.Join(probe, segment)
		info, statErr := os.Lstat(probe)
		if os.IsNotExist(statErr) {
			break
		}
		if statErr != nil {
			return "", "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", "", ErrPathEscapesRoot
		}
	}
	return target, tempKey(segments[0]), nil
}

// Acquire pins a local I/O path until the returned function is called, after
// closing its handles. Paths outside this root need no pin. Pins survive owner
// release/discard and have no age limit, including while a transfer is paused.
func (t *TempService) Acquire(filePath string) (func(), error) {
	if t == nil {
		return func() {}, nil
	}
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return nil, err
	}
	if !withinBase(t.root, abs) {
		return func() {}, nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	_, key, err := t.entryPath(abs)
	if err != nil {
		return nil, err
	}
	entry := t.active[key]
	if entry == nil {
		rel, _ := filepath.Rel(t.root, abs)
		entry = &tempEntry{path: filepath.Join(t.root, strings.Split(rel, string(filepath.Separator))[0])}
		t.active[key] = entry
	}
	if entry.remove {
		return nil, fmt.Errorf("temp entry is pending removal")
	}
	entry.users++
	return sync.OnceFunc(func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		entry.users--
		_ = t.finishEntry(key, entry)
	}), nil
}

func (t *TempService) finishEntry(key string, entry *tempEntry) error {
	if entry.owned || entry.users != 0 {
		return nil
	}
	delete(t.active, key)
	if entry.remove {
		return os.RemoveAll(entry.path)
	}
	return nil
}

// Release ends an allocation's ownership without deleting its contents. It is
// idempotent; active I/O pins remain until their handles close.
func (t *TempService) Release(filePath string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	target, key, err := t.entryPath(filePath)
	if err != nil || filepath.Dir(target) != t.root {
		return ErrPathEscapesRoot
	}
	if entry := t.active[key]; entry != nil {
		entry.owned = false
		return t.finishEntry(key, entry)
	}
	return nil
}

func (t *TempService) stagingEntry(filePath string) (string, *tempEntry, error) {
	target, key, err := t.entryPath(filePath)
	if err != nil || filepath.Dir(target) != t.root || !strings.HasPrefix(filepath.Base(target), StagedUploadPrefix) {
		return "", nil, ErrNotStagedFile
	}
	entry := t.active[key]
	if entry == nil || !entry.owned || entry.staged == nil {
		return "", nil, ErrNotStagedFile
	}
	info, err := os.Lstat(target)
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(info, entry.staged) {
		return "", nil, ErrNotStagedFile
	}
	return key, entry, nil
}

func (t *TempService) AppendStaging(filePath string, offset int64, data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, entry, err := t.stagingEntry(filePath)
	if err != nil {
		return err
	}
	if offset < 0 {
		return fmt.Errorf("negative staging offset")
	}
	file, err := os.OpenFile(entry.path, os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = file.WriteAt(data, offset)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func (t *TempService) DiscardStaging(filePath string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	key, entry, err := t.stagingEntry(filePath)
	if err != nil {
		return err
	}
	entry.owned, entry.remove = false, true
	return t.finishEntry(key, entry)
}

// Mkdir makes a directory under the temp root.
func (t *TempService) Mkdir(name string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	target, err := t.FilePath(name)
	if err != nil {
		return err
	}
	return os.MkdirAll(target, 0o700)
}

// WriteFile writes content to a managed temp file (0600).
func (t *TempService) WriteFile(name string, data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
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
	t.mu.Lock()
	defer t.mu.Unlock()
	target, err := t.FilePath(name)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(target)
}

// Remove releases ownership and deletes a managed entry. Active I/O defers
// deletion until its last pin is released.
func (t *TempService) Remove(name string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	target, err := t.FilePath(name)
	if err != nil {
		return err
	}
	if target == t.root {
		return ErrPathEscapesRoot
	}
	rel, _ := filepath.Rel(t.root, target)
	key := tempKey(strings.Split(rel, string(filepath.Separator))[0])
	if entry := t.active[key]; entry != nil {
		if tempKey(target) != tempKey(entry.path) {
			return fmt.Errorf("temp entry is in use")
		}
		entry.owned, entry.remove = false, true
		return t.finishEntry(key, entry)
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		return ErrEntryStillExists
	}
	return nil
}

// Clear removes unleased entries but keeps the root itself.
func (t *TempService) Clear() error {
	_, err := t.ClearInactive()
	return err
}

func (t *TempService) ClearInactive() (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.clear(false)
}

// CleanupOrphans is startup cleanup for the canonical profile writer. A new
// process has no leases; only known staging prefixes are removed, preserving
// external-edit downloads. Reusing this instance also protects active work.
func (t *TempService) CleanupOrphans() (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.clear(true)
}

func (t *TempService) clear(stagingOnly bool) (int, error) {
	entries, err := os.ReadDir(t.root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, ErrTempRootMissing
		}
		return 0, err
	}
	deleted := 0
	for _, entry := range entries {
		if t.active[tempKey(entry.Name())] != nil {
			continue
		}
		if stagingOnly && !strings.HasPrefix(entry.Name(), StagedUploadPrefix) && !strings.HasPrefix(entry.Name(), TransferTempPrefix) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(t.root, entry.Name())); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
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
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
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
