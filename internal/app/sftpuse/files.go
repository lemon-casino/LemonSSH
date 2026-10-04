package sftpuse

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/lemon-casino/lemonssh/internal/platform/charset"
	"github.com/lemon-casino/lemonssh/internal/terminal/sftp"
)

// parsePermissions validates an octal permission string (3-4 digits) into a
// file mode, honoring setuid/setgid/sticky bits.
func parsePermissions(text string) (os.FileMode, error) {
	if len(text) < 3 || len(text) > 4 {
		return 0, fmt.Errorf("permissions must contain 3 or 4 octal digits")
	}
	for _, digit := range text {
		if digit < '0' || digit > '7' {
			return 0, fmt.Errorf("invalid octal permissions %q", text)
		}
	}
	value, err := strconv.ParseUint(text, 8, 12)
	if err != nil {
		return 0, err
	}
	mode := os.FileMode(value & 0777)
	if value&04000 != 0 {
		mode |= os.ModeSetuid
	}
	if value&02000 != 0 {
		mode |= os.ModeSetgid
	}
	if value&01000 != 0 {
		mode |= os.ModeSticky
	}
	return mode, nil
}

// Chmod updates the permission bits of one remote path.
func (s *Service) Chmod(sessionID, target, permissions, encoding string) error {
	mode, err := parsePermissions(permissions)
	if err != nil {
		return err
	}
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return err
	}
	defer done()
	resolved, err := sftp.NormalizePath(".", s.encodePath(sessionID, encoding, target))
	if err != nil {
		return err
	}
	return client.raw.Chmod(resolved, mode)
}

// List returns one directory listing (directories first, name-ordered).
// The encoding argument selects the remote filename charset (auto / utf-8 /
// gb18030); entry names are transcoded to UTF-8 before crossing the JSON
// channel, which would otherwise replace legacy bytes with U+FFFD.
func (s *Service) List(sessionID, dir, encoding string) ([]sftp.Entry, error) {
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return nil, err
	}
	defer done()
	resolved, err := sftp.NormalizePath(".", s.encodePath(sessionID, encoding, dir))
	if err != nil {
		return nil, err
	}
	entries, err := client.fs.ReadDir(resolved)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i := range entries {
		names[i] = entries[i].Name
	}
	decoded := s.probeAndDecodeNames(sessionID, names)
	for i := range entries {
		entries[i].Name = decoded[i]
	}
	sftp.SortEntries(entries)
	return entries, nil
}

// Stat stats one remote path. The path is encoded to the remote filename
// charset before the request.
func (s *Service) Stat(sessionID, target, encoding string) (sftp.FileInfo, error) {
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return sftp.FileInfo{}, err
	}
	defer done()
	resolved, err := sftp.NormalizePath(".", s.encodePath(sessionID, encoding, target))
	if err != nil {
		return sftp.FileInfo{}, err
	}
	return client.fs.Stat(resolved)
}

// Lstat returns metadata for the remote path without following a final symlink.
func (s *Service) Lstat(sessionID, target, encoding string) (sftp.FileInfo, bool, error) {
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return sftp.FileInfo{}, false, err
	}
	defer done()
	resolved, err := sftp.NormalizePath(".", s.encodePath(sessionID, encoding, target))
	if err != nil {
		return sftp.FileInfo{}, false, err
	}
	info, err := client.raw.Lstat(resolved)
	if err != nil {
		return sftp.FileInfo{}, false, err
	}
	return sftp.FileInfo{Path: resolved, IsDir: info.IsDir(), Size: info.Size(), Mode: info.Mode().String(), ModTime: info.ModTime()}, info.Mode()&os.ModeSymlink != 0, nil
}

func (s *Service) RealPath(sessionID, target, encoding string) (string, error) {
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return "", err
	}
	defer done()
	resolved, err := sftp.NormalizePath(".", s.encodePath(sessionID, encoding, target))
	if err != nil {
		return "", err
	}
	absolute, err := client.raw.RealPath(resolved)
	if err != nil {
		return "", err
	}
	return charset.Decode(absolute, s.resolvedEncoding(sessionID)), nil
}

// Mkdir creates a remote directory.
func (s *Service) Mkdir(sessionID, dir, encoding string) error {
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return err
	}
	defer done()
	target, err := sftp.NormalizePath(".", s.encodePath(sessionID, encoding, dir))
	if err != nil {
		return err
	}
	return client.fs.Mkdir(target)
}

// Remove deletes a remote file or directory tree.
func (s *Service) Remove(sessionID, target, encoding string) error {
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return err
	}
	defer done()
	resolved, err := sftp.NormalizePath(".", s.encodePath(sessionID, encoding, target))
	if err != nil {
		return err
	}
	return client.fs.Remove(resolved)
}

// Rename moves or renames a remote path.
func (s *Service) Rename(sessionID, oldPath, newPath, encoding string) error {
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return err
	}
	defer done()
	from, err := sftp.NormalizePath(".", s.encodePath(sessionID, encoding, oldPath))
	if err != nil {
		return err
	}
	to, err := sftp.NormalizePath(".", s.encodePath(sessionID, encoding, newPath))
	if err != nil {
		return err
	}
	return client.fs.Rename(from, to)
}

// Read returns a remote file as UTF-8 text.
func (s *Service) Read(sessionID, remotePath, encoding string) (string, error) {
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return "", err
	}
	defer done()
	resolved, err := sftp.NormalizePath(".", s.encodePath(sessionID, encoding, remotePath))
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

func (s *Service) ReadBinary(sessionID, remotePath, encoding string) ([]byte, error) {
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return nil, err
	}
	defer done()
	resolved, err := sftp.NormalizePath(".", s.encodePath(sessionID, encoding, remotePath))
	if err != nil {
		return nil, err
	}
	reader, err := client.fs.Open(resolved)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// WriteText writes UTF-8 text to a remote file, creating or truncating it.
func (s *Service) WriteText(sessionID, remotePath, content, encoding string) error {
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return err
	}
	defer done()
	resolved, err := sftp.NormalizePath(".", s.encodePath(sessionID, encoding, remotePath))
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

func (s *Service) WriteBinary(sessionID, remotePath string, content []byte, encoding string) error {
	client, done, err := s.acquire(sessionID)
	if err != nil {
		return err
	}
	defer done()
	resolved, err := sftp.NormalizePath(".", s.encodePath(sessionID, encoding, remotePath))
	if err != nil {
		return err
	}
	writer, err := client.fs.Create(resolved)
	if err != nil {
		return err
	}
	if _, err = writer.Write(content); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}
