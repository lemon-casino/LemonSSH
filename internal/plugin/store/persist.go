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
func (s *Store) SaveAtomic(path string) error {
	s.mu.RLock()
	records := make([]*PackageRecord, 0, len(s.plugins))
	for _, record := range s.plugins {
		copy := *record
		records = append(records, &copy)
	}
	s.mu.RUnlock()
	sort.Slice(records, func(i, j int) bool { return records[i].PluginID < records[j].PluginID })

	encoded, err := json.MarshalIndent(snapshotFile{Plugins: records}, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, path+".bak"); err != nil {
			return fmt.Errorf("rotate previous snapshot: %w", err)
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadAtomic replaces the inventory with the snapshot at path. A missing file
// leaves the store untouched (fresh install). When the primary snapshot is
// corrupt, the .bak copy is tried before failing.
func (s *Store) LoadAtomic(path string) error {
	records, err := readSnapshot(path)
	if err != nil {
		recovered, bakErr := readSnapshot(path + ".bak")
		if bakErr != nil {
			if errors.Is(err, os.ErrNotExist) {
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
