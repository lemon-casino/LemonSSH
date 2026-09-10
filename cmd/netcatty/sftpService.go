package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/binaricat/netcatty/internal/platform/applog"
	"github.com/binaricat/netcatty/internal/terminal/sftp"
	netcattyssh "github.com/binaricat/netcatty/internal/terminal/ssh"
	"github.com/binaricat/netcatty/internal/terminal/sshpool"
	pkgsftp "github.com/pkg/sftp"
)

// SFTPService exposes SFTP browsing and file transfer over the shared SSH
// transport pool. Each Open call borrows a pooled transport (KindSFTP), opens
// one SFTP subsystem client and registers it under an opaque session ID;
// operations are bounded by the per-session client limit.
type SFTPService struct {
	mu         sync.Mutex
	pool       *sshpool.Pool
	knownHosts *netcattyssh.KnownHosts
	sessions   map[string]*sftpClient
	counter    int
}

type sftpClient struct {
	fs      *sftp.ClientFS
	lease   *sshpool.Lease
	raw     *pkgsftp.Client
	bounded *sftp.Session
}

// NewSFTPService wires the pool and known-hosts store.
func NewSFTPService(pool *sshpool.Pool, knownHosts *netcattyssh.KnownHosts) *SFTPService {
	return &SFTPService{
		pool:       pool,
		knownHosts: knownHosts,
		sessions:   make(map[string]*sftpClient),
	}
}

// Open dials (or borrows) a transport for host and registers an SFTP session.
func (s *SFTPService) Open(request SSHConnectRequest) (string, error) {
	if request.Hostname == "" || request.Username == "" {
		return "", fmt.Errorf("host and username are required")
	}
	if request.Port == 0 {
		request.Port = 22
	}
	config, err := netcattyssh.BuildDialConfigErr(sshConnectToInput(request), netcattyssh.StrictPolicy(s.knownHosts), nil)
	if err != nil {
		return "", err
	}
	lease, err := s.pool.Get(context.Background(), config, sshpool.KindSFTP)
	if err != nil {
		return "", fmt.Errorf("ssh dial %s:%d: %w", request.Hostname, request.Port, err)
	}
	raw, err := pkgsftp.NewClient(lease.Client())
	if err != nil {
		lease.Discard()
		return "", fmt.Errorf("sftp subsystem: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter++
	id := fmt.Sprintf("sftp-%d", s.counter)
	s.sessions[id] = &sftpClient{
		fs:      sftp.NewClientFS(raw),
		lease:   lease,
		raw:     raw,
		bounded: sftp.NewSession(4),
	}
	return id, nil
}

// List returns one directory listing (directories first, name-ordered).
func (s *SFTPService) List(sessionID, dir string) ([]sftp.Entry, error) {
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return nil, err
	}
	defer done()
	target, err := sftp.NormalizePath(".", dir)
	if err != nil {
		return nil, err
	}
	entries, err := client.fs.ReadDir(target)
	sftp.SortEntries(entries)
	return entries, err
}

// Stat stats one remote path.
func (s *SFTPService) Stat(sessionID, target string) (sftp.FileInfo, error) {
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return sftp.FileInfo{}, err
	}
	defer done()
	resolved, err := sftp.NormalizePath(".", target)
	if err != nil {
		return sftp.FileInfo{}, err
	}
	return client.fs.Stat(resolved)
}

// Mkdir creates a remote directory.
func (s *SFTPService) Mkdir(sessionID, dir string) error {
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return err
	}
	defer done()
	target, err := sftp.NormalizePath(".", dir)
	if err != nil {
		return err
	}
	return client.fs.Mkdir(target)
}

// Remove deletes a remote file or directory tree.
func (s *SFTPService) Remove(sessionID, target string) error {
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return err
	}
	defer done()
	resolved, err := sftp.NormalizePath(".", target)
	if err != nil {
		return err
	}
	return client.fs.Remove(resolved)
}

// Rename moves or renames a remote path.
func (s *SFTPService) Rename(sessionID, oldPath, newPath string) error {
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return err
	}
	defer done()
	from, err := sftp.NormalizePath(".", oldPath)
	if err != nil {
		return err
	}
	to, err := sftp.NormalizePath(".", newPath)
	if err != nil {
		return err
	}
	return client.fs.Rename(from, to)
}

// Read returns a remote file as UTF-8 text.
func (s *SFTPService) Read(sessionID, remotePath string) (string, error) {
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return "", err
	}
	defer done()
	resolved, err := sftp.NormalizePath(".", remotePath)
	if err != nil {
		return "", err
	}
	reader, err := client.fs.Open(resolved)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteText writes UTF-8 text to a remote file, creating or truncating it.
func (s *SFTPService) WriteText(sessionID, remotePath, content string) error {
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return err
	}
	defer done()
	resolved, err := sftp.NormalizePath(".", remotePath)
	if err != nil {
		return err
	}
	writer, err := client.fs.Create(resolved)
	if err != nil {
		return err
	}
	defer writer.Close()
	_, err = io.WriteString(writer, content)
	return err
}

// HomeDir returns the remote working directory for the SFTP session.
func (s *SFTPService) HomeDir(sessionID string) (string, error) {
	s.mu.Lock()
	client, ok := s.sessions[sessionID]
	s.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("sftp session %q not found", sessionID)
	}
	return client.raw.Getwd()
}

// Download streams a remote file to a local destination path.
func (s *SFTPService) Download(sessionID, remotePath, localPath string) (int64, error) {
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
func (s *SFTPService) Upload(sessionID, localPath, remotePath string) (int64, error) {
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
		reader, err := openLocalForUpload(localPath)
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

// openLocalForUpload opens the local source, retrying once after a short
// delay because transient drag sources race the upload pipeline.
func openLocalForUpload(localPath string) (*os.File, error) {
	reader, err := os.Open(localPath)
	if err == nil {
		return reader, nil
	}
	time.Sleep(400 * time.Millisecond)
	reader, retryErr := os.Open(localPath)
	if retryErr != nil {
		// %q exposes invisible characters that a plain %s path hides.
		return nil, err
	}
	return reader, nil
}

// ExtractArchive downloads a remote zip, extracts it locally with zip-slip
// protection, and uploads the files next to the archive.
func (s *SFTPService) ExtractArchive(sessionID, remotePath string) (int, error) {
	tempDir, err := os.MkdirTemp("", "lemonssh-extract-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(tempDir)
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
func (s *SFTPService) UploadCompressedFolder(sessionID, localFolder, remoteZipPath string) (int64, error) {
	temp, err := os.CreateTemp("", "lemonssh-upload-*.zip")
	if err != nil {
		return 0, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
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

// Close releases the SFTP client and returns the transport to the pool.
func (s *SFTPService) Close(sessionID string) error {
	s.mu.Lock()
	client, ok := s.sessions[sessionID]
	delete(s.sessions, sessionID)
	s.mu.Unlock()
	if !ok {
		return nil
	}
	_ = client.raw.Close()
	client.lease.Return()
	return nil
}

func (s *SFTPService) acquire(sessionID string) (*sftpClient, func(), error) {
	s.mu.Lock()
	client, ok := s.sessions[sessionID]
	s.mu.Unlock()
	if !ok {
		return nil, nil, fmt.Errorf("sftp session %q not found", sessionID)
	}
	release, err := client.bounded.Acquire()
	if err != nil {
		return nil, nil, err
	}
	return client, release, nil
}
