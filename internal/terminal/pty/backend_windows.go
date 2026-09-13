//go:build windows

package pty

import (
	"context"
	"fmt"
	"sync"
	"syscall"

	conpty "github.com/UserExistsError/conpty"
	"golang.org/x/sys/windows"
)

type conptyBackend struct{}

// NewPlatformBackend provides the real ConPTY adapter via the battle-tested
// UserExistsError/conpty wrapper (CreatePseudoConsole + attached process).
// Resizing, interrupt and kill are wired to ConPTY semantics so the terminal
// behaviour matches the node-pty baseline.
func NewPlatformBackend() Backend { return &conptyBackend{} }

type conptyProcess struct {
	pty        *conpty.ConPty
	quit       chan struct{}
	mu         sync.Mutex
	closeOnce  sync.Once
	waitOnce   sync.Once
	waitHandle windows.Handle
	waitErr    error
}

func (*conptyBackend) Start(ctx context.Context, config Config) (Process, error) {
	if config.Cols == 0 || config.Rows == 0 {
		return nil, fmt.Errorf("invalid ConPTY size %d x %d", config.Cols, config.Rows)
	}
	commandLine := formatCommand(config.Shell, config.Args)
	options := []conpty.ConPtyOption{
		conpty.ConPtyDimensions(int(config.Cols), int(config.Rows)),
		conpty.ConPtyWorkDir(config.CWD),
		conpty.ConPtyEnv(config.Env),
	}
	ptyInstance, err := conpty.Start(commandLine, options...)
	if err != nil {
		return nil, fmt.Errorf("ConPTY start: %w", err)
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(ptyInstance.Pid()))
	if err != nil {
		_ = ptyInstance.Close()
		return nil, err
	}
	return &conptyProcess{pty: ptyInstance, quit: make(chan struct{}), waitHandle: handle}, nil
}

func (p *conptyProcess) Read(data []byte) (int, error)  { return p.pty.Read(data) }
func (p *conptyProcess) Write(data []byte) (int, error) { return p.pty.Write(data) }
func (p *conptyProcess) Close() error                   { return p.Kill() }
func (p *conptyProcess) Resize(cols, rows uint16) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	select {
	case <-p.quit:
		return ErrSessionNotFound
	default:
	}
	return p.pty.Resize(int(cols), int(rows))
}
func (p *conptyProcess) Interrupt() error {
	_, err := p.Write([]byte{3})
	return err
}

// Kill closes the ConPTY, which terminates the attached child tree.
func (p *conptyProcess) Kill() error {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		close(p.quit)
		_ = p.pty.Close()
	})
	return nil
}

func (p *conptyProcess) Wait() error {
	p.waitOnce.Do(func() {
		defer windows.CloseHandle(p.waitHandle)
		_, p.waitErr = windows.WaitForSingleObject(p.waitHandle, windows.INFINITE)
		if p.waitErr != nil {
			return
		}
		var code uint32
		p.waitErr = windows.GetExitCodeProcess(p.waitHandle, &code)
		if p.waitErr == nil && code != 0 {
			p.waitErr = ExitError{Code: int(code)}
		}
	})
	return p.waitErr
}

func (p *conptyProcess) PID() int { return p.pty.Pid() }

func formatCommand(shell string, args []string) string {
	quoted := []string{quoteIfNeeded(shell)}
	for _, arg := range args {
		quoted = append(quoted, quoteIfNeeded(arg))
	}
	result := ""
	for index, part := range quoted {
		if index > 0 {
			result += " "
		}
		result += part
	}
	return result
}

func quoteIfNeeded(value string) string {
	return syscall.EscapeArg(value)
}

func containsSpace(value string) bool {
	for _, r := range value {
		if r == ' ' || r == '\t' {
			return true
		}
	}
	return false
}
