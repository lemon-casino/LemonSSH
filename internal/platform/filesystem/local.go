package filesystem

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// LocalEntry matches the renderer's local directory listing contract.
type LocalEntry struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	Size         string  `json:"size"`
	LastModified string  `json:"lastModified"`
	LinkTarget   *string `json:"linkTarget"`
	Hidden       bool    `json:"hidden"`
}

func ListDirectory(path string) ([]LocalEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	files := make([]LocalEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			if os.IsNotExist(err) {
				continue // The entry was removed after ReadDir.
			}
			return nil, err
		}
		file := LocalEntry{Name: entry.Name(), Type: "file", Hidden: isHidden(info)}
		if info.Mode()&os.ModeSymlink != 0 {
			file.Type = "symlink"
			if target, err := os.Stat(filepath.Join(path, entry.Name())); err == nil {
				targetType := "file"
				if target.IsDir() {
					targetType = "directory"
				}
				file.LinkTarget = &targetType
				info = target
			}
		} else if info.IsDir() {
			file.Type = "directory"
		}
		file.Size = strconv.FormatInt(info.Size(), 10)
		file.LastModified = info.ModTime().UTC().Format(time.RFC3339Nano)
		files = append(files, file)
	}
	return files, nil
}
