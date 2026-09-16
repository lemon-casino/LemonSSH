package terminaluse

import (
	"context"
	"fmt"
	"time"

	"github.com/binaricat/netcatty/internal/terminal/telnet"
)

// TelnetStartRequest is the shell-facing telnet dial payload. Auto-login
// credentials are held in memory for the prompt exchange and never persisted.
type TelnetStartRequest struct {
	Hostname          string `json:"hostname"`
	Port              uint16 `json:"port"`
	Cols              uint16 `json:"cols"`
	Rows              uint16 `json:"rows"`
	Username          string `json:"username"`
	Password          string `json:"password"`
	AutoLogin         bool   `json:"autoLogin"`
	PromptTimeoutSecs int    `json:"promptTimeoutSecs"`
}

// StartTelnet dials a Telnet host and streams IAC-decoded data onto the same
// data plane as SSH/local PTY. Echo mode transitions are surfaced as events so
// the renderer can disable local echo when the server takes over; when
// auto-login is requested the prompt exchange runs against the live stream.
func (s *Service) StartTelnet(request TelnetStartRequest) (string, error) {
	host := request.Hostname
	port := request.Port
	cols := request.Cols
	rows := request.Rows
	if host == "" {
		return "", fmt.Errorf("host is required")
	}
	if port == 0 {
		port = 23
	}
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}
	s.mu.Lock()
	s.counter++
	sessionID := fmt.Sprintf("telnet-%d", s.counter)
	s.mu.Unlock()
	bootstrap, err := s.controller.Open(sessionID)
	if err != nil {
		return "", fmt.Errorf("open route: %w", err)
	}
	client, err := telnet.Connect(context.Background(), fmt.Sprintf("%s:%d", host, port), func(event telnet.Event) {
		switch event.Kind {
		case telnet.EventData:
			if len(event.Data) > 0 {
				if !s.publishOutput(sessionID, event.Data) {
					return
				}
			}
		case telnet.EventEchoMode:
			remote := string(event.Data) == "remote"
			s.emit("telnet:echo-mode", map[string]any{
				"sessionId":  sessionID,
				"remoteEcho": remote,
				"localEcho":  !remote,
			})
		case telnet.EventAutoLoginDone:
			s.emit("telnet:auto-login-complete", map[string]any{"sessionId": sessionID})
		case telnet.EventAutoLoginFail:
			s.emit("telnet:auto-login-cancelled", map[string]any{"sessionId": sessionID})
		case telnet.EventClosed:
			s.Close(sessionID)
		}
	})
	if err != nil {
		_ = s.controller.Close(sessionID)
		return "", err
	}
	_ = client.Resize(cols, rows)
	s.mu.Lock()
	s.sessions[sessionID] = &terminalSession{telnet: client, bootstrap: bootstrap}
	s.mu.Unlock()
	if request.AutoLogin && request.Username != "" {
		timeout := time.Duration(request.PromptTimeoutSecs) * time.Second
		if timeout <= 0 {
			timeout = 20 * time.Second
		}
		go func() {
			// AutoLogin emits the complete/cancelled event itself; the returned
			// error only carries the reason for diagnostics.
			_ = telnet.AutoLogin(context.Background(), client, request.Username, request.Password, timeout)
		}()
	}
	return sessionID, nil
}

// GetTelnetEchoMode reports whether the server currently echoes input.
func (s *Service) GetTelnetEchoMode(sessionID string) (map[string]any, error) {
	term, ok := s.lookup(sessionID)
	if !ok {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	if term.telnet == nil {
		return nil, fmt.Errorf("session %q is not a telnet session", sessionID)
	}
	remote := term.telnet.RemoteEcho()
	return map[string]any{
		"success":    true,
		"sessionId":  sessionID,
		"remoteEcho": remote,
		"localEcho":  !remote,
	}, nil
}
