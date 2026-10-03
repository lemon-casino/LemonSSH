package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// KnownHostsService exposes the user's system OpenSSH known_hosts files to the
// renderer so the vault known-hosts page can scan and import them. The
// application's own pinned keys live in the profile known_hosts file owned by
// the terminal service; this surface only reads the OS-level files.
type KnownHostsService struct{}

// NewKnownHostsService wires the system known-hosts reader.
func NewKnownHostsService() *KnownHostsService { return &KnownHostsService{} }

// systemKnownHostsFiles lists the OpenSSH user files scanned on every OS.
func systemKnownHostsFiles(homeDir string) []string {
	return []string{
		filepath.Join(homeDir, ".ssh", "known_hosts"),
		filepath.Join(homeDir, ".ssh", "known_hosts2"),
	}
}

// ReadKnownHosts returns the concatenated system known_hosts content in the
// OpenSSH `host key-type base64` line format, or "" when no file exists so
// the renderer can tell "nothing scanned" from "unavailable".
func (s *KnownHostsService) ReadKnownHosts() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return readSystemKnownHosts(homeDir)
}

// readSystemKnownHosts concatenates the known_hosts files under a home
// directory; missing files are skipped instead of failing the scan.
func readSystemKnownHosts(homeDir string) (string, error) {
	var combined bytes.Buffer
	for _, path := range systemKnownHostsFiles(homeDir) {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			return "", readErr
		}
		if combined.Len() > 0 && !strings.HasSuffix(combined.String(), "\n") {
			combined.WriteByte('\n')
		}
		combined.Write(data)
	}
	return combined.String(), nil
}
