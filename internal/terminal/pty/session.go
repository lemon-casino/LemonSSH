// Package pty owns local terminal session lifecycle (P3-02). The session
// owner is platform-neutral; Unix and Windows backends implement the PTY
// primitive behind the Backend interface.
package pty

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

var (
	ErrSessionNotFound = errors.New("pty session not found")
	ErrGenerationStale = errors.New("pty session generation is stale")
	ErrAlreadyStarted  = errors.New("pty session already started")
	ErrUnsupported     = errors.New("pty backend unsupported on this platform")
)

type Config struct {
	SessionID string
	Shell     string
	Args      []string
	CWD       string
	Env       []string
	Cols      uint16
	Rows      uint16
}

type Event struct {
	SessionID  string
	Generation uint32
	Kind       string
	Data       []byte
	Err        error
}

type Backend interface {
	Start(context.Context, Config) (Process, error)
}

type Process interface {
	io.ReadWriteCloser
	Resize(cols, rows uint16) error
	Interrupt() error
	Kill() error
	Wait() error
	PID() int
}

type Session struct {
	mu         sync.Mutex
	config     Config
	generation uint32
	process    Process
	started    bool
	closed     bool
}

func NewSession(config Config) *Session {
	if config.Cols == 0 {
		config.Cols = 80
	}
	if config.Rows == 0 {
		config.Rows = 24
	}
	return &Session{config: config, generation: 1}
}

func (s *Session) ID() string         { return s.config.SessionID }
func (s *Session) Generation() uint32 { s.mu.Lock(); defer s.mu.Unlock(); return s.generation }

func (s *Session) Start(ctx context.Context, backend Backend) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrSessionNotFound
	}
	if s.started {
		return ErrAlreadyStarted
	}
	if backend == nil {
		return ErrUnsupported
	}
	process, err := backend.Start(ctx, s.config)
	if err != nil {
		return err
	}
	s.process = process
	s.started = true
	return nil
}

func (s *Session) Resize(generation uint32, cols, rows uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.process == nil {
		return ErrSessionNotFound
	}
	if generation != s.generation {
		return ErrGenerationStale
	}
	if cols == 0 || rows == 0 {
		return fmt.Errorf("invalid PTY size %d x %d", cols, rows)
	}
	s.config.Cols, s.config.Rows = cols, rows
	return s.process.Resize(cols, rows)
}

func (s *Session) Write(generation uint32, input []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.process == nil {
		return 0, ErrSessionNotFound
	}
	if generation != s.generation {
		return 0, ErrGenerationStale
	}
	return s.process.Write(input)
}

func (s *Session) Interrupt(generation uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.process == nil {
		return ErrSessionNotFound
	}
	if generation != s.generation {
		return ErrGenerationStale
	}
	return s.process.Interrupt()
}

func (s *Session) processForTest() Process {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.process
}

// readOnce reads from the started process without holding the session lock
// (blocking reads must not block Close/Resize).
func (s *Session) readOnce(buf []byte) (int, error) {
	s.mu.Lock()
	process := s.process
	started := s.started
	closed := s.closed
	s.mu.Unlock()
	if !started || process == nil || closed {
		return 0, ErrSessionNotFound
	}
	return process.Read(buf)
}

func (s *Session) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	process := s.process
	s.mu.Unlock()
	if process == nil {
		return nil
	}
	if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return process.Wait()
}

// Reconnect invalidates the previous generation and optionally terminates the
// old process before a caller starts a replacement.
func (s *Session) Reconnect() (uint32, error) {
	s.mu.Lock()
	old := s.process
	s.generation++
	s.process = nil
	s.started = false
	generation := s.generation
	s.mu.Unlock()
	if old != nil {
		_ = old.Kill()
		_ = old.Wait()
	}
	return generation, nil
}

// DefaultShell resolves the same product boundary as Electron: an explicit
// shell wins, otherwise the user's platform default shell.
func DefaultShell(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if runtime.GOOS == "windows" {
		if value := os.Getenv("COMSPEC"); value != "" {
			return value
		}
		return `C:\Windows\System32\cmd.exe`
	}
	if value := os.Getenv("SHELL"); value != "" {
		return value
	}
	return "/bin/sh"
}

func ValidCWD(value string) string {
	if value == "" {
		home, _ := os.UserHomeDir()
		return home
	}
	resolved, err := filepath.Abs(value)
	if err != nil {
		home, _ := os.UserHomeDir()
		return home
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		home, _ := os.UserHomeDir()
		return home
	}
	return resolved
}

func BuildConfig(sessionID, shell, cwd string, args, env []string, cols, rows uint16) Config {
	return Config{SessionID: sessionID, Shell: DefaultShell(shell), Args: append([]string(nil), args...), CWD: ValidCWD(cwd), Env: append([]string(nil), env...), Cols: cols, Rows: rows}
}

// ExecCommandBackend is the common process supervision layer. Platform PTY
// backends wrap the command's stdin/stdout around a PTY or ConPTY.
type ExecCommandBackend struct{}

func (*ExecCommandBackend) command(ctx context.Context, config Config) *exec.Cmd {
	return exec.CommandContext(ctx, config.Shell, config.Args...)
}
