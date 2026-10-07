// Disk persistence for the plugin store (P5-02): snapshot JSON written
// atomically (temp file in the same directory + rename) so a crash mid-write
// never corrupts the installed inventory. Load recovers from a stale temp
// file and keeps a .bak of the previous good snapshot.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

var (
	// ErrSnapshotCorrupt is returned when neither the snapshot nor its
	// backup can be parsed.
	ErrSnapshotCorrupt = errors.New("plugin store snapshot corrupt")
)

type snapshotFile struct {
	Plugins []*PackageRecord `json:"plugins"`
}

// SaveAtomic writes the inventory to path via a temp file + rename. The
// previous snapshot is kept as path.bak so a torn rename still leaves one
// parsable copy on disk.
// Open loads durable inventory and removes unpublished stages before use.
func Open(path string) (*Store, error) {
	s := New()
	if err := s.LoadAtomic(path); err != nil {
		return nil, err
	}
	s.RecoverStaged()
	s.path = path
	if err := s.SaveAtomic(path); err != nil {
		return nil, err
	}
	return s, nil
}

// persistLocked rolls back the mutation on disk failure. Caller holds mu.
func (s *Store) persistLocked(previous map[string]*PackageRecord) error {
	if s.path == "" {
		return nil
	}
	if err := s.saveLocked(s.path); err != nil {
		s.plugins = previous
		return err
	}
	return nil
}

func (s *Store) snapshotLocked() map[string]*PackageRecord {
	result := make(map[string]*PackageRecord, len(s.plugins))
	for id, record := range s.plugins {
		copy := *record
		result[id] = &copy
	}
	return result
}

func (s *Store) SaveAtomic(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(path)
}

func (s *Store) saveLocked(path string) error {
	records := make([]*PackageRecord, 0, len(s.plugins))
	for _, record := range s.plugins {
		copy := *record
		records = append(records, &copy)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].PluginID < records[j].PluginID })

	encoded, err := json.MarshalIndent(snapshotFile{Plugins: records}, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err = file.Write(encoded); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if _, err := readSnapshot(path); err == nil {
		if err := os.Rename(path, path+".bak"); err != nil {
			return fmt.Errorf("rotate previous snapshot: %w", err)
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Rename(path+".bak", path)
		return err
	}
	return nil
}

// LoadAtomic replaces the inventory with the snapshot at path. A missing file
// leaves the store untouched (fresh install). When the primary snapshot is
// corrupt, the .bak copy is tried before failing.
func (s *Store) LoadAtomic(path string) error {
	records, err := readSnapshot(path)
	if err != nil {
		recovered, bakErr := readSnapshot(path + ".bak")
		if bakErr != nil {
			if errors.Is(err, os.ErrNotExist) && errors.Is(bakErr, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("%w: primary %v, backup %v", ErrSnapshotCorrupt, err, bakErr)
		}
		records = recovered
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	plugins := make(map[string]*PackageRecord, len(records))
	for _, record := range records {
		if record == nil || record.PluginID == "" {
			continue
		}
		plugins[record.PluginID] = record
	}
	s.plugins = plugins
	return nil
}

func readSnapshot(path string) ([]*PackageRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var snapshot snapshotFile
	if err := json.Unmarshal(data, &snapshot); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read %s: %w", path, os.ErrNotExist)
		}
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return snapshot.Plugins, nil
}
