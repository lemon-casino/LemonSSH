// Package sftpuse owns the shell-neutral SFTP use cases: subsystem session
// lifecycle over the shared SSH transport pool or an authenticated terminal
// transport, browsing (list/stat/mkdir/rename/remove/chmod), text read/write,
// file download/upload, remote zip extraction, and compressed folder upload.
//
// Per the Wails v3 migration (W04), this package is the canonical owner of the
// SFTP session pool and its transfer rules. Shell facades (Wails today,
// capability dispatch later) adapt it to their transport and must not copy or
// re-implement its behavior. It must never import a shell (Wails, Electron, UI).
package sftpuse

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/binaricat/netcatty/internal/app/terminaluse"
	"github.com/binaricat/netcatty/internal/platform/filesystem"
	"github.com/binaricat/netcatty/internal/terminal/sftp"
	"github.com/binaricat/netcatty/internal/terminal/ssh"
	"github.com/binaricat/netcatty/internal/terminal/sshpool"
	pkgsftp "github.com/pkg/sftp"
)

// OpenRequest dials (or borrows) a pooled transport for a standalone SFTP
// session. It embeds the canonical terminaluse.SSHConnectRequest so every
// shell shares one dial JSON contract.
type OpenRequest struct {
	terminaluse.SSHConnectRequest
	Sudo bool `json:"sudo"`
}

// Service owns SFTP sessions end-to-end: transport borrow (KindSFTP) or
// terminal transport reuse → subsystem client → bounded per-session operations
// → teardown back to the pool. Each Open call registers one SFTP subsystem
// client under an opaque session ID; operations are bounded by the per-session
// client limit.
type Service struct {
	temp              *filesystem.TempService
	mu                sync.Mutex
	pool              *sshpool.Pool
	knownHosts        *ssh.KnownHosts
	sessions          map[string]*client
	counter           int
	terminalTransport func(sessionID string) (*gossh.Client, func() bool, error)
	openStaging       func(path string) (*os.File, error)
}

// client is one registered SFTP subsystem.
type client struct {
	channel io.Closer
	fs      *sftp.ClientFS
	lease   *sshpool.Lease
	raw     *pkgsftp.Client
	bounded *sftp.Session
}

// New wires the pool and known-hosts store.
func New(pool *sshpool.Pool, knownHosts *ssh.KnownHosts) *Service {
	return &Service{
		pool:       pool,
		knownHosts: knownHosts,
		sessions:   make(map[string]*client),
	}
}

// SetTempService wires the managed temp directory used by archive staging.
func (s *Service) SetTempService(temp *filesystem.TempService) { s.temp = temp }

// SetTerminalTransport wires the terminal session seam used by
// OpenForTerminal. Facades pass terminaluse.Service.TransportFor so the
// subsystem opens on the exact authenticated terminal transport.
func (s *Service) SetTerminalTransport(transportFor func(sessionID string) (*gossh.Client, func() bool, error)) {
	s.terminalTransport = transportFor
}

// SetStagingOpener wires the local upload-source opener (extended-length path
// fallback plus transient-source retry in the desktop shell). Unwired services
// open the source directly.
func (s *Service) SetStagingOpener(open func(path string) (*os.File, error)) {
	s.openStaging = open
}

// Open dials (or borrows) a transport for host and registers an SFTP session.
func (s *Service) Open(request OpenRequest) (string, error) {
	if request.Hostname == "" || request.Username == "" {
		return "", fmt.Errorf("host and username are required")
	}
	if request.Port == 0 {
		request.Port = 22
	}
	config, err := ssh.BuildDialConfigErr(terminaluse.ConnectInputFromRequest(request.SSHConnectRequest), ssh.StrictPolicy(s.knownHosts), nil)
	if err != nil {
		return "", err
	}
	lease, err := s.pool.Get(context.Background(), config, sshpool.KindSFTP)
	if err != nil {
		return "", fmt.Errorf("ssh dial %s:%d: %w", request.Hostname, request.Port, err)
	}
	var raw *pkgsftp.Client
	var channel io.Closer
	if request.Sudo {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		raw, channel, err = sftp.OpenElevated(ctx, lease.Client())
	} else {
		raw, err = pkgsftp.NewClient(lease.Client())
	}
	if err != nil {
		lease.Discard()
		return "", fmt.Errorf("sftp subsystem: %w", err)
	}
	return s.register(&client{channel: channel, fs: sftp.NewClientFS(raw), lease: lease, raw: raw, bounded: sftp.NewSession(4)}), nil
}

// OpenForTerminal opens only a subsystem on the exact authenticated transport
// wired by SetTerminalTransport. It owns the SFTP channel, never the terminal's
// SSH connection or credentials.
func (s *Service) OpenForTerminal(sessionID string) (string, error) {
	if s.terminalTransport == nil {
		return "", fmt.Errorf("terminal service unavailable")
	}
	transport, sameSession, err := s.terminalTransport(sessionID)
	if err != nil {
		return "", err
	}
	raw, err := pkgsftp.NewClient(transport)
	if err != nil {
		return "", fmt.Errorf("terminal sftp subsystem: %w", err)
	}
	if !sameSession() {
		_ = raw.Close()
		return "", fmt.Errorf("terminal session closed while opening SFTP")
	}
	return s.register(&client{raw: raw, fs: sftp.NewClientFS(raw), bounded: sftp.NewSession(4)}), nil
}

// register installs a constructed client under the next opaque session ID.
func (s *Service) register(session *client) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter++
	id := fmt.Sprintf("sftp-%d", s.counter)
	s.sessions[id] = session
	return id
}

// HomeDir returns the remote working directory for the SFTP session.
func (s *Service) HomeDir(sessionID string) (string, error) {
	s.mu.Lock()
	session, ok := s.sessions[sessionID]
	s.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("sftp session %q not found", sessionID)
	}
	return session.raw.Getwd()
}

// Close releases the SFTP client and returns the transport to the pool.
func (s *Service) Close(sessionID string) error {
	s.mu.Lock()
	session, ok := s.sessions[sessionID]
	delete(s.sessions, sessionID)
	s.mu.Unlock()
	if !ok {
		return nil
	}
	_ = session.raw.Close()
	if session.channel != nil {
		_ = session.channel.Close()
	}
	if session.lease != nil {
		session.lease.Return()
	}
	return nil
}

func (s *Service) acquire(sessionID string) (*client, func(), error) {
	s.mu.Lock()
	session, ok := s.sessions[sessionID]
	s.mu.Unlock()
	if !ok {
		return nil, nil, fmt.Errorf("sftp session %q not found", sessionID)
	}
	release, err := session.bounded.Acquire()
	if err != nil {
		return nil, nil, err
	}
	return session, release, nil
}

// Acquire checks out a session's raw SFTP client for callers that need direct
// file handles (the chunked transfer scheduler opens files with explicit
// flags). Ordinary operations must use the use-case methods instead; the raw
// client is closed only via Close.
func (s *Service) Acquire(sessionID string) (*pkgsftp.Client, func(), error) {
	session, release, err := s.acquire(sessionID)
	if err != nil {
		return nil, nil, err
	}
	return session.raw, release, nil
}
