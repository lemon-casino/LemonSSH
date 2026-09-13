package cloudsync

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

type SyncedFile struct {
	Meta    json.RawMessage `json:"meta"`
	Payload string          `json:"payload"`
	ETag    string          `json:"etag,omitempty"`
}

type FileOptions struct {
	AccessToken string      `json:"accessToken"`
	FileID      string      `json:"fileId,omitempty"`
	FileName    string      `json:"fileName,omitempty"`
	SyncedFile  *SyncedFile `json:"syncedFile,omitempty"`
	Revision    string      `json:"revision,omitempty"`
}
type FileResult struct {
	FileID *string `json:"fileId"`
	ETag   string  `json:"etag,omitempty"`
}
type DownloadResult struct {
	SyncedFile *SyncedFile `json:"syncedFile"`
}
type OKResult struct {
	OK bool `json:"ok"`
}
type GistRevision struct {
	Version string `json:"version"`
	Date    string `json:"date"`
}
type RawGistOptions struct {
	AccessToken string `json:"accessToken"`
	RawURL      string `json:"rawUrl"`
}

func snapshotBody(file *SyncedFile) ([]byte, error) {
	if file == nil || len(file.Payload) == 0 || len(file.Meta) == 0 || file.Meta[0] != '{' || !json.Valid(file.Meta) {
		return nil, errors.New("Invalid encrypted snapshot")
	}
	if len(file.Payload) > int(maxSnapshotBytes) {
		return nil, errors.New("Snapshot exceeds size limit")
	}
	if _, err := base64.StdEncoding.DecodeString(file.Payload); err != nil {
		return nil, errors.New("Invalid snapshot ciphertext")
	}
	body, err := json.Marshal(file)
	if int64(len(body)) > maxSnapshotBytes {
		return nil, errors.New("Snapshot exceeds size limit")
	}
	return body, err
}

func decodeSnapshot(body []byte, etag string) (DownloadResult, error) {
	var file SyncedFile
	if json.Unmarshal(body, &file) != nil {
		return DownloadResult{}, errors.New("Invalid encrypted snapshot JSON")
	}
	if _, err := snapshotBody(&file); err != nil {
		return DownloadResult{}, err
	}
	file.ETag = etag
	return DownloadResult{&file}, nil
}

func fileResult(id string) (FileResult, error) {
	if !resourceID.MatchString(id) {
		return FileResult{}, errors.New("Invalid cloud file ID")
	}
	return FileResult{FileID: &id}, nil
}

func (c *OAuthClient) GoogleFind(ctx context.Context, o FileOptions) (FileResult, error) {
	if err := validFileName(o.FileName); err != nil {
		return FileResult{}, err
	}
	q := url.Values{"spaces": {"appDataFolder"}, "q": {"trashed = false and name = '" + syncFileName + "'"}, "fields": {"files(id),nextPageToken"}, "pageSize": {"100"}}
	var result struct {
		Files []struct {
			ID string `json:"id"`
		} `json:"files"`
		NextPageToken string `json:"nextPageToken"`
	}
	_, err := c.api(ctx, "GET", c.googleAPI+"/drive/v3/files?"+q.Encode(), o.AccessToken, nil, "", &result)
	if err != nil {
		return FileResult{}, err
	}
	if len(result.Files) == 0 {
		return FileResult{}, nil
	}
	if result.NextPageToken != "" {
		return FileResult{}, errors.New("Too many Netcatty appDataFolder snapshots")
	}
	if o.FileID != "" {
		for _, file := range result.Files {
			if file.ID == o.FileID {
				return fileResult(file.ID)
			}
		}
		return FileResult{}, errors.New("Google file is outside the Netcatty appDataFolder")
	}
	return fileResult(result.Files[0].ID)
}

func (c *OAuthClient) googleFile(ctx context.Context, o FileOptions) (string, error) {
	if !resourceID.MatchString(o.FileID) {
		return "", errors.New("Invalid Google file ID")
	}
	found, err := c.GoogleFind(ctx, o)
	if err != nil {
		return "", err
	}
	if found.FileID == nil {
		return "", ErrNotFound
	}
	return c.googleAPI + "/drive/v3/files/" + o.FileID, nil
}

