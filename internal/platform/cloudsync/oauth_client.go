package cloudsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const syncFileName = "netcatty-vault.json"
const gistDescription = "Netcatty Encrypted Vault (DO NOT EDIT MANUALLY)"
const oneDriveScope = "https://graph.microsoft.com/Files.ReadWrite.AppFolder https://graph.microsoft.com/User.Read offline_access"
const maxOAuthBytes = 64 << 10

type OAuthOptions struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret,omitempty"`
	Code         string `json:"code,omitempty"`
	CodeVerifier string `json:"codeVerifier,omitempty"`
	RedirectURI  string `json:"redirectUri,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

type OAuthTokens struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
	TokenType    string `json:"tokenType"`
	Scope        string `json:"scope,omitempty"`
}

type DeviceOptions struct {
	ClientID   string `json:"clientId,omitempty"`
	Scope      string `json:"scope,omitempty"`
	DeviceCode string `json:"deviceCode,omitempty"`
	PollID     string `json:"pollId,omitempty"`
}

type DeviceCode struct {
	DeviceCode      string `json:"deviceCode"`
	UserCode        string `json:"userCode"`
	VerificationURI string `json:"verificationUri"`
	ExpiresAt       int64  `json:"expiresAt"`
	Interval        int    `json:"interval"`
}

type DeviceToken struct {
	AccessToken      string `json:"access_token,omitempty"`
	TokenType        string `json:"token_type,omitempty"`
	Scope            string `json:"scope,omitempty"`
	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}

type UserOptions struct {
	AccessToken string `json:"accessToken"`
}
type UserInfo struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture,omitempty"`
}

// OAuthClient owns network policy. Endpoint overrides are deliberately private;
// httptest can replace them without exposing an arbitrary renderer HTTP proxy.
type OAuthClient struct {
	http                                                                                *http.Client
	googleToken, googleAPI, googleUpload, githubOAuth, githubAPI, graph, microsoftToken string
	mu                                                                                  sync.Mutex
	polls                                                                               map[string]context.CancelFunc
	ctx                                                                                 context.Context
	cancel                                                                              context.CancelFunc
}

func NewOAuthClient() *OAuthClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &OAuthClient{
		http:        &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		googleToken: "https://oauth2.googleapis.com/token", googleAPI: "https://www.googleapis.com", googleUpload: "https://www.googleapis.com/upload/drive/v3",
		githubOAuth: "https://github.com", githubAPI: "https://api.github.com", graph: "https://graph.microsoft.com/v1.0",
		microsoftToken: "https://login.microsoftonline.com/consumers/oauth2/v2.0/token",
		polls:          make(map[string]context.CancelFunc), ctx: ctx, cancel: cancel,
	}
}

func (c *OAuthClient) Close() { c.cancel(); c.http.CloseIdleConnections() }

func (c *OAuthClient) request(ctx context.Context, method, endpoint, token, contentType string, body []byte, etag string, limit int64) ([]byte, http.Header, int, error) {
	if int64(len(body)) > maxSnapshotBytes+maxOAuthBytes || len(token) > maxOAuthBytes {
		return nil, nil, 0, errors.New("Cloud request exceeds size limit")
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(c.ctx, cancel)
	defer stop()
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, nil, 0, errors.New("Invalid cloud request")
	}
	r.Header.Set("Accept", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	if etag != "" {
		r.Header.Set("If-Match", etag)
	}
	resp, err := c.http.Do(r)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, 0, errors.New("Cloud request cancelled")
		}
		// net/url errors include URLs; never return token-bearing URLs or bodies.
		return nil, nil, 0, errors.New("Cloud provider request failed or timed out")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, nil, resp.StatusCode, errors.New("Unable to read cloud response")
	}
	if int64(len(data)) > limit {
		return nil, nil, resp.StatusCode, errors.New("Cloud response exceeds size limit")
	}
	return data, resp.Header, resp.StatusCode, nil
}

