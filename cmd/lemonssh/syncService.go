package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/lemon-casino/lemonssh/internal/platform/cloudsync"
	"github.com/lemon-casino/lemonssh/internal/platform/credentials"
	"github.com/lemon-casino/lemonssh/internal/profile/store"
	"github.com/lemon-casino/lemonssh/internal/syncengine"
)

type SyncService struct {
	oauth     *cloudsync.OAuthClient
	callbacks *cloudsync.CallbackServer
	passwords *cloudSyncSessionPassword
	reset     *CloudSyncResetService
	backups   *VaultBackupService
}

func newSyncService() *SyncService {
	return &SyncService{oauth: cloudsync.NewOAuthClient(), callbacks: cloudsync.NewCallbackServer()}
}

func (s *SyncService) setSessionDependencies(profile *store.Store, profileDir string, provider credentials.Provider) {
	s.passwords = newCloudSyncSessionPassword(profileDir, provider)
	s.reset = newCloudSyncResetService(profile, s.passwords)
	s.backups = newVaultBackupService(profileDir, provider)
}

// setVaultBackupEventEmitter forwards the cross-window "vault backups changed"
// broadcast to the backup service created by setSessionDependencies.
func (s *SyncService) setVaultBackupEventEmitter(emit func(name string, payload any)) {
	if s.backups == nil {
		return
	}
	s.backups.setEventEmitter(emit)
}

// CloudSyncResetEverything forgets the master key and every cloud sync
// identity key so the user can start over with a new master key.
func (s *SyncService) CloudSyncResetEverything(ctx context.Context) ([]string, error) {
	if s.reset == nil {
		return nil, errSyncResetUnavailable
	}
	return s.reset.ResetSyncEverything(ctx)
}

type VaultBackupListResult struct {
	Backups []VaultBackupSummary `json:"backups"`
}

func (s *SyncService) GetVaultBackupCapabilities() VaultBackupCapabilities {
	if s.backups == nil {
		return VaultBackupCapabilities{}
	}
	return s.backups.GetVaultBackupCapabilities()
}

func (s *SyncService) CreateVaultBackup(req VaultBackupCreateRequest) (VaultBackupCreateResult, error) {
	if s.backups == nil {
		return VaultBackupCreateResult{}, errors.New("Vault backup service unavailable")
	}
	return s.backups.CreateVaultBackup(req)
}

func (s *SyncService) ListVaultBackups() (VaultBackupListResult, error) {
	if s.backups == nil {
		return VaultBackupListResult{}, nil
	}
	backups, err := s.backups.ListVaultBackups()
	return VaultBackupListResult{Backups: backups}, err
}

func (s *SyncService) ReadVaultBackup(req VaultBackupReadRequest) (VaultBackupReadResult, error) {
	if s.backups == nil {
		return VaultBackupReadResult{}, errors.New("Vault backup service unavailable")
	}
	return s.backups.ReadVaultBackup(req)
}

func (s *SyncService) TrimVaultBackups(req VaultBackupTrimRequest) (VaultBackupTrimResult, error) {
	if s.backups == nil {
		return VaultBackupTrimResult{}, errors.New("Vault backup service unavailable")
	}
	return s.backups.TrimVaultBackups(req)
}

func (s *SyncService) OpenVaultBackupDir() (VaultBackupOpenDirResult, error) {
	if s.backups == nil {
		return VaultBackupOpenDirResult{}, errors.New("Vault backup service unavailable")
	}
	return s.backups.OpenVaultBackupDir()
}

func (s *SyncService) ServiceShutdown() error {
	s.callbacks.Close()
	s.oauth.Close()
	return nil
}

func (s *SyncService) Merge(local, remote map[string]syncengine.Entry) map[string]syncengine.Entry {
	return syncengine.Merge(local, remote)
}

func (s *SyncService) Fingerprint(entries map[string]syncengine.Entry) string {
	return syncengine.Fingerprint(entries)
}

// CloudSyncWebDAVConfig mirrors the renderer's WebDAVConfig. All three auth
// modes (basic/digest/token) are implemented by the Go transport; digest and
// token configs fail closed with a clear error when their required secret is
// missing.
type CloudSyncWebDAVConfig struct {
	Endpoint      string `json:"endpoint"`
	AuthType      string `json:"authType,omitempty"`
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	Token         string `json:"token,omitempty"`
	AllowInsecure bool   `json:"allowInsecure,omitempty"`
}