func (c *OAuthClient) GoogleCreate(ctx context.Context, o FileOptions) (FileResult, error) {
	if err := validFileName(o.FileName); err != nil {
		return FileResult{}, err
	}
	body, err := snapshotBody(o.SyncedFile)
	if err != nil {
		return FileResult{}, err
	}
	// JSON contains no literal CR/LF, so this boundary cannot appear as a MIME delimiter.
	const boundary = "netcatty_encrypted_snapshot"
	metadata := `{"name":"` + syncFileName + `","parents":["appDataFolder"]}`
	multipart := []byte("--" + boundary + "\r\nContent-Type: application/json; charset=UTF-8\r\n\r\n" + metadata + "\r\n--" + boundary + "\r\nContent-Type: application/json\r\n\r\n" + string(body) + "\r\n--" + boundary + "--\r\n")
	data, _, status, err := c.request(ctx, "POST", c.googleUpload+"/files?uploadType=multipart&fields=id", o.AccessToken, "multipart/related; boundary="+boundary, multipart, "", maxOAuthBytes)
	if err != nil {
		return FileResult{}, err
	}
	if err = statusError(status); err != nil {
		return FileResult{}, err
	}
	var result struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(data, &result) != nil {
		return FileResult{}, errors.New("Invalid Google file response")
	}
	return fileResult(result.ID)
}

func (c *OAuthClient) GoogleUpdate(ctx context.Context, o FileOptions) (OKResult, error) {
	body, err := snapshotBody(o.SyncedFile)
	if err != nil {
		return OKResult{}, err
	}
	if _, err = c.googleFile(ctx, o); err != nil {
		return OKResult{}, err
	}
	_, err = c.api(ctx, "PATCH", c.googleUpload+"/files/"+o.FileID+"?uploadType=media", o.AccessToken, body, o.SyncedFile.ETag, nil)
	return OKResult{err == nil}, err
}

func (c *OAuthClient) GoogleDownload(ctx context.Context, o FileOptions) (DownloadResult, error) {
	endpoint, err := c.googleFile(ctx, o)
	if errors.Is(err, ErrNotFound) {
		return DownloadResult{}, nil
	}
	if err != nil {
		return DownloadResult{}, err
	}
	if o.Revision != "" {
		if !resourceID.MatchString(o.Revision) {
			return DownloadResult{}, errors.New("Invalid Google revision")
		}
		data, _, status, err := c.request(ctx, "GET", endpoint+"/revisions/"+o.Revision+"?alt=media", o.AccessToken, "", nil, "", maxSnapshotBytes)
		if err != nil {
			return DownloadResult{}, err
		}
		if status == 404 {
			return DownloadResult{}, nil
		}
		if err = statusError(status); err != nil {
			return DownloadResult{}, err
		}
		return decodeSnapshot(data, "")
	}
	data, headers, status, err := c.request(ctx, "GET", endpoint+"?alt=media", o.AccessToken, "", nil, "", maxSnapshotBytes)
	if err != nil {
		return DownloadResult{}, err
	}
	if status == 404 {
		return DownloadResult{}, nil
	}
	if err = statusError(status); err != nil {
		return DownloadResult{}, err
	}
	return decodeSnapshot(data, headers.Get("ETag"))
}

func (c *OAuthClient) GoogleDelete(ctx context.Context, o FileOptions) (OKResult, error) {
	endpoint, err := c.googleFile(ctx, o)
	if errors.Is(err, ErrNotFound) {
		return OKResult{true}, nil
	}
	if err != nil {
		return OKResult{}, err
	}
	_, err = c.api(ctx, "DELETE", endpoint, o.AccessToken, nil, "", nil)
	if errors.Is(err, ErrNotFound) {
		err = nil
	}
	return OKResult{err == nil}, err
}

