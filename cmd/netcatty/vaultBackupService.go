package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/binaricat/netcatty/internal/platform/credentials"
)

const (
	vaultBackupDirName      = "vault-backups"
	vaultBackupFilePrefix   = "vault-backup-"
	vaultBackupFileExt      = ".json"
	vaultBackupPurpose      = "vault-backup"
	vaultBackupMaxPayload   = 25 << 20
	vaultBackupMaxFile      = vaultBackupMaxPayload * 2
	vaultBackupMinCount     = 1
	vaultBackupMaxCount     = 100
	vaultBackupDefaultCount = 20
	vaultBackupEncoding     = "credential-v1"
)

var errVaultBackupEncryptionUnavailable = errors.New("Secure storage is unavailable on this platform; vault backups cannot be created or read safely.")

type VaultBackupPreview struct {
	HostCount               int `json:"hostCount"`
	KeyCount                int `json:"keyCount"`
	SnippetCount            int `json:"snippetCount"`
	NoteCount               int `json:"noteCount"`
	IdentityCount           int `json:"identityCount"`
	PortForwardingRuleCount int `json:"portForwardingRuleCount"`
}

type VaultBackupSummary struct {
	ID               string             `json:"id"`
	CreatedAt        int64              `json:"createdAt"`
	Reason           string             `json:"reason"`
	SyncDataVersion  *int               `json:"syncDataVersion,omitempty"`
	SourceAppVersion string             `json:"sourceAppVersion,omitempty"`
	TargetAppVersion string             `json:"targetAppVersion,omitempty"`
	Fingerprint      string             `json:"fingerprint"`
	Preview          VaultBackupPreview `json:"preview"`
}

type VaultBackupCreateRequest struct {
	Payload          json.RawMessage `json:"payload"`
	Reason           string          `json:"reason"`
	SourceAppVersion string          `json:"sourceAppVersion,omitempty"`
	TargetAppVersion string          `json:"targetAppVersion,omitempty"`
	SyncDataVersion  int             `json:"syncDataVersion,omitempty"`
	MaxCount         int             `json:"maxCount,omitempty"`
}

type VaultBackupCreateResult struct {
	Created bool                `json:"created"`
	Backup  *VaultBackupSummary `json:"backup"`
}

type VaultBackupReadRequest struct {
	ID string `json:"id"`
}

type VaultBackupReadResult struct {
	Backup  VaultBackupSummary `json:"backup"`
	Payload json.RawMessage    `json:"payload"`
}

type VaultBackupTrimRequest struct {
	MaxCount int `json:"maxCount"`
}

type VaultBackupTrimResult struct {
	DeletedCount int `json:"deletedCount"`
	KeptCount    int `json:"keptCount"`
}

type VaultBackupOpenDirResult struct {
	Success bool   `json:"success"`
	Path    string `json:"path"`
}

type VaultBackupCapabilities struct {
	EncryptionAvailable bool `json:"encryptionAvailable"`
}

type vaultBackupRecord struct {
	FormatVersion    int                `json:"formatVersion"`
	ID               string             `json:"id"`
	CreatedAt        int64              `json:"createdAt"`
	Reason           string             `json:"reason"`
	SyncDataVersion  *int               `json:"syncDataVersion,omitempty"`
	SourceAppVersion string             `json:"sourceAppVersion,omitempty"`
	TargetAppVersion string             `json:"targetAppVersion,omitempty"`
	Fingerprint      string             `json:"fingerprint"`
	Preview          VaultBackupPreview `json:"preview"`
	PayloadEncoding  string             `json:"payloadEncoding"`
	PayloadData      string             `json:"payloadData"`
}

type vaultBackupFile struct {
	record   vaultBackupRecord
	filePath string
}

type VaultBackupService struct {
	mu            sync.Mutex
	rootDir       string
	provider      credentials.Provider
	lastCreatedAt int64
	openPath      func(string) error
}

func newVaultBackupService(rootDir string, provider credentials.Provider) *VaultBackupService {
	return &VaultBackupService{rootDir: rootDir, provider: provider, openPath: openSystemFile}
}

func (s *VaultBackupService) backupDir() string {
	return filepath.Join(s.rootDir, vaultBackupDirName)
}

func (s *VaultBackupService) GetVaultBackupCapabilities() VaultBackupCapabilities {
	return VaultBackupCapabilities{EncryptionAvailable: s.provider != nil && s.provider.Available()}
}

