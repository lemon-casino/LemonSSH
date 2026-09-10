package main

import "github.com/binaricat/netcatty/internal/platform/filesystem"

type FilesystemService struct{}

func newFilesystemService() *FilesystemService {
	return &FilesystemService{}
}

func (s *FilesystemService) ExtractArchive(archivePath, destinationRoot string) (int, error) {
	return filesystem.ExtractArchive(archivePath, destinationRoot)
}