// GoogleRevisionHistory lists stored snapshots of the Drive sync file,
// newest first. Drive keeps at most 100 revisions per file; older uploads
// are silently pruned by Google, which is acceptable for a safety net.
func (c *OAuthClient) GoogleRevisionHistory(ctx context.Context, o FileOptions) ([]GistRevision, error) {
	if !resourceID.MatchString(o.FileID) {
		return nil, errors.New("Invalid Google file ID")
	}
	var wire struct {
		Revisions []struct {
			ID           string `json:"id"`
			ModifiedTime string `json:"modifiedTime"`
		} `json:"revisions"`
	}
	if _, err := c.api(ctx, "GET", c.googleAPI+"/drive/v3/files/"+o.FileID+"/revisions?pageSize=100", o.AccessToken, nil, "", &wire); err != nil {
		return nil, err
	}
	result := make([]GistRevision, 0, len(wire.Revisions))
	for _, revision := range wire.Revisions {
		result = append(result, GistRevision{Version: revision.ID, Date: revision.ModifiedTime})
	}
	return result, nil
}

const appRootSnapshot = "/me/drive/special/approot:/" + syncFileName

func (c *OAuthClient) OneDriveFind(ctx context.Context, o FileOptions) (FileResult, error) {
	if err := validFileName(o.FileName); err != nil {
		return FileResult{}, err
	}
	var item struct {
		ID   string `json:"id"`
		ETag string `json:"eTag"`
	}
	_, err := c.api(ctx, "GET", c.graph+appRootSnapshot, o.AccessToken, nil, "", &item)
	if errors.Is(err, ErrNotFound) {
		return FileResult{}, nil
	}
	if err != nil {
		return FileResult{}, err
	}
	if o.FileID != "" && o.FileID != item.ID {
		return FileResult{}, errors.New("OneDrive file is outside the Netcatty AppFolder")
	}
	result, err := fileResult(item.ID)
	result.ETag = item.ETag
	return result, err
}

func (c *OAuthClient) OneDriveUpload(ctx context.Context, o FileOptions) (FileResult, error) {
	if err := validFileName(o.FileName); err != nil {
		return FileResult{}, err
	}
	body, err := snapshotBody(o.SyncedFile)
	if err != nil {
		return FileResult{}, err
	}
	var item struct {
		ID string `json:"id"`
	}
	_, err = c.api(ctx, "PUT", c.graph+appRootSnapshot+":/content", o.AccessToken, body, o.SyncedFile.ETag, &item)
	if err != nil {
		return FileResult{}, err
	}
	return fileResult(item.ID)
}

func (c *OAuthClient) OneDriveDownload(ctx context.Context, o FileOptions) (DownloadResult, error) {
	found, err := c.OneDriveFind(ctx, o)
	if err != nil {
		return DownloadResult{}, err
	}
	if found.FileID == nil {
		return DownloadResult{}, nil
	}
	data, headers, status, err := c.request(ctx, "GET", c.graph+appRootSnapshot+":/content", o.AccessToken, "", nil, "", maxSnapshotBytes)
	if err != nil {
		return DownloadResult{}, err
	}
	etag := headers.Get("ETag")
	if etag == "" {
		etag = found.ETag
	}
	if status == http.StatusFound || status == http.StatusTemporaryRedirect {
		data, err = c.downloadURL(ctx, headers.Get("Location"), false)
	} else {
		err = statusError(status)
	}
	if errors.Is(err, ErrNotFound) {
		return DownloadResult{}, nil
	}
	if err != nil {
		return DownloadResult{}, err
	}
	return decodeSnapshot(data, etag)
}

func (c *OAuthClient) OneDriveDelete(ctx context.Context, o FileOptions) (OKResult, error) {
	found, err := c.OneDriveFind(ctx, o)
	if err != nil {
		return OKResult{}, err
	}
	if found.FileID == nil {
		return OKResult{true}, nil
	}
	_, err = c.api(ctx, "DELETE", c.graph+"/me/drive/items/"+*found.FileID, o.AccessToken, nil, "", nil)
	if errors.Is(err, ErrNotFound) {
		err = nil
	}
	return OKResult{err == nil}, err
}

type gistFile struct {
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
	RawURL    string `json:"raw_url"`
	Size      int    `json:"size"`
}
type gist struct {
	ID          string              `json:"id"`
	Description string              `json:"description"`
	Files       map[string]gistFile `json:"files"`
	History     []struct {
		Version string `json:"version"`
		Date    string `json:"committed_at"`
	} `json:"history"`
}