// CloudSyncSyncedFile mirrors the renderer's SyncedFile: meta is the
// encryption envelope, payload is the base64 ciphertext. The pair is stored
// verbatim on the remote so download returns it byte-identical.
type CloudSyncSyncedFile struct {
	Meta    json.RawMessage `json:"meta"`
	Payload string          `json:"payload"`
	ETag    string          `json:"etag,omitempty"`
}

type CloudSyncResource struct {
	ResourceID *string `json:"resourceId"`
}

type CloudSyncDownloadResult struct {
	SyncedFile *CloudSyncSyncedFile `json:"syncedFile"`
}

type CloudSyncDeleteResult struct {
	OK bool `json:"ok"`
}

func webdavClient(config CloudSyncWebDAVConfig) (cloudsync.WebDAVClient, error) {
	authType := strings.ToLower(strings.TrimSpace(config.AuthType))
	switch authType {
	case "", "basic":
		// basic: Username/Password, both optional.
	case "digest":
		if config.Username == "" || config.Password == "" {
			return cloudsync.WebDAVClient{}, fmt.Errorf("digest auth requires username and password")
		}
	case "token":
		if config.Token == "" {
			return cloudsync.WebDAVClient{}, fmt.Errorf("token auth requires a token")
		}
	default:
		return cloudsync.WebDAVClient{}, fmt.Errorf("unsupported WebDAV auth type %q", config.AuthType)
	}
	client := cloudsync.NewWebDAVClient(cloudsync.WebDAVConfig{
		Endpoint:      config.Endpoint,
		AuthType:      authType,
		Username:      config.Username,
		Password:      config.Password,
		Token:         config.Token,
		AllowInsecure: config.AllowInsecure,
	})
	return *client, nil
}

// CloudSyncWebdavInitialize resolves the current remote resource id (the
// strong ETag) or null when no snapshot exists yet.
func (s *SyncService) CloudSyncWebdavInitialize(config CloudSyncWebDAVConfig) (CloudSyncResource, error) {
	client, err := webdavClient(config)
	if err != nil {
		return CloudSyncResource{}, err
	}
	_, etag, err := client.Snapshot(context.Background())
	if err != nil {
		if err == cloudsync.ErrNotFound {
			return CloudSyncResource{}, nil
		}
		return CloudSyncResource{}, err
	}
	resource := etag
	return CloudSyncResource{ResourceID: &resource}, nil
}

// CloudSyncWebdavUpload stores the synced file (meta + ciphertext) verbatim.
func (s *SyncService) CloudSyncWebdavUpload(config CloudSyncWebDAVConfig, syncedFile CloudSyncSyncedFile) (CloudSyncResource, error) {
	client, err := webdavClient(config)
	if err != nil {
		return CloudSyncResource{}, err
	}
	body, err := json.Marshal(syncedFile)
	if err != nil {
		return CloudSyncResource{}, err
	}
	etag, err := client.PutSnapshot(context.Background(), body, syncedFile.ETag)
	if err != nil {
		return CloudSyncResource{}, err
	}
	return CloudSyncResource{ResourceID: &etag}, nil
}

// CloudSyncWebdavDownload fetches the remote synced file or null.
func (s *SyncService) CloudSyncWebdavDownload(config CloudSyncWebDAVConfig) (CloudSyncDownloadResult, error) {
	client, err := webdavClient(config)
	if err != nil {
		return CloudSyncDownloadResult{}, err
	}
	body, etag, err := client.Snapshot(context.Background())
	if err != nil {
		if err == cloudsync.ErrNotFound {
			return CloudSyncDownloadResult{}, nil
		}
		return CloudSyncDownloadResult{}, err
	}
	var synced CloudSyncSyncedFile
	if err := json.Unmarshal(body, &synced); err != nil {
		return CloudSyncDownloadResult{}, fmt.Errorf("decode synced file: %w", err)
	}
	synced.ETag = etag
	return CloudSyncDownloadResult{SyncedFile: &synced}, nil
}

// CloudSyncWebdavDelete removes the remote snapshot.
func (s *SyncService) CloudSyncWebdavDelete(config CloudSyncWebDAVConfig) (CloudSyncDeleteResult, error) {
	client, err := webdavClient(config)
	if err != nil {
		return CloudSyncDeleteResult{}, err
	}
	if err := client.DeleteSnapshot(context.Background()); err != nil {
		return CloudSyncDeleteResult{}, err
	}
	return CloudSyncDeleteResult{OK: true}, nil
}

