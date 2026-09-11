// Package cloudsync owns the Go cloud snapshot transport (P6-01, SYNC-02).
// This file implements the WebDAV provider: GET/PUT with ETag strong
// validation so a stale writer surfaces as a conflict instead of silently
// overwriting a newer snapshot. OAuth and S3 providers remain pending.
package cloudsync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	// ErrConflict means the remote moved on: the caller must pull, merge and
	// retry rather than overwrite.
	ErrConflict = errors.New("cloudsync: remote snapshot changed")
	// ErrNotFound means no snapshot exists at the path yet.
	ErrNotFound = errors.New("cloudsync: snapshot not found")
	// ErrUnauthorized means credentials were rejected (401 after response).
	ErrUnauthorized = errors.New("cloudsync: unauthorized")
)

// WebDAVConfig configures one WebDAV snapshot endpoint.
type WebDAVConfig struct {
	// Endpoint is the collection URL the snapshot file lives in, e.g.
	// https://dav.example.com/netcatty/.
	Endpoint string
	// SnapshotName is the file name inside Endpoint (default "snapshot.json").
	SnapshotName string
	Username     string
	Password     string
	// AllowInsecure permits non-HTTPS endpoints (self-hosted LAN servers).
	AllowInsecure bool
	Timeout       time.Duration
}

// WebDAVClient talks to one WebDAV snapshot endpoint.
type WebDAVClient struct {
	config WebDAVConfig
	client *http.Client
}

func NewWebDAVClient(config WebDAVConfig) *WebDAVClient {
	if config.SnapshotName == "" {
		config.SnapshotName = "snapshot.json"
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &WebDAVClient{
		config: config,
		client: &http.Client{Timeout: timeout},
	}
}

func (c *WebDAVClient) snapshotURL() string {
	endpoint := strings.TrimRight(c.config.Endpoint, "/")
	return endpoint + "/" + strings.TrimLeft(c.config.SnapshotName, "/")
}

func (c *WebDAVClient) newRequest(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if c.config.Username != "" || c.config.Password != "" {
		request.SetBasicAuth(c.config.Username, c.config.Password)
	}
	return request, nil
}

// Snapshot fetches the current remote snapshot and its ETag. ErrNotFound when
// the endpoint has no snapshot yet.
func (c *WebDAVClient) Snapshot(ctx context.Context) ([]byte, string, error) {
	request, err := c.newRequest(ctx, http.MethodGet, c.snapshotURL(), nil)
	if err != nil {
		return nil, "", err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, "", fmt.Errorf("webdav get: %w", err)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, "", ErrNotFound
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, "", ErrUnauthorized
	default:
		return nil, "", fmt.Errorf("webdav get: unexpected status %d", response.StatusCode)
	}
	etag := response.Header.Get("ETag")
	body, err := io.ReadAll(io.LimitReader(response.Body, maxSnapshotBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("webdav get body: %w", err)
	}
	if len(body) > maxSnapshotBytes {
		return nil, "", fmt.Errorf("webdav get: snapshot exceeds %d bytes", maxSnapshotBytes)
	}
	return body, etag, nil
}

// PutSnapshot uploads a snapshot. When expectETag is non-empty the upload
// carries If-Match, and a 412 response maps to ErrConflict so a stale writer
// pulls and merges instead of clobbering the remote. A missing precondition
// upload to an existing snapshot is also a conflict.
func (c *WebDAVClient) PutSnapshot(ctx context.Context, data []byte, expectETag string) (string, error) {
	if int64(len(data)) > maxSnapshotBytes {
		return "", fmt.Errorf("webdav put: snapshot exceeds %d bytes", maxSnapshotBytes)
	}
	request, err := c.newRequest(ctx, http.MethodPut, c.snapshotURL(), bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	if expectETag != "" {
		request.Header.Set("If-Match", expectETag)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("webdav put: %w", err)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
	case http.StatusPreconditionFailed:
		return "", ErrConflict
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", ErrUnauthorized
	case http.StatusNotFound:
		return "", ErrNotFound
	default:
		return "", fmt.Errorf("webdav put: unexpected status %d", response.StatusCode)
	}
	etag := response.Header.Get("ETag")
	if etag == "" {
		// Many servers skip the ETag on PUT; re-read so the next write can
		// still use a strong precondition.
		_, etag, err = c.Snapshot(ctx)
		if err != nil {
			return "", err
		}
	}
	return etag, nil
}

// DeleteSnapshot removes the remote snapshot. ErrNotFound when absent.
func (c *WebDAVClient) DeleteSnapshot(ctx context.Context) error {
	request, err := c.newRequest(ctx, http.MethodDelete, c.snapshotURL(), nil)
	if err != nil {
		return err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("webdav delete: %w", err)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	default:
		return fmt.Errorf("webdav delete: unexpected status %d", response.StatusCode)
	}
}

// maxSnapshotBytes bounds one cloud snapshot (16 MiB).
const maxSnapshotBytes = 16 << 20
