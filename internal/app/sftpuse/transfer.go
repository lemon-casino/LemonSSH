package sftpuse

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/binaricat/lemonssh/internal/platform/applog"
	"github.com/binaricat/lemonssh/internal/platform/charset"
	"github.com/binaricat/lemonssh/internal/platform/filesystem"
	"github.com/binaricat/lemonssh/internal/terminal/sftp"
)

// Download streams a remote file to a local destination path.
func (s *Service) Download(sessionID, remotePath, localPath, encoding string) (int64, error) {
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
	resolved, err := sftp.NormalizePath(".", s.encodePath(sessionID, encoding, remotePath))
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
func (s *Service) Upload(sessionID, localPath, remotePath, encoding string) (int64, error) {
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
	resolved, err := sftp.NormalizePath(".", s.encodePath(sessionID, encoding, remotePath))
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
// protection, and uploads the files next to the archive. The remote path and
// archive entry names are interpreted with the session filename charset:
// legacy (e.g. GBK) names are decoded to UTF-8 locally and re-encoded on the
// way back up.
func (s *Service) ExtractArchive(sessionID, remotePath, encoding string) (int, error) {
	if s.temp == nil {
		return 0, fmt.Errorf("managed temp unavailable")
	}
	tempDir, err := s.temp.CreateDir(filesystem.TransferTempPrefix + "extract-")
	if err != nil {
		return 0, err
	}
	defer s.temp.Remove(filepath.Base(tempDir))
	localZip := filepath.Join(tempDir, "archive.zip")
	if _, err := s.Download(sessionID, remotePath, localZip, encoding); err != nil {
		return 0, err
	}
	outDir := filepath.Join(tempDir, "out")
	// Archive entry names carry the server's legacy charset; auto-probe like
	// a listing does and pin the session when a legacy charset is proven.
	probe := charset.Auto
	decodeName := func(raw string) string {
		decoded := charset.Decode(raw, probe)
		if detected, ok := charset.DetectListingEncoding([]string{raw}); ok {
			probe = detected
			s.pinEncoding(sessionID, detected)
		}
		return decoded
	}
	count, err := filesystem.ExtractArchiveDecoded(localZip, outDir, decodeName)
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
			return s.Mkdir(sessionID, remote, encoding)
		}
		_, err = s.Upload(sessionID, path, remote, encoding)
		return err
	}); walkErr != nil {
		return count, walkErr
	}
	return count, nil
}

// UploadCompressedFolder zips a local folder and uploads the archive.
func (s *Service) UploadCompressedFolder(sessionID, localFolder, remoteZipPath, encoding string) (int64, error) {
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
	return s.Upload(sessionID, tempPath, remoteZipPath, encoding)
}
