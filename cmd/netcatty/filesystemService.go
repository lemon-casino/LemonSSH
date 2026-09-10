package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
