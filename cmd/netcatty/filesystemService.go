package main

import (
	"os"

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
		return LocalPathStat{}, err
	}
	return LocalPathStat{
		Name:  info.Name(),
		IsDir: info.IsDir(),
		Size:  info.Size(),
	}, nil
}