// CloudSyncS3Initialize resolves the current remote resource id (the strong
// ETag) or null when no snapshot exists yet, under the current name or the
// legacy pre-rename name.
func (s *SyncService) CloudSyncS3Initialize(config json.RawMessage) (CloudSyncResource, error) {
	client, key, legacyKey, err := s3ClientFromConfig(config)
	if err != nil {
		return CloudSyncResource{}, err
	}
	_, etag, err := s3ResolveActiveKey(context.Background(), &client, key, legacyKey)
	if err != nil {
		return CloudSyncResource{}, err
	}
	if etag == "" {
		return CloudSyncResource{}, nil
	}
	resource := etag
	return CloudSyncResource{ResourceID: &resource}, nil
}

// CloudSyncS3Upload stores the synced file in the bucket. The write target is
// the key the snapshot currently lives at (current name first, legacy
// fallback) so pre-rename snapshots keep being updated in place; the synced
// file's ETag is only sent as If-Match for that resolved key, and never
// against a missing object.
func (s *SyncService) CloudSyncS3Upload(config json.RawMessage, syncedFile CloudSyncSyncedFile) (CloudSyncResource, error) {
	client, key, legacyKey, err := s3ClientFromConfig(config)
	if err != nil {
		return CloudSyncResource{}, err
	}
	activeKey, activeETag, err := s3ResolveActiveKey(context.Background(), &client, key, legacyKey)
	if err != nil {
		return CloudSyncResource{}, err
	}
	ifMatch := ""
	if activeETag != "" && syncedFile.ETag != "" {
		ifMatch = syncedFile.ETag
	}
	body, err := json.Marshal(syncedFile)
	if err != nil {
		return CloudSyncResource{}, err
	}
	etag, err := client.PutObject(context.Background(), activeKey, body, ifMatch)
	if err != nil {
		return CloudSyncResource{}, err
	}
	return CloudSyncResource{ResourceID: &etag}, nil
}

// CloudSyncS3Download fetches the remote synced file or null, reading the
// current name first and the legacy pre-rename name as fallback.
func (s *SyncService) CloudSyncS3Download(config json.RawMessage) (CloudSyncDownloadResult, error) {
	client, key, legacyKey, err := s3ClientFromConfig(config)
	if err != nil {
		return CloudSyncDownloadResult{}, err
	}
	body, etag, err := client.GetObject(context.Background(), key)
	if errors.Is(err, cloudsync.ErrNotFound) {
		body, etag, err = client.GetObject(context.Background(), legacyKey)
	}
	if err != nil {
		if errors.Is(err, cloudsync.ErrNotFound) {
			return CloudSyncDownloadResult{}, nil
		}
		return CloudSyncDownloadResult{}, err
	}
	var synced CloudSyncSyncedFile
	if err := json.Unmarshal(body, &synced); err != nil {
		return CloudSyncDownloadResult{}, fmt.Errorf("decode synced file: %w", err)
	}
	synced.ETag = etag
	return CloudSyncDownloadResult{SyncedFile: &synced}, nil
}

// CloudSyncS3Delete removes the remote snapshot. The user explicitly asked
// to drop the remote snapshot, so both the current and the legacy key are
// removed and a missing object counts as removed.
func (s *SyncService) CloudSyncS3Delete(config json.RawMessage) (CloudSyncDeleteResult, error) {
	client, key, legacyKey, err := s3ClientFromConfig(config)
	if err != nil {
		return CloudSyncDeleteResult{}, err
	}
	for _, target := range []string{key, legacyKey} {
		if err := client.DeleteObject(context.Background(), target); err != nil && !errors.Is(err, cloudsync.ErrNotFound) {
			return CloudSyncDeleteResult{}, err
		}
	}
	return CloudSyncDeleteResult{OK: true}, nil
}

// s3ResolveActiveKey returns the key the snapshot currently lives at (current
// name first, legacy fallback) plus its current ETag; (currentKey, "") when
// neither object exists.
func s3ResolveActiveKey(ctx context.Context, client *cloudsync.S3Client, currentKey, legacyKey string) (string, string, error) {
	etag, err := client.HeadObject(ctx, currentKey)
	if err == nil {
		return currentKey, etag, nil
	}
	if !errors.Is(err, cloudsync.ErrNotFound) {
		return "", "", err
	}
	etag, err = client.HeadObject(ctx, legacyKey)
	if err == nil {
		return legacyKey, etag, nil
	}
	if !errors.Is(err, cloudsync.ErrNotFound) {
		return "", "", err
	}
	return currentKey, "", nil
}

