package sftpuse

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/binaricat/netcatty/internal/platform/applog"
	"github.com/binaricat/netcatty/internal/platform/filesystem"
	"github.com/binaricat/netcatty/internal/terminal/sftp"
)

// Download streams a remote file to a local destination path.
func (s *Service) Download(sessionID, remotePath, localPath string) (int64, error) {
	release, err := s.temp.Acquire(localPath)
	if err != nil {
		return 0, err
	}
	defer release()
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return 0, err
	}
	defer done()
	resolved, err := sftp.NormalizePath(".", remotePath)
	if err != nil {
		return 0, err
	}
	reader, err := client.fs.Open(resolved)
	if err != nil {
		return 0, err
	}
	defer reader.Close()
	if mkdirErr := os.MkdirAll(filepath.Dir(localPath), 0o755); mkdirErr != nil {
		return 0, mkdirErr
	}
	writer, err := os.Create(localPath)
	if err != nil {
		return 0, err
	}
	defer writer.Close()
	return io.Copy(writer, reader)
}

// Upload streams a local file to a remote destination path.
func (s *Service) Upload(sessionID, localPath, remotePath string) (int64, error) {
	release, err := s.temp.Acquire(localPath)
	if err != nil {
		return 0, err
	}
	defer release()
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return 0, err
	}
	defer done()
	resolved, err := sftp.NormalizePath(".", remotePath)
	if err != nil {
		return 0, err
	}
	// Transient drag sources (chat apps, browser download popups, archive
	// previews) can delete or rename the file between drop and read; give
	// the path one short retry before failing.
	reader, err := s.openLocal(localPath)
	if err != nil {
		applog.Errorf("sftp upload open failed path=%q err=%v", localPath, err)
		return 0, fmt.Errorf("upload open %q: %w", localPath, err)
	}
	defer reader.Close()
	writer, err := client.fs.Create(resolved)
	if err != nil {
		return 0, err
	}
	defer writer.Close()
	return io.Copy(writer, reader)
}

// openLocal opens the local upload source via the wired staging opener
// (SetStagingOpener); unwired services open the source directly.
func (s *Service) openLocal(localPath string) (*os.File, error) {
	if s.openStaging != nil {
		return s.openStaging(localPath)
	}
	return os.Open(localPath)
}

// ExtractArchive downloads a remote zip, extracts it locally with zip-slip
// protection, and uploads the files next to the archive.
func (s *Service) ExtractArchive(sessionID, remotePath string) (int, error) {
	if s.temp == nil {
		return 0, fmt.Errorf("managed temp unavailable")
	}
	tempDir, err := s.temp.CreateDir(filesystem.TransferTempPrefix + "extract-")
	if err != nil {
		return 0, err
	}
	defer s.temp.Remove(filepath.Base(tempDir))
	localZip := filepath.Join(tempDir, "archive.zip")
	if _, err := s.Download(sessionID, remotePath, localZip); err != nil {
		return 0, err
	}
	outDir := filepath.Join(tempDir, "out")
	count, err := sftp.ExtractZipArchive(localZip, outDir)
	if err != nil {
		return 0, err
	}
	parent := filepath.ToSlash(filepath.Dir(remotePath))
	if parent == "." || parent == "" {
		parent = "/"
	}
	if walkErr := filepath.Walk(outDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(outDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		remote := parent + "/" + filepath.ToSlash(rel)
		if info.IsDir() {
			return s.Mkdir(sessionID, remote)
		}
		_, err = s.Upload(sessionID, path, remote)
		return err
	}); walkErr != nil {
		return count, walkErr
	}
	return count, nil
}

// UploadCompressedFolder zips a local folder and uploads the archive.
func (s *Service) UploadCompressedFolder(sessionID, localFolder, remoteZipPath string) (int64, error) {
	if s.temp == nil {
		return 0, fmt.Errorf("managed temp unavailable")
	}
	temp, err := s.temp.CreateFile(filesystem.TransferTempPrefix + "upload-*.zip")
	if err != nil {
		return 0, err
	}
	tempPath := temp.Name()
	defer s.temp.Remove(filepath.Base(tempPath))
	zipWriter := zip.NewWriter(temp)
	err = filepath.Walk(localFolder, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(localFolder, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		name := filepath.ToSlash(rel)
		if info.IsDir() {
			_, err := zipWriter.Create(name + "/")
			return err
		}
		writer, err := zipWriter.Create(name)
		if err != nil {
			return err
		}
		reader, err := os.Open(path)
		if err != nil {
			return err
		}
		defer reader.Close()
		_, err = io.Copy(writer, reader)
		return err
	})
	if err != nil {
		_ = zipWriter.Close()
		_ = temp.Close()
		return 0, err
	}
	if err := zipWriter.Close(); err != nil {
		_ = temp.Close()
		return 0, err
	}
	if err := temp.Close(); err != nil {
		return 0, err
	}
	return s.Upload(sessionID, tempPath, remoteZipPath)
}