func (c *OAuthClient) GitHubFind(ctx context.Context, o FileOptions) (FileResult, error) {
	for page := 1; page <= 100; page++ {
		var gists []gist
		_, err := c.api(ctx, "GET", fmt.Sprintf("%s/gists?per_page=100&page=%d", c.githubAPI, page), o.AccessToken, nil, "", &gists)
		if err != nil {
			return FileResult{}, err
		}
		for _, g := range gists {
			if _, ok := g.Files[syncFileName]; ok && g.Description == gistDescription {
				return fileResult(g.ID)
			}
		}
		if len(gists) < 100 {
			return FileResult{}, nil
		}
	}
	return FileResult{}, errors.New("GitHub gist listing exceeds page limit")
}

func (c *OAuthClient) githubGist(ctx context.Context, o FileOptions) (gist, http.Header, error) {
	var result gist
	if !resourceID.MatchString(o.FileID) {
		return result, nil, errors.New("Invalid GitHub gist ID")
	}
	endpoint := c.githubAPI + "/gists/" + o.FileID
	if o.Revision != "" {
		if !resourceID.MatchString(o.Revision) {
			return result, nil, errors.New("Invalid GitHub revision")
		}
		endpoint += "/" + o.Revision
	}
	headers, err := c.api(ctx, "GET", endpoint, o.AccessToken, nil, "", &result)
	if err != nil {
		return result, nil, err
	}
	if _, ok := result.Files[syncFileName]; !ok || result.Description != gistDescription {
		return result, nil, errors.New("Gist is not a Netcatty encrypted vault")
	}
	return result, headers, nil
}

func (c *OAuthClient) GitHubUpload(ctx context.Context, o FileOptions) (FileResult, error) {
	content, err := snapshotBody(o.SyncedFile)
	if err != nil {
		return FileResult{}, err
	}
	method, endpoint := "POST", c.githubAPI+"/gists"
	if o.FileID != "" {
		if _, _, err = c.githubGist(ctx, o); err != nil {
			return FileResult{}, err
		}
		method, endpoint = "PATCH", endpoint+"/"+o.FileID
	}
	body, err := json.Marshal(map[string]any{"description": gistDescription, "public": false, "files": map[string]any{syncFileName: map[string]string{"content": string(content)}}})
	if err != nil {
		return FileResult{}, err
	}
	var result gist
	_, err = c.api(ctx, method, endpoint, o.AccessToken, body, o.SyncedFile.ETag, &result)
	if err != nil {
		return FileResult{}, err
	}
	return fileResult(result.ID)
}

func (c *OAuthClient) GitHubDownload(ctx context.Context, o FileOptions) (DownloadResult, error) {
	g, headers, err := c.githubGist(ctx, o)
	if errors.Is(err, ErrNotFound) {
		return DownloadResult{}, nil
	}
	if err != nil {
		return DownloadResult{}, err
	}
	file := g.Files[syncFileName]
	data := []byte(file.Content)
	if file.Truncated || file.Size > len(data) || !json.Valid(data) {
		data, err = c.downloadURL(ctx, file.RawURL, true)
		if err != nil {
			return DownloadResult{}, err
		}
	}
	return decodeSnapshot(data, headers.Get("ETag"))
}

func (c *OAuthClient) GitHubDelete(ctx context.Context, o FileOptions) (OKResult, error) {
	_, _, err := c.githubGist(ctx, o)
	if errors.Is(err, ErrNotFound) {
		return OKResult{true}, nil
	}
	if err != nil {
		return OKResult{}, err
	}
	_, err = c.api(ctx, "DELETE", c.githubAPI+"/gists/"+o.FileID, o.AccessToken, nil, "", nil)
	if errors.Is(err, ErrNotFound) {
		err = nil
	}
	return OKResult{err == nil}, err
}

func (c *OAuthClient) GitHubHistory(ctx context.Context, o FileOptions) ([]GistRevision, error) {
	g, _, err := c.githubGist(ctx, o)
	if err != nil {
		return nil, err
	}
	result := make([]GistRevision, 0, len(g.History))
	for _, h := range g.History {
		result = append(result, GistRevision{h.Version, h.Date})
	}
	return result, nil
}

func (c *OAuthClient) GitHubRaw(ctx context.Context, o RawGistOptions) (string, error) {
	data, err := c.downloadURL(ctx, o.RawURL, true)
	return string(data), err
}