// s3SnapshotFileName matches SYNC_CONSTANTS.SYNC_FILE_NAME in the renderer so
// the Go transport and the renderer fallback address the same object, and
// legacyS3SnapshotFileName keeps pre-rename cloud snapshots discoverable
// (read/write fallback only; new snapshots are created under the new name).
const (
	s3SnapshotFileName       = "lemonssh-vault.json"
	legacyS3SnapshotFileName = "netcatty-vault.json"
)

// s3SnapshotKey resolves the object key for one optional prefix. Leading and
// trailing whitespace/slashes are trimmed, matching the renderer's
// getObjectKey; an empty prefix addresses the bucket root.
func s3SnapshotKey(prefix string) string {
	return s3ObjectKey(prefix, s3SnapshotFileName)
}

// s3LegacySnapshotKey is the pre-rename object key for one optional prefix.
func s3LegacySnapshotKey(prefix string) string {
	return s3ObjectKey(prefix, legacyS3SnapshotFileName)
}

func s3ObjectKey(prefix, fileName string) string {
	trimmed := strings.Trim(strings.TrimSpace(prefix), "/")
	if trimmed == "" {
		return fileName
	}
	return trimmed + "/" + fileName
}

// s3Settings is the parsed renderer S3 config.
type s3Settings struct {
	endpoint        string
	region          string
	bucket          string
	accessKeyID     string
	secretAccessKey string
	sessionToken    string
	prefix          string
	usePathStyle    bool
	allowInsecure   bool
}

func parseS3Settings(config json.RawMessage) (s3Settings, error) {
	var parsed struct {
		Endpoint        string `json:"endpoint"`
		Region          string `json:"region"`
		Bucket          string `json:"bucket"`
		AccessKeyID     string `json:"accessKeyId"`
		SecretAccessKey string `json:"secretAccessKey"`
		// SecretKey is a legacy alias so older stored configs keep working.
		SecretKey    string `json:"secretKey,omitempty"`
		SessionToken string `json:"sessionToken,omitempty"`
		Prefix       string `json:"prefix,omitempty"`
		// ForcePathStyle is a pointer so an explicit false (virtual-host
		// style) survives; absent defaults to true like the renderer fallback
		// (forcePathStyle ?? true), which keeps MinIO/localhost endpoints
		// working out of the box.
		ForcePathStyle *bool `json:"forcePathStyle,omitempty"`
		AllowInsecure  bool  `json:"allowInsecure,omitempty"`
	}
	if err := json.Unmarshal(config, &parsed); err != nil {
		return s3Settings{}, fmt.Errorf("s3 config: %w", err)
	}
	secret := parsed.SecretAccessKey
	if secret == "" {
		secret = parsed.SecretKey
	}
	usePathStyle := true
	if parsed.ForcePathStyle != nil {
		usePathStyle = *parsed.ForcePathStyle
	}
	return s3Settings{
		endpoint:        parsed.Endpoint,
		region:          parsed.Region,
		bucket:          parsed.Bucket,
		accessKeyID:     parsed.AccessKeyID,
		secretAccessKey: secret,
		sessionToken:    parsed.SessionToken,
		prefix:          parsed.Prefix,
		usePathStyle:    usePathStyle,
		allowInsecure:   parsed.AllowInsecure,
	}, nil
}

// snapshotKey is the object key this config stores the snapshot under.
func (s s3Settings) snapshotKey() string {
	return s3SnapshotKey(s.prefix)
}

func s3ClientFromConfig(config json.RawMessage) (cloudsync.S3Client, string, string, error) {
	settings, err := parseS3Settings(config)
	if err != nil {
		return cloudsync.S3Client{}, "", "", err
	}
	client := *cloudsync.NewS3Client(cloudsync.S3Config{
		Endpoint:        settings.endpoint,
		Region:          settings.region,
		Bucket:          settings.bucket,
		AccessKeyID:     settings.accessKeyID,
		SecretAccessKey: settings.secretAccessKey,
		SessionToken:    settings.sessionToken,
		UsePathStyle:    settings.usePathStyle,
		AllowInsecure:   settings.allowInsecure,
	})
	return client, settings.snapshotKey(), s3LegacySnapshotKey(settings.prefix), nil
}