func tokenErrorText(provider, code string) error {
	switch code {
	case "invalid_client":
		return errors.New("Google OAuth client is not a desktop app, or the client ID does not match. Create a Desktop app client and paste that client ID.")
	case "redirect_uri_mismatch":
		return errors.New("Google rejected the loopback redirect. Use a Desktop app client; LemonSSH redirects to http://127.0.0.1:<port>/callback.")
	case "invalid_grant":
		if provider == "google" {
			return errors.New("Google authorization code is invalid or already used (invalid_grant). Close the callback tab and connect Google again from LemonSSH.")
		}
		return errors.New("OAuth authorization code is invalid or already used (invalid_grant)")
	case "unauthorized_client":
		return errors.New("This Google client is not allowed to exchange authorization codes. Recreate it as a Desktop app.")
	case "invalid_request":
		if provider == "google" {
			return errors.New("Google rejected the token request (invalid_request). Desktop app clients still need the client secret from the downloaded JSON. Paste that client secret into LemonSSH settings and connect again.")
		}
		return errors.New("OAuth token request rejected: invalid_request")
	default:
		if code == "" {
			return errors.New("OAuth token request rejected")
		}
		return fmt.Errorf("OAuth token request rejected: %s", code)
	}
}

func statusError(status int) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == 404:
		return ErrNotFound
	case status == 412 || status == 409:
		return ErrConflict
	case status == 401:
		return fmt.Errorf("cloudsync: 401 unauthorized")
	default:
		return fmt.Errorf("Cloud provider HTTP %d", status)
	}
}

func (c *OAuthClient) form(ctx context.Context, endpoint string, values url.Values) ([]byte, int, error) {
	body := values.Encode()
	if len(body) > maxOAuthBytes {
		return nil, 0, errors.New("OAuth request exceeds size limit")
	}
	data, _, status, err := c.request(ctx, "POST", endpoint, "", "application/x-www-form-urlencoded", []byte(body), "", maxOAuthBytes)
	return data, status, err
}

var pkceVerifier = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)
var resourceID = regexp.MustCompile(`^[A-Za-z0-9_!-]{1,256}$`)

func (c *OAuthClient) Tokens(ctx context.Context, provider string, o OAuthOptions, refresh bool) (OAuthTokens, error) {
	if o.ClientID == "" || len(o.ClientID) > 512 {
		return OAuthTokens{}, errors.New("OAuth client ID is required")
	}
	endpoint := c.googleToken
	v := url.Values{"client_id": {o.ClientID}}
	switch provider {
	case "google":
		if o.ClientSecret != "" {
			v.Set("client_secret", o.ClientSecret)
		}
	case "onedrive":
		endpoint = c.microsoftToken
		v.Set("scope", oneDriveScope)
	default:
		return OAuthTokens{}, errors.New("OAuth provider is not allowed")
	}
	if refresh {
		if o.RefreshToken == "" {
			return OAuthTokens{}, errors.New("Refresh token is required")
		}
		v.Set("grant_type", "refresh_token")
		v.Set("refresh_token", o.RefreshToken)
	} else {
		if o.Code == "" || !pkceVerifier.MatchString(o.CodeVerifier) || !validRedirect(o.RedirectURI) {
			return OAuthTokens{}, errors.New("Invalid PKCE token exchange")
		}
		v.Set("grant_type", "authorization_code")
		v.Set("code", o.Code)
		v.Set("code_verifier", o.CodeVerifier)
		v.Set("redirect_uri", o.RedirectURI)
	}
	data, status, err := c.form(ctx, endpoint, v)
	if err != nil {
		return OAuthTokens{}, err
	}
	var wire struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
		ExpiresIn    int64  `json:"expires_in"`
		Error        string `json:"error"`
	}
	if json.Unmarshal(data, &wire) != nil {
		return OAuthTokens{}, errors.New("Invalid OAuth token response")
	}
	if provider == "onedrive" && refresh && (wire.Error == "invalid_grant" || wire.Error == "interaction_required" || wire.Error == "consent_required") {
		return OAuthTokens{}, errors.New("ONEDRIVE_REAUTH_REQUIRED: OneDrive session expired, please reconnect.")
	}
	if wire.Error != "" {
		return OAuthTokens{}, tokenErrorText(provider, wire.Error)
	}
	if err = statusError(status); err != nil {
		return OAuthTokens{}, err
	}
	if wire.AccessToken == "" {
		return OAuthTokens{}, errors.New("OAuth token request rejected")
	}
	if refresh && wire.RefreshToken == "" {
		wire.RefreshToken = o.RefreshToken
	}
	if wire.TokenType == "" {
		wire.TokenType = "Bearer"
	}
	var expiresAt int64
	if wire.ExpiresIn > 0 && wire.ExpiresIn <= 365*24*3600 {
		expiresAt = time.Now().UnixMilli() + wire.ExpiresIn*1000
	}
	return OAuthTokens{wire.AccessToken, wire.RefreshToken, expiresAt, wire.TokenType, wire.Scope}, nil
}

