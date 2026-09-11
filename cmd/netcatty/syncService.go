package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/binaricat/netcatty/internal/platform/cloudsync"
	"github.com/binaricat/netcatty/internal/syncengine"
)

type SyncService struct{}

func newSyncService() *SyncService { return &SyncService{} }

func (s *SyncService) Merge(local, remote map[string]syncengine.Entry) map[string]syncengine.Entry {
	return syncengine.Merge(local, remote)
}

func (s *SyncService) Fingerprint(entries map[string]syncengine.Entry) string {
	return syncengine.Fingerprint(entries)
}

// CloudSyncWebDAVConfig mirrors the renderer's WebDAVConfig. Token-based auth
// is fail-closed: the Go WebDAV transport implements basic auth only.
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
	if strings.EqualFold(config.AuthType, "token") && config.Token != "" && config.Username == "" {
		return cloudsync.WebDAVClient{}, fmt.Errorf("token auth is not supported by the Go WebDAV transport yet; use username/password")
	}
	client := cloudsync.NewWebDAVClient(cloudsync.WebDAVConfig{
		Endpoint:      config.Endpoint,
		Username:      config.Username,
		Password:      config.Password,
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
// ETag) or null when no snapshot exists yet.
func (s *SyncService) CloudSyncS3Initialize(config json.RawMessage) (CloudSyncResource, error) {
	client, err := s3ClientFromConfig(config)
	if err != nil {
		return CloudSyncResource{}, err
	}
	etag, headErr := client.HeadObject(context.Background(), snapshotKey)
	if headErr != nil {
		if headErr == cloudsync.ErrNotFound {
			return CloudSyncResource{}, nil
		}
		return CloudSyncResource{}, headErr
	}
	resource := etag
	return CloudSyncResource{ResourceID: &resource}, nil
}

// CloudSyncS3Upload stores the synced file in the bucket.
func (s *SyncService) CloudSyncS3Upload(config json.RawMessage, syncedFile CloudSyncSyncedFile) (CloudSyncResource, error) {
	client, err := s3ClientFromConfig(config)
	if err != nil {
		return CloudSyncResource{}, err
	}
	body, err := json.Marshal(syncedFile)
	if err != nil {
		return CloudSyncResource{}, err
	}
	etag, err := client.PutObject(context.Background(), snapshotKey, body, syncedFile.ETag)
	if err != nil {
		return CloudSyncResource{}, err
	}
	return CloudSyncResource{ResourceID: &etag}, nil
}

// CloudSyncS3Download fetches the remote synced file or null.
func (s *SyncService) CloudSyncS3Download(config json.RawMessage) (CloudSyncDownloadResult, error) {
	client, err := s3ClientFromConfig(config)
	if err != nil {
		return CloudSyncDownloadResult{}, err
	}
	body, etag, err := client.GetObject(context.Background(), snapshotKey)
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

// CloudSyncS3Delete removes the remote snapshot.
func (s *SyncService) CloudSyncS3Delete(config json.RawMessage) (CloudSyncDeleteResult, error) {
	client, err := s3ClientFromConfig(config)
	if err != nil {
		return CloudSyncDeleteResult{}, err
	}
	if err := client.DeleteObject(context.Background(), snapshotKey); err != nil {
		return CloudSyncDeleteResult{}, err
	}
	return CloudSyncDeleteResult{OK: true}, nil
}

// snapshotKey is the object key every provider stores the snapshot under.
const snapshotKey = "netcatty/snapshot.json"

func s3ClientFromConfig(config json.RawMessage) (cloudsync.S3Client, error) {
	var parsed struct {
		Endpoint      string `json:"endpoint"`
		Region        string `json:"region"`
		Bucket        string `json:"bucket"`
		AccessKeyID   string `json:"accessKeyId"`
		SecretKey     string `json:"secretKey"`
		SessionToken  string `json:"sessionToken,omitempty"`
		UsePathStyle  bool   `json:"usePathStyle,omitempty"`
		AllowInsecure bool   `json:"allowInsecure,omitempty"`
	}
	if err := json.Unmarshal(config, &parsed); err != nil {
		return cloudsync.S3Client{}, fmt.Errorf("s3 config: %w", err)
	}
	return *cloudsync.NewS3Client(cloudsync.S3Config{
		Endpoint:      parsed.Endpoint,
		Region:        parsed.Region,
		Bucket:        parsed.Bucket,
		AccessKeyID:   parsed.AccessKeyID,
		SecretAccessKey: parsed.SecretKey,
		SessionToken:  parsed.SessionToken,
		UsePathStyle:  parsed.UsePathStyle,
	}), nil
}