func (s *VaultBackupService) CreateVaultBackup(req VaultBackupCreateRequest) (VaultBackupCreateResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.provider == nil || !s.provider.Available() {
		return VaultBackupCreateResult{}, errVaultBackupEncryptionUnavailable
	}
	if len(req.Payload) == 0 || req.Payload[0] != '{' {
		return VaultBackupCreateResult{}, errors.New("Missing vault backup payload.")
	}
	if len(req.Payload) > vaultBackupMaxPayload {
		return VaultBackupCreateResult{}, fmt.Errorf("Vault backup payload exceeds maximum allowed size (%d > %d).", len(req.Payload), vaultBackupMaxPayload)
	}
	var payload any
	if err := json.Unmarshal(req.Payload, &payload); err != nil {
		return VaultBackupCreateResult{}, errors.New("Missing vault backup payload.")
	}
	fingerprint, err := vaultBackupFingerprint(payload)
	if err != nil {
		return VaultBackupCreateResult{}, err
	}
	dir := s.backupDir()
	existing, err := listVaultBackupFiles(dir)
	if err != nil {
		return VaultBackupCreateResult{}, err
	}
	if len(existing) > 0 && existing[0].record.Fingerprint == fingerprint {
		summary := toVaultBackupSummary(existing[0].record)
		return VaultBackupCreateResult{Created: false, Backup: &summary}, nil
	}
	createdAt := s.lastCreatedAt + 1
	if now := unixMilliNow(); now > createdAt {
		createdAt = now
	}
	s.lastCreatedAt = createdAt
	id, err := newVaultBackupID()
	if err != nil {
		return VaultBackupCreateResult{}, err
	}
	sealed, err := s.provider.Seal(req.Payload, vaultBackupPurpose)
	if err != nil {
		if errors.Is(err, credentials.ErrUnavailable) {
			return VaultBackupCreateResult{}, errVaultBackupEncryptionUnavailable
		}
		return VaultBackupCreateResult{}, err
	}
	record := vaultBackupRecord{
		FormatVersion:    1,
		ID:               id,
		CreatedAt:        createdAt,
		Reason:           sanitizeVaultBackupReason(req.Reason),
		SyncDataVersion:  sanitizeVaultBackupVersion(req.SyncDataVersion),
		SourceAppVersion: sanitizeVaultBackupVersionString(req.SourceAppVersion),
		TargetAppVersion: sanitizeVaultBackupVersionString(req.TargetAppVersion),
		Fingerprint:      fingerprint,
		Preview:          vaultBackupPreview(payload),
		PayloadEncoding:  vaultBackupEncoding,
		PayloadData:      base64.StdEncoding.EncodeToString(sealed),
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return VaultBackupCreateResult{}, err
	}
	filePath := filepath.Join(dir, fmt.Sprintf("%s%d-%s%s", vaultBackupFilePrefix, createdAt, id, vaultBackupFileExt))
	if err := writeVaultBackupFile(filePath, record); err != nil {
		return VaultBackupCreateResult{}, err
	}
	next := append([]vaultBackupFile{{record: record, filePath: filePath}}, existing...)
	if _, err := pruneVaultBackupFiles(dir, req.MaxCount, next); err != nil {
		return VaultBackupCreateResult{}, err
	}
	summary := toVaultBackupSummary(record)
	return VaultBackupCreateResult{Created: true, Backup: &summary}, nil
}

func (s *VaultBackupService) ListVaultBackups() ([]VaultBackupSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, err := listVaultBackupFiles(s.backupDir())
	if err != nil {
		return nil, err
	}
	out := make([]VaultBackupSummary, 0, len(files))
	for _, file := range files {
		out = append(out, toVaultBackupSummary(file.record))
	}
	return out, nil
}

func (s *VaultBackupService) ReadVaultBackup(req VaultBackupReadRequest) (VaultBackupReadResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.ID == "" {
		return VaultBackupReadResult{}, errors.New("Missing vault backup id.")
	}
	if s.provider == nil || !s.provider.Available() {
		return VaultBackupReadResult{}, errVaultBackupEncryptionUnavailable
	}
	files, err := listVaultBackupFiles(s.backupDir())
	if err != nil {
		return VaultBackupReadResult{}, err
	}
	for _, file := range files {
		if file.record.ID != req.ID {
			continue
		}
		payload, err := s.decodeVaultBackupPayload(file.record)
		if err != nil {
			return VaultBackupReadResult{}, err
		}
		return VaultBackupReadResult{Backup: toVaultBackupSummary(file.record), Payload: payload}, nil
	}
	return VaultBackupReadResult{}, errors.New("Vault backup not found.")
}

func (s *VaultBackupService) TrimVaultBackups(req VaultBackupTrimRequest) (VaultBackupTrimResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return pruneVaultBackupFiles(s.backupDir(), req.MaxCount, nil)
}

func (s *VaultBackupService) OpenVaultBackupDir() (VaultBackupOpenDirResult, error) {
	dir := s.backupDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return VaultBackupOpenDirResult{}, err
	}
	if s.openPath != nil {
		if err := s.openPath(dir); err != nil {
			return VaultBackupOpenDirResult{}, err
		}
	}
	return VaultBackupOpenDirResult{Success: true, Path: dir}, nil
}