// deviceFlowErrorText maps GitHub device-flow error codes to actionable
// messages. Descriptions from the provider are never relayed.
func deviceFlowErrorText(code string) string {
	switch code {
	case "device_flow_disabled":
		return "Device flow is disabled for this GitHub application. Open its settings (Settings → Developer settings → GitHub Apps or OAuth Apps → Enable device flow), check it, save, and connect again."
	case "unverified_user_email":
		return "GitHub requires a verified email for device authorization. Verify your email on github.com and try again."
	default:
		return fmt.Sprintf("GitHub authorization failed: %s", code)
	}
}

func (c *OAuthClient) StartDevice(ctx context.Context, o DeviceOptions) (DeviceCode, error) {
	if o.ClientID == "" {
		return DeviceCode{}, errors.New("GitHub OAuth client ID is required")
	}
	data, status, err := c.form(ctx, c.githubOAuth+"/login/device/code", url.Values{"client_id": {o.ClientID}, "scope": {"gist read:user"}})
	if err != nil {
		return DeviceCode{}, err
	}
	// The start endpoint answers HTTP 400 with an error code in the JSON body
	// (device_flow_disabled when the app has not enabled device flow), so
	// parse the body before treating 400 as fatal.
	if status != 200 && status != 400 {
		return DeviceCode{}, statusError(status)
	}
	var wire struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
		Error           string `json:"error"`
	}
	if json.Unmarshal(data, &wire) != nil {
		return DeviceCode{}, errors.New("Invalid GitHub device response")
	}
	if wire.Error != "" {
		return DeviceCode{}, errors.New(deviceFlowErrorText(wire.Error))
	}
	if wire.DeviceCode == "" || wire.UserCode == "" || wire.ExpiresIn <= 0 || wire.ExpiresIn > 3600 || wire.VerificationURI != "https://github.com/login/device" {
		return DeviceCode{}, errors.New("Invalid GitHub device response")
	}
	return DeviceCode{wire.DeviceCode, wire.UserCode, wire.VerificationURI, time.Now().UnixMilli() + int64(wire.ExpiresIn)*1000, max(wire.Interval, 5)}, nil
}

