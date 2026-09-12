package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/binaricat/netcatty/internal/platform/cloudsync"
)

// These methods mirror the existing cloud-sync bridge. Wails injects context;
// the renderer supplies only the JSON options shown by the domain types.
func (s *SyncService) PrepareOAuthCallback() (cloudsync.CallbackSession, error) {
	return s.callbacks.Prepare()
}
func (s *SyncService) AwaitOAuthCallback(ctx context.Context, expectedState, sessionID string) (cloudsync.CallbackResult, error) {
	return s.callbacks.Wait(ctx, expectedState, sessionID)
}
func (s *SyncService) CancelOAuthCallback(sessionID string) { s.callbacks.Cancel(sessionID) }

// Map only OAuth browser handoffs here; the coordinator owns general URL opening.
func (s *SyncService) OpenOAuthExternal(ctx context.Context, rawURL string) error {
	if err := cloudsync.ValidateOAuthExternal(rawURL); err != nil {
		return err
	}
	return openExternalBrowser(ctx, rawURL)
}

// providerConsoleURLs are the exact registration pages offered in Settings.
// The bridge takes a provider name, never a raw URL, so no arbitrary link
// can turn the app into a URL opener.
var providerConsoleURLs = map[string]string{
	"github":   "https://github.com/settings/developers",
	"google":   "https://console.cloud.google.com/apis/credentials",
	"onedrive": "https://portal.azure.com/#blade/Microsoft_AAD_RegisteredApps/ApplicationsListBlade",
}

// OpenProviderConsole opens the OAuth application registration page for a
// provider in the system browser.
func (s *SyncService) OpenProviderConsole(ctx context.Context, provider string) error {
	rawURL, ok := providerConsoleURLs[provider]
	if !ok {
		return fmt.Errorf("unknown provider %q", provider)
	}
	return openExternalBrowser(ctx, rawURL)
}

// openExternalLauncher is a seam so tests can intercept browser launches.
var openExternalLauncher = func(ctx context.Context, rawURL string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.CommandContext(ctx, "rundll32.exe", "url.dll,FileProtocolHandler", rawURL)
	case "darwin":
		command = exec.CommandContext(ctx, "open", rawURL)
	default:
		command = exec.CommandContext(ctx, "xdg-open", rawURL)
	}
	if err := command.Run(); err != nil {
		return errors.New("Unable to open OAuth browser")
	}
	return nil
}

func openExternalBrowser(ctx context.Context, rawURL string) error {
	return openExternalLauncher(ctx, rawURL)
}

func (s *SyncService) GithubStartDeviceFlow(ctx context.Context, options cloudsync.DeviceOptions) (cloudsync.DeviceCode, error) {
	return s.oauth.StartDevice(ctx, options)
}
func (s *SyncService) GithubPollDeviceFlowToken(ctx context.Context, options cloudsync.DeviceOptions) (cloudsync.DeviceToken, error) {
	return s.oauth.PollDevice(ctx, options)
}
func (s *SyncService) GithubCancelDeviceFlowPoll(pollID string) { s.oauth.CancelDevice(pollID) }
func (s *SyncService) GithubDownloadGistRawContent(ctx context.Context, options cloudsync.RawGistOptions) (string, error) {
	return s.oauth.GitHubRaw(ctx, options)
}
func (s *SyncService) GithubGetUserInfo(ctx context.Context, options cloudsync.UserOptions) (cloudsync.UserInfo, error) {
	return s.oauth.User(ctx, "github", options)
}
func (s *SyncService) GithubFindSyncFile(ctx context.Context, options cloudsync.FileOptions) (cloudsync.FileResult, error) {
	return s.oauth.GitHubFind(ctx, options)
}
func (s *SyncService) GithubUploadSyncFile(ctx context.Context, options cloudsync.FileOptions) (cloudsync.FileResult, error) {
	return s.oauth.GitHubUpload(ctx, options)
}
func (s *SyncService) GithubDownloadSyncFile(ctx context.Context, options cloudsync.FileOptions) (cloudsync.DownloadResult, error) {
	return s.oauth.GitHubDownload(ctx, options)
}
func (s *SyncService) GithubDeleteSyncFile(ctx context.Context, options cloudsync.FileOptions) (cloudsync.OKResult, error) {
	return s.oauth.GitHubDelete(ctx, options)
}
func (s *SyncService) GithubGetGistHistory(ctx context.Context, options cloudsync.FileOptions) ([]cloudsync.GistRevision, error) {
	return s.oauth.GitHubHistory(ctx, options)
}

