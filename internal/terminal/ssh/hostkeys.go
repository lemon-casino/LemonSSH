// Package ssh owns SSH authentication and dial primitives (P3-03). It is the
// shared transport foundation for P3-04's pool and deliberately contains no
// business connection pooling.
package ssh

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"strconv"
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

// Replace rewrites the stored key for hostname+type (host-key rotation after
// explicit user confirmation) or appends when no entry exists yet.
func (k *KnownHosts) Replace(hostname, keyType, base64Key string) error {
	if hostname == "" || keyType == "" || base64Key == "" {
		return fmt.Errorf("invalid known-hosts entry")
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	file, err := os.Open(k.path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return k.appendLocked(hostname, keyType, base64Key)
	}
	var kept []byte
	scanner := bufio.NewScanner(file)
	replaced := false
	for scanner.Scan() {
		line := scanner.Text()
		if entry, ok := parseKnownHostsLine(line); ok && entry.host == hostname && entry.keyType == keyType {
			if !replaced {
				kept = append(kept, []byte(fmt.Sprintf("%s %s %s\n", hostname, keyType, base64Key))...)
				replaced = true
			}
			continue
		}
		kept = append(kept, []byte(line+"\n")...)
	}
	scanErr := scanner.Err()
	closeErr := file.Close()
	if scanErr != nil {
		return scanErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.WriteFile(k.path, kept, 0o600); err != nil {
		return err
	}
	if !replaced {
		return k.appendLocked(hostname, keyType, base64Key)
	}
	return nil
}

// appendLocked appends one entry; the caller holds k.mu.
func (k *KnownHosts) appendLocked(hostname, keyType, base64Key string) error {
	file, err := os.OpenFile(k.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(fmt.Sprintf("%s %s %s\n", hostname, keyType, base64Key))
	return err
}

// Fingerprint renders the OpenSSH "SHA256:..." fingerprint of a public key.
func Fingerprint(key ssh.PublicKey) string {
	sum := sha256.Sum256(key.Marshal())
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
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

// HostKeyChange is the renderer-facing payload for a changed host key. It
// carries both the presented and the previously pinned fingerprints so the
// verification dialog can show the rotation.
type HostKeyChange struct {
	Hostname         string `json:"hostname"`
	Port             uint16 `json:"port"`
	KeyType          string `json:"keyType"`
	Fingerprint      string `json:"fingerprint"`
	PublicKey        string `json:"publicKey"`
	KnownFingerprint string `json:"knownFingerprint"`
}

// HostKeyConfirm asks the renderer to accept or reject a changed host key.
// accept=true with store=false connects once without updating the pinned key.
type HostKeyConfirm func(change HostKeyChange) (accept bool, store bool, err error)

// ConfirmPolicy wraps StrictPolicy with a renderer confirmation for changed
// host keys: accepting (and storing) rotates the pinned key, accepting without
// storing allows exactly this connection, anything else fails closed.
// First sightings stay auto-pinned (accept-new) like StrictPolicy.
func ConfirmPolicy(hosts *KnownHosts, confirm HostKeyConfirm) HostKeyPolicy {
	strict := StrictPolicy(hosts)
	if confirm == nil {
		return strict
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := strict(hostname, remote, key)
		if err == nil {
			return nil
		}
		stored, lookupErr := hosts.Lookup(hostname, key.Type())
		if lookupErr != nil || stored == "" {
			// Not a rotation (first-sight pin or store failure): keep the
			// strict failure instead of prompting over a broken store.
			return err
		}
		encoded := base64.StdEncoding.EncodeToString(key.Marshal())
		displayHost, displayPort := displayHostPort(hostname, remote)
		accept, store, confirmErr := confirm(HostKeyChange{
			Hostname:         displayHost,
			Port:             displayPort,
			KeyType:          key.Type(),
			Fingerprint:      Fingerprint(key),
			PublicKey:        key.Type() + " " + encoded,
			KnownFingerprint: storedFingerprint(stored),
		})
		if confirmErr != nil {
			return confirmErr
		}
		if !accept {
			return err
		}
		if !store {
			return nil
		}
		return hosts.Replace(hostname, key.Type(), encoded)
	}
}

// displayHostPort splits the dial address ("host:port", as the host-key
// callback receives it) into display fields; unparseable hostnames stay whole.
func displayHostPort(hostname string, remote net.Addr) (string, uint16) {
	host, port, err := net.SplitHostPort(hostname)
	if err != nil || port == "" {
		return hostname, remotePort(remote)
	}
	parsed, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return hostname, remotePort(remote)
	}
	return host, uint16(parsed)
}

// remotePort extracts the remote port from the callback address when present.
func remotePort(remote net.Addr) uint16 {
	if tcp, ok := remote.(*net.TCPAddr); ok && tcp.Port > 0 {
		return uint16(tcp.Port)
	}
	return 22
}

// storedFingerprint hashes a stored base64 wire-format key blob.
func storedFingerprint(encoded string) string {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

func short(value string) string {
	if len(value) > 16 {
		return value[:16] + "..."
	}
	return value
}
