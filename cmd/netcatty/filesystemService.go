package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/binaricat/netcatty/internal/platform/applog"
	"github.com/binaricat/netcatty/internal/platform/filesystem"
)

type FilesystemService struct{ temp *filesystem.TempService }

func (s *FilesystemService) setTempService(temp *filesystem.TempService) { s.temp = temp }

type TempDirectoryInfo struct {
	Path      string `json:"path"`
	FileCount int64  `json:"fileCount"`
	TotalSize int64  `json:"totalSize"`
}
type TempClearResult struct {
	Success      bool `json:"success"`
	DeletedCount int  `json:"deletedCount"`
}

func (s *FilesystemService) TempInfo() (TempDirectoryInfo, error) {
	if s.temp == nil {
		return TempDirectoryInfo{}, fmt.Errorf("managed temp unavailable")
	}
	count, size, err := s.temp.Usage(context.Background())
	return TempDirectoryInfo{Path: s.temp.Root(), FileCount: count, TotalSize: size}, err
}
func (s *FilesystemService) TempFilePath(name string) (string, error) {
	if s.temp == nil {
		return "", fmt.Errorf("managed temp unavailable")
	}
	return s.temp.ReserveFilePath(name)
}

// ReleaseTempFile ends renderer ownership without removing an external-edit
// download. Call after the last consumer finishes; transfer I/O has its own pin.
func (s *FilesystemService) ReleaseTempFile(filePath string) error {
	if s.temp == nil {
		return fmt.Errorf("managed temp unavailable")
	}
	return s.temp.Release(filePath)
}
func (s *FilesystemService) ClearTemp() (TempClearResult, error) {
	if s.temp == nil {
		return TempClearResult{}, fmt.Errorf("managed temp unavailable")
	}
	deleted, err := s.temp.ClearInactive()
	return TempClearResult{Success: err == nil, DeletedCount: deleted}, err
}

func (s *FilesystemService) managedTempPath(filePath string) (string, error) {
	if s.temp == nil {
		return "", fmt.Errorf("managed temp unavailable")
	}
	rel, err := filepath.Rel(s.temp.Root(), filePath)
	if err != nil || rel == "." {
		return "", fmt.Errorf("invalid managed temp file")
	}
	return s.temp.FilePath(rel)
}

func (s *FilesystemService) ValidateTempFile(filePath string) error {
	target, err := s.managedTempPath(filePath)
	if err != nil {
		return err
	}
	return validateOpenFile(target)
}

func (s *FilesystemService) DeleteTempFile(filePath string) error {
	target, err := s.managedTempPath(filePath)
	if err != nil {
		return err
	}
	info, err := os.Lstat(target)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("temp path is not a regular file")
	}
	rel, _ := filepath.Rel(s.temp.Root(), target)
	return s.temp.Remove(rel)
}

func validateOpenFile(filePath string) error {
	if !filepath.IsAbs(filePath) {
		return fmt.Errorf("absolute file path required")
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("path is not a regular file")
	}
	return nil
}

func (s *FilesystemService) OpenWithSystemDefault(filePath string) error {
	if err := validateOpenFile(filePath); err != nil {
		return err
	}
	return openSystemFile(filePath)
}

func (s *FilesystemService) OpenWithApplication(filePath, appPath string) error {
	if err := validateOpenFile(filePath); err != nil {
		return err
	}
	if runtime.GOOS == "darwin" && strings.HasSuffix(appPath, ".app") {
		if !filepath.IsAbs(appPath) {
			return fmt.Errorf("absolute application path required")
		}
		info, err := os.Stat(appPath)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("invalid application bundle")
		}
		return startFileApplication(exec.Command("open", "-a", appPath, "--", filePath))
	}
	if err := validateOpenFile(appPath); err != nil {
		return err
	}
	return startFileApplication(exec.Command(appPath, filePath))
}