func (s *VaultBackupService) decodeVaultBackupPayload(record vaultBackupRecord) (json.RawMessage, error) {
	if record.PayloadEncoding == "plain-json-v1" {
		return json.RawMessage(record.PayloadData), nil
	}
	if record.PayloadEncoding != vaultBackupEncoding {
		return nil, fmt.Errorf("Unsupported vault backup encoding: %s", record.PayloadEncoding)
	}
	sealed, err := base64.StdEncoding.DecodeString(record.PayloadData)
	if err != nil {
		return nil, err
	}
	plain, err := s.provider.Open(sealed, vaultBackupPurpose)
	if err != nil {
		if errors.Is(err, credentials.ErrUnavailable) {
			return nil, errVaultBackupEncryptionUnavailable
		}
		return nil, err
	}
	return json.RawMessage(plain), nil
}

func toVaultBackupSummary(record vaultBackupRecord) VaultBackupSummary {
	return VaultBackupSummary{
		ID: record.ID, CreatedAt: record.CreatedAt, Reason: record.Reason,
		SyncDataVersion: record.SyncDataVersion, SourceAppVersion: record.SourceAppVersion,
		TargetAppVersion: record.TargetAppVersion, Fingerprint: record.Fingerprint, Preview: record.Preview,
	}
}

func vaultBackupPreview(payload any) VaultBackupPreview {
	object, _ := payload.(map[string]any)
	count := func(key string) int {
		items, _ := object[key].([]any)
		return len(items)
	}
	return VaultBackupPreview{
		HostCount: count("hosts"), KeyCount: count("keys"), SnippetCount: count("snippets"),
		NoteCount: count("notes"), IdentityCount: count("identities"), PortForwardingRuleCount: count("portForwardingRules"),
	}
}

func vaultBackupFingerprint(payload any) (string, error) {
	canonical, err := json.Marshal(canonicalizeVaultBackup(payload, true))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func canonicalizeVaultBackup(value any, root bool) any {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(keys))
		for _, key := range keys {
			if root && key == "syncedAt" {
				out[key] = float64(0)
				continue
			}
			out[key] = canonicalizeVaultBackup(typed[key], false)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = canonicalizeVaultBackup(item, false)
		}
		return out
	default:
		return typed
	}
}

func sanitizeVaultBackupReason(reason string) string {
	if reason == "app_version_change" || reason == "before_restore" {
		return reason
	}
	return "before_restore"
}

func sanitizeVaultBackupVersionString(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len(trimmed) > 64 {
		return ""
	}
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '+' || r == '-' {
			continue
		}
		return ""
	}
	return trimmed
}

func sanitizeVaultBackupVersion(value int) *int {
	if value < 1 {
		return nil
	}
	copied := value
	return &copied
}

func sanitizeVaultBackupMaxCount(value int) int {
	if value < vaultBackupMinCount {
		return vaultBackupDefaultCount
	}
	if value > vaultBackupMaxCount {
		return vaultBackupMaxCount
	}
	return value
}

func newVaultBackupID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func unixMilliNow() int64 {
	return time.Now().UnixMilli()
}

func writeVaultBackupFile(path string, record vaultBackupRecord) error {
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(body, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func listVaultBackupFiles(dir string) ([]vaultBackupFile, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []vaultBackupFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), vaultBackupFilePrefix) || !strings.HasSuffix(entry.Name(), vaultBackupFileExt) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil || info.Size() > vaultBackupMaxFile {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var record vaultBackupRecord
		if json.Unmarshal(raw, &record) != nil || record.ID == "" {
			continue
		}
		files = append(files, vaultBackupFile{record: record, filePath: path})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].record.CreatedAt != files[j].record.CreatedAt {
			return files[i].record.CreatedAt > files[j].record.CreatedAt
		}
		return files[i].record.ID > files[j].record.ID
	})
	return files, nil
}

func pruneVaultBackupFiles(dir string, maxCount int, files []vaultBackupFile) (VaultBackupTrimResult, error) {
	if files == nil {
		var err error
		files, err = listVaultBackupFiles(dir)
		if err != nil {
			return VaultBackupTrimResult{}, err
		}
	}
	limit := sanitizeVaultBackupMaxCount(maxCount)
	deleted := 0
	for _, file := range files[min(len(files), limit):] {
		if os.Remove(file.filePath) == nil {
			deleted++
		}
	}
	kept := len(files) - deleted
	if kept < 0 {
		kept = 0
	}
	if kept > limit {
		kept = limit
	}
	return VaultBackupTrimResult{DeletedCount: deleted, KeptCount: kept}, nil
}