func (s *SyncService) GoogleExchangeCodeForTokens(ctx context.Context, options cloudsync.OAuthOptions) (cloudsync.OAuthTokens, error) {
	return s.oauth.Tokens(ctx, "google", options, false)
}
func (s *SyncService) GoogleRefreshAccessToken(ctx context.Context, options cloudsync.OAuthOptions) (cloudsync.OAuthTokens, error) {
	return s.oauth.Tokens(ctx, "google", options, true)
}
func (s *SyncService) GoogleGetUserInfo(ctx context.Context, options cloudsync.UserOptions) (cloudsync.UserInfo, error) {
	return s.oauth.User(ctx, "google", options)
}
func (s *SyncService) GoogleDriveFindSyncFile(ctx context.Context, options cloudsync.FileOptions) (cloudsync.FileResult, error) {
	return s.oauth.GoogleFind(ctx, options)
}
func (s *SyncService) GoogleDriveCreateSyncFile(ctx context.Context, options cloudsync.FileOptions) (cloudsync.FileResult, error) {
	return s.oauth.GoogleCreate(ctx, options)
}
func (s *SyncService) GoogleDriveUpdateSyncFile(ctx context.Context, options cloudsync.FileOptions) (cloudsync.OKResult, error) {
	return s.oauth.GoogleUpdate(ctx, options)
}
func (s *SyncService) GoogleDriveDownloadSyncFile(ctx context.Context, options cloudsync.FileOptions) (cloudsync.DownloadResult, error) {
	return s.oauth.GoogleDownload(ctx, options)
}
func (s *SyncService) GoogleDriveDeleteSyncFile(ctx context.Context, options cloudsync.FileOptions) (cloudsync.OKResult, error) {
	return s.oauth.GoogleDelete(ctx, options)
}

func (s *SyncService) OnedriveExchangeCodeForTokens(ctx context.Context, options cloudsync.OAuthOptions) (cloudsync.OAuthTokens, error) {
	return s.oauth.Tokens(ctx, "onedrive", options, false)
}
func (s *SyncService) OnedriveRefreshAccessToken(ctx context.Context, options cloudsync.OAuthOptions) (cloudsync.OAuthTokens, error) {
	return s.oauth.Tokens(ctx, "onedrive", options, true)
}
func (s *SyncService) OnedriveGetUserInfo(ctx context.Context, options cloudsync.UserOptions) (cloudsync.UserInfo, error) {
	return s.oauth.User(ctx, "onedrive", options)
}
func (s *SyncService) OnedriveFindSyncFile(ctx context.Context, options cloudsync.FileOptions) (cloudsync.FileResult, error) {
	return s.oauth.OneDriveFind(ctx, options)
}
func (s *SyncService) OnedriveUploadSyncFile(ctx context.Context, options cloudsync.FileOptions) (cloudsync.FileResult, error) {
	return s.oauth.OneDriveUpload(ctx, options)
}
func (s *SyncService) OnedriveDownloadSyncFile(ctx context.Context, options cloudsync.FileOptions) (cloudsync.DownloadResult, error) {
	return s.oauth.OneDriveDownload(ctx, options)
}
func (s *SyncService) OnedriveDeleteSyncFile(ctx context.Context, options cloudsync.FileOptions) (cloudsync.OKResult, error) {
	return s.oauth.OneDriveDelete(ctx, options)
}