func (c *OAuthClient) PollDevice(ctx context.Context, o DeviceOptions) (DeviceToken, error) {
	if o.ClientID == "" || o.DeviceCode == "" || len(o.PollID) > 256 {
		return DeviceToken{}, errors.New("Invalid GitHub device poll")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if o.PollID != "" {
		c.mu.Lock()
		if _, ok := c.polls[o.PollID]; ok || len(c.polls) >= 8 {
			c.mu.Unlock()
			return DeviceToken{}, errors.New("GitHub device poll already active")
		}
		c.polls[o.PollID] = cancel
		c.mu.Unlock()
		defer func() { c.mu.Lock(); delete(c.polls, o.PollID); c.mu.Unlock() }()
	}
	data, status, err := c.form(ctx, c.githubOAuth+"/login/oauth/access_token", url.Values{"client_id": {o.ClientID}, "device_code": {o.DeviceCode}, "grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}})
	if err != nil {
		return DeviceToken{}, err
	}
	// The device-flow token endpoint answers HTTP 400 with the real status in
	// the JSON body (authorization_pending while the user is still typing the
	// code), so parse the body before treating 400 as fatal.
	if status != 200 && status != 400 {
		return DeviceToken{}, statusError(status)
	}
	var result DeviceToken
	if json.Unmarshal(data, &result) != nil {
		return DeviceToken{}, errors.New("Invalid GitHub token response")
	}
	// Do not relay provider descriptions, which can echo credentials.
	result.ErrorDescription = ""
	switch result.Error {
	case "", "authorization_pending", "slow_down", "expired_token", "access_denied":
	default:
		return DeviceToken{}, errors.New("GitHub authorization failed")
	}
	if result.AccessToken == "" && result.Error == "" {
		return DeviceToken{}, errors.New("Invalid GitHub token response")
	}
	return result, nil
}

func (c *OAuthClient) CancelDevice(pollID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cancel := c.polls[pollID]; cancel != nil {
		cancel()
	}
}

func (c *OAuthClient) api(ctx context.Context, method, endpoint, token string, body []byte, etag string, out any) (http.Header, error) {
	if token == "" {
		return nil, errors.New("Access token is required")
	}
	data, headers, status, err := c.request(ctx, method, endpoint, token, "application/json", body, etag, maxSnapshotBytes)
	if err != nil {
		return nil, err
	}
	if err = statusError(status); err != nil {
		return nil, err
	}
	if out != nil && json.Unmarshal(data, out) != nil {
		return nil, errors.New("Invalid cloud provider JSON")
	}
	return headers, nil
}

func (c *OAuthClient) User(ctx context.Context, provider string, o UserOptions) (UserInfo, error) {
	var result UserInfo
	switch provider {
	case "google":
		_, err := c.api(ctx, "GET", c.googleAPI+"/oauth2/v2/userinfo", o.AccessToken, nil, "", &result)
		return result, err
	case "onedrive":
		var wire struct {
			ID        string `json:"id"`
			Name      string `json:"displayName"`
			Email     string `json:"mail"`
			Principal string `json:"userPrincipalName"`
		}
		_, err := c.api(ctx, "GET", c.graph+"/me?$select=id,displayName,mail,userPrincipalName", o.AccessToken, nil, "", &wire)
		if wire.Email == "" {
			wire.Email = wire.Principal
		}
		return UserInfo{ID: wire.ID, Name: wire.Name, Email: wire.Email}, err
	case "github":
		var wire struct {
			ID     json.Number `json:"id"`
			Name   string      `json:"name"`
			Login  string      `json:"login"`
			Email  string      `json:"email"`
			Avatar string      `json:"avatar_url"`
		}
		_, err := c.api(ctx, "GET", c.githubAPI+"/user", o.AccessToken, nil, "", &wire)
		if wire.Name == "" {
			wire.Name = wire.Login
		}
		return UserInfo{ID: string(wire.ID), Name: wire.Name, Email: wire.Email, Picture: wire.Avatar}, err
	default:
		return result, errors.New("Cloud provider is not allowed")
	}
}

func validFileName(name string) error {
	if name != "" && name != syncFileName {
		return errors.New("Only the Netcatty encrypted snapshot is allowed")
	}
	return nil
}

func allowedDownloadURL(raw string, github bool) bool {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 8192 || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" {
		return false
	}
	if github {
		return u.Host == "gist.githubusercontent.com" && strings.Contains(u.Path, "/raw/")
	}
	return u.Host == "public.dm.files.1drv.com" || strings.HasSuffix(u.Host, ".files.1drv.com") || strings.HasSuffix(u.Host, ".storage.live.com")
}

func (c *OAuthClient) downloadURL(ctx context.Context, raw string, github bool) ([]byte, error) {
	if !allowedDownloadURL(raw, github) {
		return nil, errors.New("Cloud download URL is not allowed")
	}
	// Pre-authenticated download URLs must never receive the Graph bearer token.
	data, _, status, err := c.request(ctx, "GET", raw, "", "", nil, "", maxSnapshotBytes)
	if err != nil {
		return nil, err
	}
	return data, statusError(status)
}
