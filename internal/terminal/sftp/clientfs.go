package sftp

import (
	"io"
	"os"
	"path"
	"time"

	pkgsftp "github.com/pkg/sftp"
)

// ClientFS adapts a production pkg/sftp client to the transport-neutral
// RemoteFS surface. Symlink entries are reported via Lstat-before-Stat.
type ClientFS struct {
	client *pkgsftp.Client
}

// NewClientFS wraps an authenticated SFTP client.
func NewClientFS(client *pkgsftp.Client) *ClientFS { return &ClientFS{client: client} }

func toEntry(info os.FileInfo) Entry {
	entry := Entry{
		Name:    info.Name(),
		IsDir:   info.IsDir(),
		Size:    info.Size(),
		Mode:    info.Mode().String(),
		ModTime: info.ModTime().Truncate(time.Second).UTC(),
	}
	if info.Mode()&os.ModeSymlink != 0 {
		entry.Symlink = true
	}
	return entry
}

// ReadDir lists one directory with symlink resolution per entry.
func (f *ClientFS) ReadDir(dir string) ([]Entry, error) {
	infos, err := f.client.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(infos))
	for _, info := range infos {
		entry := toEntry(info)
		if entry.Symlink {
			full := path.Join(dir, info.Name())
			if resolved, statErr := f.client.Stat(full); statErr == nil {
				entry.IsDir = resolved.IsDir()
				entry.Size = resolved.Size()
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// Stat stats one path.
func (f *ClientFS) Stat(target string) (FileInfo, error) {
	info, err := f.client.Stat(target)
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{
		Path:    target,
		IsDir:   info.IsDir(),
		Size:    info.Size(),
		Mode:    info.Mode().String(),
		ModTime: info.ModTime().Truncate(time.Second).UTC(),
	}, nil
}

// Mkdir creates a directory.
func (f *ClientFS) Mkdir(dir string) error { return f.client.Mkdir(dir) }

// Remove deletes a file or (recursively) a directory tree.
func (f *ClientFS) Remove(target string) error {
	info, err := f.client.Stat(target)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return f.client.Remove(target)
	}
	return f.removeTree(target)
}

func (f *ClientFS) removeTree(dir string) error {
	entries, err := f.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		child := path.Join(dir, entry.Name)
		if entry.IsDir {
			if err := f.removeTree(child); err != nil {
				return err
			}
			continue
		}
		if err := f.client.Remove(child); err != nil {
			return err
		}
	}
	return f.client.RemoveDirectory(dir)
}

// Rename moves or renames a path.
func (f *ClientFS) Rename(oldPath, newPath string) error { return f.client.PosixRename(oldPath, newPath) }

// Open opens a remote file for reading.
func (f *ClientFS) Open(target string) (io.ReadCloser, error) {
	file, err := f.client.Open(target)
	if err != nil {
		return nil, err
	}
	return file, nil
}

// Create opens (or truncates) a remote file for writing.
func (f *ClientFS) Create(target string) (io.WriteCloser, error) {
	file, err := f.client.Create(target)
	if err != nil {
		return nil, err
	}
	return file, nil
}
