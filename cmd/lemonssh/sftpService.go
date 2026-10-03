package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"

	pkgsftp "github.com/pkg/sftp"

	"github.com/binaricat/lemonssh/internal/app/sftpuse"
	"github.com/binaricat/lemonssh/internal/platform/charset"
	"github.com/binaricat/lemonssh/internal/platform/filesystem"
	"github.com/binaricat/lemonssh/internal/terminal/sftp"
	lemonsshssh "github.com/binaricat/lemonssh/internal/terminal/ssh"
	"github.com/binaricat/lemonssh/internal/terminal/sshpool"
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
	core           *sftpuse.Service
	mu             sync.Mutex
	transferLeases map[string]map[string]struct{}
	pendingClose   map[string]bool
	closeObserver  func(sessionID string)
}

type SFTPLstatResult struct {
	Path      string `json:"path"`
	IsDir     bool   `json:"isDir"`
	IsSymlink bool   `json:"isSymlink"`
	Size      int64  `json:"size"`
	Mode      string `json:"mode"`
	ModTime   any    `json:"modTime"`
}

// NewSFTPService wires the pool, known-hosts store and the local upload-source
// staging opener shared with the filesystem staging path.
func NewSFTPService(pool *sshpool.Pool, knownHosts *lemonsshssh.KnownHosts) *SFTPService {
	core := sftpuse.New(pool, knownHosts)
	core.SetStagingOpener(openLocalForUpload)
	return &SFTPService{core: core, transferLeases: make(map[string]map[string]struct{}), pendingClose: make(map[string]bool)}
}

func (s *SFTPService) setTempService(temp *filesystem.TempService) { s.core.SetTempService(temp) }

