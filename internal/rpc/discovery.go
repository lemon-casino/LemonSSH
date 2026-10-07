package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Discovery is the on-disk first-party contract: the loopback port and
// bearer token a first-party launcher hands its own CLI/MCP children. The
// field names match the existing Electron bridge payload so launchers keep
// working across the migration; the token itself never appears in argv or
// traces, only in this 0600 file.
type Discovery struct {
	Port           int    `json:"port"`
	Token          string `json:"token"`
	PID            int    `json:"pid,omitempty"`
	PermissionMode string `json:"permissionMode,omitempty"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
}

var (
	ErrDiscoveryMissing = errors.New("rpc: discovery file missing")
	ErrDiscoveryInvalid = errors.New("rpc: discovery file invalid")
)

// WriteDiscovery atomically writes the discovery file with 0600 permission.
// Windows ACL tightening is recorded as pending platform evidence (W06
// acceptance); the mode byte is best-effort on that platform.
func WriteDiscovery(path string, discovery Discovery) error {
	raw, err := json.MarshalIndent(discovery, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

// LoadDiscovery reads and validates a discovery file.
func LoadDiscovery(path string) (*Discovery, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w at %s", ErrDiscoveryMissing, path)
		}
		return nil, err
	}
	var discovery Discovery
	if err := json.Unmarshal(raw, &discovery); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDiscoveryInvalid, err)
	}
	if discovery.Port == 0 || discovery.Token == "" {
		return nil, fmt.Errorf("%w: missing port/token", ErrDiscoveryInvalid)
	}
	return &discovery, nil
}

// RemoveDiscovery deletes the discovery file if present.
func RemoveDiscovery(path string) error {
	err := os.Remove(path)
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}
