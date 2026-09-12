package cloudsync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func callbackWaiting(t *testing.T, c *CallbackServer, id string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		a := c.sessions[id]
		ready := a != nil && a.waiting
		c.mu.Unlock()
		if ready {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("callback waiter did not start")
}

func TestOAuthCallbackStateCancellationTimeoutAndShutdown(t *testing.T) {
	c := NewCallbackServer()
	defer c.Close()
	first, err := c.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if first.Port == second.Port || !strings.HasPrefix(first.RedirectURI, "http://127.0.0.1:") {
		t.Fatal("listener not isolated on IPv4 loopback")
	}
	state := "fixture-state-0123456789"
	finished := make(chan error, 1)
	go func() {
		result, err := c.Wait(context.Background(), state, first.SessionID)
		if err == nil && (result.Code != "fixture-code" || result.State != state) {
			err = fmt.Errorf("unexpected callback result")
		}
		finished <- err
	}()
	callbackWaiting(t, c, first.SessionID)
	for _, query := range []string{"?state=wrong&code=fixture-code", "?state=" + state + "&state=other&code=fixture-code", "?code=fixture-code"} {
		resp, err := http.Get(first.RedirectURI + query)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("invalid state accepted: %d", resp.StatusCode)
		}
	}
	resp, err := http.Get(first.RedirectURI + "?state=" + state + "&code=fixture-code")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if _, err := c.Wait(context.Background(), state, first.SessionID); err == nil {
		t.Fatal("callback replay accepted")
	}
	go func() { _, err := c.Wait(context.Background(), state, second.SessionID); finished <- err }()
	callbackWaiting(t, c, second.SessionID)
	c.Cancel(second.SessionID)
	if err := <-finished; err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("cancel: %v", err)
	}
	c.timeout = 25 * time.Millisecond
	third, err := c.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Wait(context.Background(), state, third.SessionID); err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("timeout: %v", err)
	}
	c.Close()
	if _, err := c.Prepare(); err == nil {
		t.Fatal("prepare after shutdown")
	}
}

func TestOAuthExternalAllowlist(t *testing.T) {
	query := "?response_type=code&redirect_uri=http%3A%2F%2F127.0.0.1%3A45678%2Fcallback&code_challenge_method=S256&code_challenge=" + strings.Repeat("A", 43) + "&state=fixture-state-0123456789"
	for _, raw := range []string{"https://github.com/login/device", "https://accounts.google.com/o/oauth2/v2/auth" + query, "https://login.microsoftonline.com/consumers/oauth2/v2.0/authorize" + query} {
		if err := ValidateOAuthExternal(raw); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{"https://github.com.evil.test/login/device", "https://user@github.com/login/device", "file:///tmp/auth", "https://login.microsoftonline.com/common/oauth2/v2.0/authorize" + query, "https://accounts.google.com/o/oauth2/v2/auth?redirect_uri=http://localhost:45678"} {
		if ValidateOAuthExternal(raw) == nil {
			t.Fatalf("allowed %s", raw)
		}
	}
	for _, raw := range []string{"http://127.0.0.1/x", "https://evil.test/raw/x", "https://gist.githubusercontent.com.evil.test/u/raw/x", "https://user@public.dm.files.1drv.com/x"} {
		if allowedDownloadURL(raw, true) || allowedDownloadURL(raw, false) {
			t.Fatalf("download allowed %s", raw)
		}
	}
}

func TestOAuthTokensPKCERefreshAndRedactedFailures(t *testing.T) {
	for _, provider := range []string{"google", "onedrive"} {
		t.Run(provider, func(t *testing.T) {
			call := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call++
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				if r.Form.Get("client_id") != "fixture-client" {
					t.Error("client ID missing")
				}
				if call == 1 {
					if r.Form.Get("code_verifier") != strings.Repeat("v", 43) || r.Form.Get("redirect_uri") != "http://127.0.0.1:45678/callback" {
						t.Error("PKCE fields missing")
					}
				} else if r.Form.Get("refresh_token") != "old-refresh" {
					t.Error("refresh token missing")
				}
				if provider == "onedrive" && r.Form.Get("scope") != oneDriveScope {
					t.Error("personal account scopes missing")
				}
				if call == 3 {
					w.WriteHeader(400)
					io.WriteString(w, `{"error":"invalid_grant","error_description":"old-refresh must never be echoed"}`)
					return
				}
				refresh := ""
				if provider == "onedrive" || call == 1 {
					refresh = `,"refresh_token":"rotated-refresh"`
				}
				fmt.Fprint(w, `{"access_token":"fixture-access","token_type":"Bearer","expires_in":3600`+refresh+`}`)
			}))
			defer server.Close()
			c := NewOAuthClient()
			defer c.Close()
			c.googleToken, c.microsoftToken = server.URL, server.URL
			o := OAuthOptions{ClientID: "fixture-client", Code: "fixture-code", CodeVerifier: strings.Repeat("v", 43), RedirectURI: "http://127.0.0.1:45678/callback", RefreshToken: "old-refresh"}
			if _, err := c.Tokens(context.Background(), provider, o, false); err != nil {
				t.Fatal(err)
			}
			result, err := c.Tokens(context.Background(), provider, o, true)
			if err != nil {
				t.Fatal(err)
			}
			want := "old-refresh"
			if provider == "onedrive" {
				want = "rotated-refresh"
			}
			if result.RefreshToken != want || result.ExpiresAt <= time.Now().UnixMilli() {
				t.Fatalf("refresh result: %#v", result)
			}
			_, err = c.Tokens(context.Background(), provider, o, true)
			if err == nil || strings.Contains(err.Error(), "old-refresh") {
				t.Fatalf("provider description leaked: %v", err)
			}
			if provider == "onedrive" && !strings.Contains(err.Error(), "ONEDRIVE_REAUTH_REQUIRED") {
				t.Fatal("reauth marker missing")
			}
			o.RedirectURI = "https://evil.test/callback"
			if _, err = c.Tokens(context.Background(), provider, o, false); err == nil {
				t.Fatal("non-loopback exchange")
			}
		})
	}
}