func (s *SFTPService) setTerminalService(terminal *TerminalService) {
	s.core.SetTerminalTransport(terminal.TransportFor)
	// Standalone SFTP dials share the terminal's interactive auth surface
	// (passphrase prompts, changed host-key confirmation) so the renderer's
	// SFTP verification dialog stays reachable.
	s.core.SetInteractiveDial(func(request sftpuse.OpenRequest) lemonsshssh.DialInteractive {
		return terminal.core.DialInteractive(context.Background(), request.SSHConnectRequest)
	})
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
// Encoding selects the remote filename charset ("", "auto", "utf-8",
// "gb18030"); names are transcoded to UTF-8 Go-side because the JSON channel
// would otherwise replace legacy bytes with U+FFFD.
func (s *SFTPService) List(sessionID, dir, encoding string) ([]sftp.Entry, error) {
	return s.core.List(sessionID, dir, encoding)
}

// Stat stats one remote path.
func (s *SFTPService) Stat(sessionID, target, encoding string) (sftp.FileInfo, error) {
	return s.core.Stat(sessionID, target, encoding)
}

func (s *SFTPService) Lstat(sessionID, target, encoding string) (SFTPLstatResult, error) {
	info, symlink, err := s.core.Lstat(sessionID, target, encoding)
	if err != nil {
		return SFTPLstatResult{}, err
	}
	return SFTPLstatResult{Path: info.Path, IsDir: info.IsDir, IsSymlink: symlink, Size: info.Size, Mode: info.Mode, ModTime: info.ModTime}, nil
}

func (s *SFTPService) RealPath(sessionID, target, encoding string) (string, error) {
	return s.core.RealPath(sessionID, target, encoding)
}

// Mkdir creates a remote directory.
func (s *SFTPService) Mkdir(sessionID, dir, encoding string) error { return s.core.Mkdir(sessionID, dir, encoding) }

// Remove deletes a remote file or directory tree.
func (s *SFTPService) Remove(sessionID, target, encoding string) error { return s.core.Remove(sessionID, target, encoding) }

// Rename moves or renames a remote path.
func (s *SFTPService) Rename(sessionID, oldPath, newPath, encoding string) error {
	return s.core.Rename(sessionID, oldPath, newPath, encoding)
}

// Chmod updates the permission bits of one remote path.
func (s *SFTPService) Chmod(sessionID, target, permissions, encoding string) error {
	return s.core.Chmod(sessionID, target, permissions, encoding)
}

// Read returns a remote file as UTF-8 text.
func (s *SFTPService) Read(sessionID, remotePath, encoding string) (string, error) {
	return s.core.Read(sessionID, remotePath, encoding)
}

func (s *SFTPService) ReadBinary(sessionID, remotePath, encoding string) ([]byte, error) {
	return s.core.ReadBinary(sessionID, remotePath, encoding)
}

// WriteText writes UTF-8 text to a remote file, creating or truncating it.
func (s *SFTPService) WriteText(sessionID, remotePath, content, encoding string) error {
	return s.core.WriteText(sessionID, remotePath, content, encoding)
}

func (s *SFTPService) WriteBinary(sessionID, remotePath string, content []byte, encoding string) error {
	return s.core.WriteBinary(sessionID, remotePath, content, encoding)
}

// HomeDir returns the remote working directory for the SFTP session.
func (s *SFTPService) HomeDir(sessionID string) (string, error) { return s.core.HomeDir(sessionID) }

// Download streams a remote file to a local destination path.
func (s *SFTPService) Download(sessionID, remotePath, localPath, encoding string) (int64, error) {
	return s.core.Download(sessionID, remotePath, localPath, encoding)
}

// Upload streams a local file to a remote destination path.
func (s *SFTPService) Upload(sessionID, localPath, remotePath, encoding string) (int64, error) {
	return s.core.Upload(sessionID, localPath, remotePath, encoding)
}

// ExtractArchive downloads a remote zip, extracts it locally with zip-slip
// protection, and uploads the files next to the archive.
func (s *SFTPService) ExtractArchive(sessionID, remotePath, encoding string) (int, error) {
	return s.core.ExtractArchive(sessionID, remotePath, encoding)
}

// UploadCompressedFolder zips a local folder and uploads the archive.
func (s *SFTPService) UploadCompressedFolder(sessionID, localFolder, remoteZipPath, encoding string) (int64, error) {
	return s.core.UploadCompressedFolder(sessionID, localFolder, remoteZipPath, encoding)
}

// RetainTransfer defers a close while a renderer-owned transfer still uses the
// SFTP session. Lease IDs are idempotent within one session.
func (s *SFTPService) RetainTransfer(sessionID, leaseID string) error {
	if leaseID == "" {
		return fmt.Errorf("lease id is required")
	}
	// Validate the session without retaining a raw client handle.
	_, release, err := s.core.Acquire(sessionID)
	if err != nil {
		return err
	}
	release()
	s.mu.Lock()
	defer s.mu.Unlock()
	leases := s.transferLeases[sessionID]
	if leases == nil {
		leases = make(map[string]struct{})
		s.transferLeases[sessionID] = leases
	}
	leases[leaseID] = struct{}{}
	return nil
}

func (s *SFTPService) ReleaseTransfer(sessionID, leaseID string) error {
	s.mu.Lock()
	leases := s.transferLeases[sessionID]
	delete(leases, leaseID)
	shouldClose := len(leases) == 0 && s.pendingClose[sessionID]
	if len(leases) == 0 {
		delete(s.transferLeases, sessionID)
	}
	if shouldClose {
		delete(s.pendingClose, sessionID)
	}
	s.mu.Unlock()
	if shouldClose {
		if err := s.core.Close(sessionID); err != nil {
			return err
		}
		s.notifySessionClosed(sessionID)
	}
	return nil
}

// CopyDirectory performs a same-host recursive copy entirely through the
// existing SFTP connection, avoiding a renderer/local staging round trip.
func (s *SFTPService) CopyDirectory(sessionID, sourcePath, targetPath, encoding string) error {
	client, release, err := s.core.Acquire(sessionID)
	if err != nil {
		return err
	}
	defer release()
	source := path.Clean(charset.Encode(sourcePath, s.core.ResolveEncoding(sessionID, encoding)))
	target := path.Clean(charset.Encode(targetPath, s.core.ResolveEncoding(sessionID, encoding)))
	if source == "." || target == "." {
		return fmt.Errorf("source and target paths are required")
	}
	if err := client.MkdirAll(target); err != nil {
		return err
	}
	walker := client.Walk(source)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			return err
		}
		current := path.Clean(walker.Path())
		relative := strings.TrimPrefix(current, source)
		relative = strings.TrimPrefix(relative, "/")
		destination := target
		if relative != "" {
			destination = path.Join(target, relative)
		}
		info := walker.Stat()
		if info.IsDir() {
			if err := client.MkdirAll(destination); err != nil {
				return err
			}
			continue
		}
		reader, err := client.Open(current)
		if err != nil {
			return err
		}
		writer, err := client.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
		if err != nil {
			_ = reader.Close()
			return err
		}
		_, copyErr := io.Copy(writer, reader)
		readCloseErr := reader.Close()
		writeCloseErr := writer.Close()
		if copyErr != nil {
			return copyErr
		}
		if readCloseErr != nil {
			return readCloseErr
		}
		if writeCloseErr != nil {
			return writeCloseErr
		}
	}
	return nil
}

// Close releases the SFTP client once outstanding transfer leases drain.
func (s *SFTPService) Close(sessionID string) error {
	s.mu.Lock()
	if len(s.transferLeases[sessionID]) > 0 {
		s.pendingClose[sessionID] = true
		s.mu.Unlock()
		return nil
	}
	delete(s.pendingClose, sessionID)
	s.mu.Unlock()
	if err := s.core.Close(sessionID); err != nil {
		return err
	}
	s.notifySessionClosed(sessionID)
	return nil
}

// setSessionCloseObserver wires a callback invoked when a session actually
// closes (immediately, or once transfer leases drain). The file watch service
// uses it to stop external-edit sync watches bound to the session.
func (s *SFTPService) setSessionCloseObserver(fn func(sessionID string)) { s.closeObserver = fn }

func (s *SFTPService) notifySessionClosed(sessionID string) {
	if s.closeObserver == nil {
		return
	}
	s.closeObserver(sessionID)
}
