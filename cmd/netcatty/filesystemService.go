package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/binaricat/netcatty/internal/platform/applog"
	"github.com/binaricat/netcatty/internal/platform/filesystem"
)

type FilesystemService struct{}

func newFilesystemService() *FilesystemService {
	return &FilesystemService{}
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

// StageFromLocalPath copies one local file into the LemonSSH staging temp
// directory in a single call (stat → open → copy with no renderer round trip
// in between), so transient drag sources and path quirks cannot race the
// upload. Returns the staged temp path and original size.
func (s *FilesystemService) StageFromLocalPath(path string) (string, int64, error) {
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
	staged, err := os.CreateTemp("", stagedUploadPrefix+base)
	if err != nil {
		return "", 0, err
	}
	written, copyErr := io.Copy(staged, reader)
	if closeErr := staged.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		_ = os.Remove(staged.Name())
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

const stagedUploadPrefix = "lemonssh-stage-"

// StageBegin creates a temp file for a renderer-staged upload and returns
// its path. The renderer streams chunks via StageAppend.
func (s *FilesystemService) StageBegin(fileName string) (string, error) {
	base := filepath.Base(fileName)
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "upload.bin"
	}
	file, err := os.CreateTemp("", stagedUploadPrefix+base)
	if err != nil {
		return "", err
	}
	if closeErr := file.Close(); closeErr != nil {
		return "", closeErr
	}
	return file.Name(), nil
}

// StageAppend writes one base64-decoded chunk at offset. []byte bindings
// arrive base64-encoded through the Wails transport.
func (s *FilesystemService) StageAppend(tempPath string, offset int64, data []byte) error {
	if !strings.Contains(filepath.Base(tempPath), stagedUploadPrefix) {
		return fmt.Errorf("staged path is not a LemonSSH staging file")
	}
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if offset > 0 {
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return err
		}
	}
	_, err = file.Write(data)
	return err
}

// StageDiscard removes a staged temp file. Only LemonSSH staging files are
// eligible so a renderer cannot delete arbitrary paths.
func (s *FilesystemService) StageDiscard(tempPath string) error {
	if !strings.Contains(filepath.Base(tempPath), stagedUploadPrefix) {
		return fmt.Errorf("staged path is not a LemonSSH staging file")
	}
	err := os.Remove(tempPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
