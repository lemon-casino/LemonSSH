// Package ssh owns SSH authentication and dial primitives (P3-03). It is the
// shared transport foundation for P3-04's pool and deliberately contains no
// business connection pooling.
package ssh

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

// HostKeyPolicy decides whether a presented host key is acceptable.
type HostKeyPolicy func(hostname string, remote net.Addr, key ssh.PublicKey) error

// KnownHosts is the structured known-hosts store backed by a text file in the
// OpenSSH `host key-type base64` line format. Lookups are thread-safe.
type KnownHosts struct {
	mu   sync.Mutex
	path string
}

// NewKnownHosts opens (creating on first write) a known-hosts file.
func NewKnownHosts(path string) *KnownHosts { return &KnownHosts{path: path} }

type knownHostLine struct {
	host     string
	keyType  string
	keyBytes string
}

func parseKnownHostsLine(line string) (knownHostLine, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return knownHostLine{}, false
	}
	fields := strings.Fields(trimmed)
	if len(fields) != 3 {
		return knownHostLine{}, false
	}
	return knownHostLine{host: fields[0], keyType: fields[1], keyBytes: fields[2]}, true
}

// Lookup returns the stored base64 key for hostname+type, or "".
func (k *KnownHosts) Lookup(hostname, keyType string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	file, err := os.Open(k.path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line, ok := parseKnownHostsLine(scanner.Text())
		if !ok || line.host != hostname || line.keyType != keyType {
			continue
		}
		return line.keyBytes, nil
	}
	return "", scanner.Err()
}

// Add appends one host key entry.
func (k *KnownHosts) Add(hostname, keyType, base64Key string) error {
	if hostname == "" || keyType == "" || base64Key == "" {
		return fmt.Errorf("invalid known-hosts entry")
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	file, err := os.OpenFile(k.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(fmt.Sprintf("%s %s %s\n", hostname, keyType, base64Key))
	return err
}

// StrictPolicy implements accept-new / reject-changed: a first sighting is
// pinned; any mismatch fails closed; stored matches pass.
func StrictPolicy(hosts *KnownHosts) HostKeyPolicy {
	return func(hostname string, _ net.Addr, key ssh.PublicKey) error {
		stored, err := hosts.Lookup(hostname, key.Type())
		if err != nil {
			return err
		}
		encoded := base64.StdEncoding.EncodeToString(key.Marshal())
		if stored == "" {
			return hosts.Add(hostname, key.Type(), encoded)
		}
		if stored != encoded {
			return fmt.Errorf("host key mismatch for %s: stored %s got %s", hostname, short(stored), short(encoded))
		}
		return nil
	}
}

func short(value string) string {
	if len(value) > 16 {
		return value[:16] + "..."
	}
	return value
}
