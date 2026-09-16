package main

import (
	"os"

	pkgsftp "github.com/pkg/sftp"

	"github.com/binaricat/netcatty/internal/app/sftpuse"
	"github.com/binaricat/netcatty/internal/platform/filesystem"
	"github.com/binaricat/netcatty/internal/terminal/sftp"
	netcattyssh "github.com/binaricat/netcatty/internal/terminal/ssh"
	"github.com/binaricat/netcatty/internal/terminal/sshpool"
)

// SFTPOpenRequest is the shell-facing SFTP open payload. The canonical
// definition (and JSON contract) lives in internal/app/sftpuse; this alias
// keeps the Wails API name stable.
type SFTPOpenRequest = sftpuse.OpenRequest

// SFTPService is the Wails-facing facade over internal/app/sftpuse. It owns
// only shell wiring: renderer service registration, the terminal transport
// seam and the local staging opener. All SFTP session rules (transport borrow,
// subsystem lifecycle, browsing, transfers, archive staging) live in the
// shared sftpuse.Service, so a future capability dispatch entry point can call
// the same instance.
type SFTPService struct {
	core *sftpuse.Service
}

// NewSFTPService wires the pool, known-hosts store and the local upload-source
// staging opener shared with the filesystem staging path.
func NewSFTPService(pool *sshpool.Pool, knownHosts *netcattyssh.KnownHosts) *SFTPService {
	core := sftpuse.New(pool, knownHosts)
	core.SetStagingOpener(openLocalForUpload)
	return &SFTPService{core: core}
}

func (s *SFTPService) setTempService(temp *filesystem.TempService) { s.core.SetTempService(temp) }

func (s *SFTPService) setTerminalService(terminal *TerminalService) {
	s.core.SetTerminalTransport(terminal.TransportFor)
}

// sftpClient is the legacy package-main view handed to TransferService's
// chunked scheduler, which opens transfer handles with explicit flags; the
// canonical session state lives in internal/app/sftpuse.
type sftpClient struct {
	raw *pkgsftp.Client
}

func (s *SFTPService) acquire(sessionID string) (*sftpClient, func(), error) {
	raw, release, err := s.core.Acquire(sessionID)
	if err != nil {
		return nil, nil, err
	}
	return &sftpClient{raw: raw}, release, nil
}

// openLocalForUpload opens the local source, retrying once after a short
// delay because transient drag sources race the upload pipeline.
func openLocalForUpload(localPath string) (*os.File, error) {
	return openStagingSource(localPath)
}

// Open dials (or borrows) a transport for host and registers an SFTP session.
func (s *SFTPService) Open(request SFTPOpenRequest) (string, error) { return s.core.Open(request) }

// OpenForTerminal opens only a subsystem on the exact authenticated transport.
// It owns the SFTP channel, never the terminal's SSH connection or credentials.
func (s *SFTPService) OpenForTerminal(sessionID string) (string, error) {
	return s.core.OpenForTerminal(sessionID)
}

// List returns one directory listing (directories first, name-ordered).
func (s *SFTPService) List(sessionID, dir string) ([]sftp.Entry, error) {
	return s.core.List(sessionID, dir)
}

// Stat stats one remote path.
func (s *SFTPService) Stat(sessionID, target string) (sftp.FileInfo, error) {
	return s.core.Stat(sessionID, target)
}

// Mkdir creates a remote directory.
func (s *SFTPService) Mkdir(sessionID, dir string) error { return s.core.Mkdir(sessionID, dir) }

// Remove deletes a remote file or directory tree.
func (s *SFTPService) Remove(sessionID, target string) error { return s.core.Remove(sessionID, target) }

// Rename moves or renames a remote path.
func (s *SFTPService) Rename(sessionID, oldPath, newPath string) error {
	return s.core.Rename(sessionID, oldPath, newPath)
}

// Chmod updates the permission bits of one remote path.
func (s *SFTPService) Chmod(sessionID, target, permissions string) error {
	return s.core.Chmod(sessionID, target, permissions)
}

// Read returns a remote file as UTF-8 text.
func (s *SFTPService) Read(sessionID, remotePath string) (string, error) {
	return s.core.Read(sessionID, remotePath)
}

// WriteText writes UTF-8 text to a remote file, creating or truncating it.
func (s *SFTPService) WriteText(sessionID, remotePath, content string) error {
	return s.core.WriteText(sessionID, remotePath, content)
}

// HomeDir returns the remote working directory for the SFTP session.
func (s *SFTPService) HomeDir(sessionID string) (string, error) { return s.core.HomeDir(sessionID) }

// Download streams a remote file to a local destination path.
func (s *SFTPService) Download(sessionID, remotePath, localPath string) (int64, error) {
	return s.core.Download(sessionID, remotePath, localPath)
}

// Upload streams a local file to a remote destination path.
func (s *SFTPService) Upload(sessionID, localPath, remotePath string) (int64, error) {
	return s.core.Upload(sessionID, localPath, remotePath)
}

// ExtractArchive downloads a remote zip, extracts it locally with zip-slip
// protection, and uploads the files next to the archive.
func (s *SFTPService) ExtractArchive(sessionID, remotePath string) (int, error) {
	return s.core.ExtractArchive(sessionID, remotePath)
}

// UploadCompressedFolder zips a local folder and uploads the archive.
func (s *SFTPService) UploadCompressedFolder(sessionID, localFolder, remoteZipPath string) (int64, error) {
	return s.core.UploadCompressedFolder(sessionID, localFolder, remoteZipPath)
}

// Close releases the SFTP client and returns the transport to the pool.
func (s *SFTPService) Close(sessionID string) error { return s.core.Close(sessionID) }