func TestGitHubDeviceFlowPollCancelAndBounds(t *testing.T) {
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login/device/code" {
			io.WriteString(w, `{"device_code":"device","user_code":"USER","verification_uri":"https://github.com/login/device","expires_in":900,"interval":5}`)
			return
		}
		r.ParseForm()
		switch r.Form.Get("device_code") {
		case "cancel":
			started <- struct{}{}
			<-r.Context().Done()
		case "large":
			io.WriteString(w, strings.Repeat("x", maxOAuthBytes+1))
		case "success":
			io.WriteString(w, `{"access_token":"fixture-token","token_type":"bearer"}`)
		default:
			json.NewEncoder(w).Encode(DeviceToken{Error: "authorization_pending", ErrorDescription: "redacted"})
		}
	}))
	defer server.Close()
	c := NewOAuthClient()
	defer c.Close()
	c.githubOAuth = server.URL
	if result, err := c.StartDevice(context.Background(), DeviceOptions{ClientID: "fixture"}); err != nil || result.Interval != 5 {
		t.Fatalf("start: %v", err)
	}
	for _, code := range []string{"pending", "success"} {
		if _, err := c.PollDevice(context.Background(), DeviceOptions{ClientID: "fixture", DeviceCode: code, PollID: code}); err != nil {
			t.Fatal(err)
		}
	}
	finished := make(chan error, 1)
	go func() {
		_, err := c.PollDevice(context.Background(), DeviceOptions{ClientID: "fixture", DeviceCode: "cancel", PollID: "poll"})
		finished <- err
	}()
	<-started
	c.CancelDevice("poll")
	if err := <-finished; err == nil {
		t.Fatal("poll cancellation ignored")
	}
	if _, err := c.PollDevice(context.Background(), DeviceOptions{ClientID: "fixture", DeviceCode: "large"}); err == nil {
		t.Fatal("unbounded response")
	}
}

func TestOAuthClientShutdownCancelsRequests(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	c := NewOAuthClient()
	c.googleAPI = server.URL
	finished := make(chan error, 1)
	go func() {
		_, err := c.User(context.Background(), "google", UserOptions{AccessToken: "fixture"})
		finished <- err
	}()
	<-started
	c.Close()
	if err := <-finished; err == nil {
		t.Fatal("shutdown did not cancel provider request")
	}
}

// GitHub answers the device-flow token poll with HTTP 400 while the user has
// not completed the browser step; the real status lives in the JSON body and
// must reach the polling loop instead of surfacing as a fatal 400.
func TestOAuthPollDeviceTreats400AsPending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DeviceToken{Error: "authorization_pending"})
	}))
	defer server.Close()
	c := NewOAuthClient()
	defer c.Close()
	c.githubOAuth = server.URL
	result, err := c.PollDevice(context.Background(), DeviceOptions{ClientID: "fixture", DeviceCode: "device", PollID: "pending400"})
	if err != nil {
		t.Fatalf("authorization_pending 400 must not be fatal: %v", err)
	}
	if result.Error != "authorization_pending" {
		t.Fatalf("pending result = %+v", result)
	}

	wrote := false
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !wrote {
			wrote = true
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":"slow_down"}`)
			return
		}
		io.WriteString(w, `{"access_token":"fixture-token","token_type":"bearer"}`)
	}))
	defer server2.Close()
	c2 := NewOAuthClient()
	defer c2.Close()
	c2.githubOAuth = server2.URL
	// One poll per call: slow_down is returned to the loop, which re-polls.
	slow, err := c2.PollDevice(context.Background(), DeviceOptions{ClientID: "fixture", DeviceCode: "device", PollID: "slow"})
	if err != nil || slow.Error != "slow_down" {
		t.Fatalf("slow_down poll: %v %+v", err, slow)
	}
	token, err := c2.PollDevice(context.Background(), DeviceOptions{ClientID: "fixture", DeviceCode: "device", PollID: "slow"})
	if err != nil || token.AccessToken == "" {
		t.Fatalf("success after slow_down: %v %+v", err, token)
	}
}

// The user-visible case from WV3-L129 feedback: a GitHub app without device
// flow enabled answers the start request with 400 device_flow_disabled; the
// error must surface as an actionable message, not a generic HTTP 400.
func TestOAuthStartDeviceSurfacesDisabledFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"device_flow_disabled","error_description":"Device Flow must be explicitly enabled for this App"}`)
	}))
	defer server.Close()
	c := NewOAuthClient()
	defer c.Close()
	c.githubOAuth = server.URL
	_, err := c.StartDevice(context.Background(), DeviceOptions{ClientID: "Ov23licrO6aqtR2h1WBC"})
	if err == nil || !strings.Contains(err.Error(), "Device flow is disabled") {
		t.Fatalf("device_flow_disabled must surface an actionable message, got %v", err)
	}
}