func startFileApplication(cmd *exec.Cmd) error {
	configureFileOpenProcess(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func newFilesystemService() *FilesystemService {
	return &FilesystemService{}
}

func (s *FilesystemService) HomeDir() (string, error) {
	return os.UserHomeDir()
}

func (s *FilesystemService) ListDir(path string) ([]filesystem.LocalEntry, error) {
	return filesystem.ListDirectory(path)
}

func (s *FilesystemService) ExtractArchive(archivePath, destinationRoot string) (int, error) {
	return filesystem.ExtractArchive(archivePath, destinationRoot)
}

// LocalPathStat is the os.Stat view of one local path.
type LocalPathStat struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

// LocalFileInfo is the complete metadata shape used by the renderer's local
// file operations. Lstat preserves symlink identity while Stat follows it.
type LocalFileInfo struct {
	Path      string    `json:"path"`
	IsDir     bool      `json:"isDir"`
	IsSymlink bool      `json:"isSymlink"`
	Size      int64     `json:"size"`
	Mode      string    `json:"mode"`
	ModTime   time.Time `json:"modTime"`
}

type LocalSystemInfo struct {
	Username string `json:"username"`
	Hostname string `json:"hostname"`
}

// StatPath stats one local path (read-only) so dropped files can be
// classified before upload.
func (s *FilesystemService) StatPath(path string) (LocalPathStat, error) {
	info, err := os.Stat(path)
	if err != nil {
		// %q exposes invisible characters that a plain %s path hides.
		return LocalPathStat{}, fmt.Errorf("stat %q: %w", path, err)
	}
	return LocalPathStat{
		Name:  info.Name(),
		IsDir: info.IsDir(),
		Size:  info.Size(),
	}, nil
}

func localFileInfo(path string, info os.FileInfo) LocalFileInfo {
	return LocalFileInfo{
		Path:      path,
		IsDir:     info.IsDir(),
		IsSymlink: info.Mode()&os.ModeSymlink != 0,
		Size:      info.Size(),
		Mode:      info.Mode().String(),
		ModTime:   info.ModTime(),
	}
}

// ReadFile returns a bounded local file payload. A non-positive maxBytes uses
// the editor's existing 64 MiB desktop ceiling.
func (s *FilesystemService) ReadFile(path string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = 64 << 20
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path is not a regular file")
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("file exceeds maximum size of %d bytes", maxBytes)
	}
	return io.ReadAll(io.LimitReader(file, maxBytes+1))
}

// WriteFile writes renderer-provided bytes to a local file without following
// a final symlink. Parent directories must already exist.
func (s *FilesystemService) WriteFile(path string, data []byte) error {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to write through a symlink")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (s *FilesystemService) DeletePath(path, expectedType string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	actual := "file"
	if info.Mode()&os.ModeSymlink != 0 {
		actual = "symlink"
	} else if info.IsDir() {
		actual = "directory"
	}
	if expectedType != "" && expectedType != actual {
		return fmt.Errorf("path type changed: expected %s, got %s", expectedType, actual)
	}
	if actual == "directory" {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

func (s *FilesystemService) RenamePath(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (s *FilesystemService) Mkdir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func (s *FilesystemService) Stat(path string) (LocalFileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return LocalFileInfo{}, err
	}
	return localFileInfo(path, info), nil
}

func (s *FilesystemService) Lstat(path string) (LocalFileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return LocalFileInfo{}, err
	}
	return localFileInfo(path, info), nil
}

func (s *FilesystemService) ListDrives() []string {
	if runtime.GOOS != "windows" {
		return []string{"/"}
	}
	drives := make([]string, 0, 8)
	for letter := 'A'; letter <= 'Z'; letter++ {
		root := string(letter) + `:\`
		if _, err := os.Stat(root); err == nil {
			drives = append(drives, root)
		}
	}
	return drives
}

func (s *FilesystemService) SystemInfo() LocalSystemInfo {
	hostname, _ := os.Hostname()
	username := os.Getenv("USERNAME")
	if username == "" {
		username = os.Getenv("USER")
	}
	return LocalSystemInfo{Username: username, Hostname: hostname}
}

// StageFromLocalPath copies one local file into the LemonSSH staging temp
// directory in a single call (stat → open → copy with no renderer round trip
// in between), so transient drag sources and path quirks cannot race the
// upload. Returns the staged temp path and original size.
func (s *FilesystemService) StageFromLocalPath(path string) (string, int64, error) {
	release, err := s.temp.Acquire(path)
	if err != nil {
		return "", 0, err
	}
	defer release()
	info, err := os.Stat(path)
	if err != nil {
		applog.Errorf("stage stat failed path=%q err=%v", path, err)
		return "", 0, fmt.Errorf("stage stat %q: %w", path, err)
	}
	if info.IsDir() {
		return "", 0, fmt.Errorf("stage %q: is a directory", path)
	}
	reader, err := openStagingSource(path)
	if err != nil {
		applog.Errorf("stage open failed path=%q err=%v", path, err)
		return "", 0, fmt.Errorf("stage open %q: %w", path, err)
	}
	defer reader.Close()
	base := filepath.Base(path)
	if s.temp == nil {
		return "", 0, fmt.Errorf("managed temp unavailable")
	}
	staged, err := s.temp.CreateStagingFile(base)
	if err != nil {
		return "", 0, err
	}
	written, copyErr := io.Copy(staged, reader)
	if closeErr := staged.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		_ = s.temp.Remove(filepath.Base(staged.Name()))
		return "", 0, copyErr
	}
	applog.Infof("staged %q -> %q (%d bytes)", path, staged.Name(), written)
	return staged.Name(), info.Size(), nil
}

// openStagingSource opens the source with fallbacks: the plain path, the
// \\?\ extended-length form (bypasses Win32 path normalization and some
// filter-driver quirks), then one delayed retry for transient sources.
func openStagingSource(path string) (*os.File, error) {
	open := func(p string) (*os.File, error) {
		file, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		return file, nil
	}
	reader, firstErr := open(path)
	if firstErr == nil {
		return reader, nil
	}
	if abs, absErr := filepath.Abs(path); absErr == nil {
		if reader, err := open(`\\?\` + abs); err == nil {
			applog.Infof("stage open succeeded via extended-length prefix for %q", path)
			return reader, nil
		}
	}
	time.Sleep(300 * time.Millisecond)
	reader, retryErr := open(path)
	if retryErr == nil {
		applog.Infof("stage open succeeded on retry for %q", path)
		return reader, nil
	}
	return nil, firstErr
}

// StageBegin creates a temp file for a renderer-staged upload and returns
// its path. The renderer streams chunks via StageAppend.
func (s *FilesystemService) StageBegin(fileName string) (string, error) {
	base := filepath.Base(fileName)
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "upload.bin"
	}
	if s.temp == nil {
		return "", fmt.Errorf("managed temp unavailable")
	}
	file, err := s.temp.CreateStagingFile(base)
	if err != nil {
		return "", err
	}
	if closeErr := file.Close(); closeErr != nil {
		_ = s.temp.Remove(filepath.Base(file.Name()))
		return "", closeErr
	}
	return file.Name(), nil
}

// StageAppend writes one base64-decoded chunk at offset. []byte bindings
// arrive base64-encoded through the Wails transport.
func (s *FilesystemService) StageAppend(tempPath string, offset int64, data []byte) error {
	if s.temp == nil {
		return fmt.Errorf("managed temp unavailable")
	}
	return s.temp.AppendStaging(tempPath, offset, data)
}

// StageDiscard removes a staged temp file. Only LemonSSH staging files are
// eligible so a renderer cannot delete arbitrary paths.
func (s *FilesystemService) StageDiscard(tempPath string) error {
	if s.temp == nil {
		return fmt.Errorf("managed temp unavailable")
	}
	return s.temp.DiscardStaging(tempPath)
}
