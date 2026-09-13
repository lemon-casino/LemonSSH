package cloudsync

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

type CallbackSession struct {
	SessionID   string `json:"sessionId"`
	Port        int    `json:"port"`
	RedirectURI string `json:"redirectUri"`
}

type CallbackResult struct {
	Code  string `json:"code"`
	State string `json:"state,omitempty"`
}

type callbackAttempt struct {
	info    CallbackSession
	server  *http.Server
	ctx     context.Context
	cancel  context.CancelFunc
	state   string
	waiting bool
	claimed bool
	result  chan CallbackResult
	err     chan error
}

// CallbackServer owns bounded, single-use OAuth listeners. No callback values
// are logged, reflected into HTML, or written to disk.
type CallbackServer struct {
	mu       sync.Mutex
	sessions map[string]*callbackAttempt
	closed   bool
	timeout  time.Duration
}

func NewCallbackServer() *CallbackServer {
	return &CallbackServer{sessions: make(map[string]*callbackAttempt), timeout: 5 * time.Minute}
}

func (c *CallbackServer) Prepare() (CallbackSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return CallbackSession{}, errors.New("OAuth service closed")
	}
	if len(c.sessions) >= 8 {
		return CallbackSession{}, errors.New("Too many OAuth sessions")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:45678")
	if err != nil {
		listener, err = net.Listen("tcp4", "127.0.0.1:0")
	}
	if err != nil {
		return CallbackSession{}, errors.New("Unable to bind OAuth loopback listener")
	}
	var id [32]byte
	if _, err = rand.Read(id[:]); err != nil {
		listener.Close()
		return CallbackSession{}, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	info := CallbackSession{hex.EncodeToString(id[:]), port, fmt.Sprintf("http://127.0.0.1:%d/callback", port)}
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	a := &callbackAttempt{info: info, ctx: ctx, cancel: cancel, result: make(chan CallbackResult, 1), err: make(chan error, 1)}
	a.server = &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 8192}
	a.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		if r.Method != http.MethodGet || r.URL.Path != "/callback" || r.Host != listener.Addr().String() || len(r.RequestURI) > 8192 {
			http.Error(w, "Invalid callback", http.StatusBadRequest)
			return
		}
		q, err := url.ParseQuery(r.URL.RawQuery)
		c.mu.Lock()
		defer c.mu.Unlock()
		state := q.Get("state")
		if err != nil || len(q["state"]) != 1 || !a.waiting || a.state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(a.state)) != 1 {
			http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
			return
		}
		if q.Get("error") != "" {
			a.waiting = false
			http.Error(w, "Authorization denied. Return to LemonSSH.", http.StatusBadRequest)
			http.NewResponseController(w).Flush()
			a.err <- errors.New("OAuth authorization denied")
			return
		}
		if len(q["code"]) != 1 || q.Get("code") == "" || len(q.Get("code")) > 4096 {
			http.Error(w, "Invalid authorization code", http.StatusBadRequest)
			return
		}
		a.waiting = false
			fmt.Fprint(w, "Authorization received. Return to LemonSSH.")
		http.NewResponseController(w).Flush()
		a.result <- CallbackResult{q.Get("code"), state}
	})
	c.sessions[info.SessionID] = a
	go func() {
		if err := a.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			cancel()
		}
	}()
	go func() {
		<-ctx.Done()
		a.server.Close()
		c.mu.Lock()
		delete(c.sessions, info.SessionID)
		c.mu.Unlock()
	}()
	return info, nil
}

func (c *CallbackServer) Wait(ctx context.Context, state, sessionID string) (CallbackResult, error) {
	if len(state) < 16 || len(state) > 512 {
		return CallbackResult{}, errors.New("OAuth expected state is required")
	}
	c.mu.Lock()
	a := c.sessions[sessionID]
	if a == nil || a.claimed {
		c.mu.Unlock()
		return CallbackResult{}, errors.New("OAuth session unavailable")
	}
	a.state, a.waiting = state, true
	a.claimed = true
	c.mu.Unlock()
	defer c.Cancel(sessionID)
	select {
	case result := <-a.result:
		return result, nil
	case err := <-a.err:
		return CallbackResult{}, err
	case <-ctx.Done():
		return CallbackResult{}, errors.New("OAuth flow cancelled")
	case <-a.ctx.Done():
		if errors.Is(a.ctx.Err(), context.DeadlineExceeded) {
			return CallbackResult{}, errors.New("OAuth callback timeout")
		}
		return CallbackResult{}, errors.New("OAuth flow cancelled")
	}
}

func (c *CallbackServer) Cancel(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if a := c.sessions[sessionID]; a != nil {
		a.cancel()
		delete(c.sessions, sessionID)
	}
}

func (c *CallbackServer) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	for id, a := range c.sessions {
		a.cancel()
		a.server.Close()
		delete(c.sessions, id)
	}
}

func validRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.Path != "/callback" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	port, err := strconv.Atoi(u.Port())
	return err == nil && port > 0 && port <= 65535
}

// ValidateOAuthExternal restricts the browser handoff to built-in providers.
func ValidateOAuthExternal(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 8192 || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" {
		return errors.New("OAuth URL is not allowed")
	}
	switch u.Host + u.Path {
	case "github.com/login/device":
		if u.RawQuery != "" {
			return errors.New("OAuth URL is not allowed")
		}
		return nil
	case "accounts.google.com/o/oauth2/v2/auth", "login.microsoftonline.com/consumers/oauth2/v2.0/authorize":
		q := u.Query()
		if !validRedirect(q.Get("redirect_uri")) || q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" || len(q.Get("code_challenge")) != 43 || len(q.Get("state")) < 16 {
			return errors.New("Invalid PKCE authorization URL")
		}
		return nil
	default:
		return errors.New("OAuth URL is not allowed")
	}
}
