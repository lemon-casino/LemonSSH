//go:build windows

package pty

import (
	"context"
	"fmt"

	conpty "github.com/UserExistsError/conpty"
)

type conptyBackend struct{}

// NewPlatformBackend provides the real ConPTY adapter via the battle-tested
// UserExistsError/conpty wrapper (CreatePseudoConsole + attached process).
// Resizing, interrupt and kill are wired to ConPTY semantics so the terminal
// behaviour matches the node-pty baseline.
func NewPlatformBackend() Backend { return &conptyBackend{} }

type conptyProcess struct {
	pty  *conpty.ConPty
	quit chan struct{}
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
		return nil, fmt.Errorf("ConPTY start %s: %w", commandLine, err)
	}
	return &conptyProcess{pty: ptyInstance, quit: make(chan struct{})}, nil
}

func (p *conptyProcess) Read(data []byte) (int, error)  { return p.pty.Read(data) }
func (p *conptyProcess) Write(data []byte) (int, error) { return p.pty.Write(data) }
func (p *conptyProcess) Close() error                   { return p.pty.Close() }
func (p *conptyProcess) Resize(cols, rows uint16) error { return p.pty.Resize(int(cols), int(rows)) }
func (p *conptyProcess) Interrupt() error {
	_, err := p.Write([]byte{3})
	return err
}

// Kill closes the ConPTY, which terminates the attached child tree.
func (p *conptyProcess) Kill() error {
	select {
	case <-p.quit:
		return nil
	default:
		close(p.quit)
	}
	return p.pty.Close()
}

func (p *conptyProcess) Wait() error {
	select {
	case <-p.quit:
		// Kill/Close already tore the ConPTY down; the handle is gone.
		return nil
	default:
	}
	_, err := p.pty.Wait(context.Background())
	return err
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
	if value != "" && !containsSpace(value) {
		return value
	}
	return `"` + value + `"`
}

func containsSpace(value string) bool {
	for _, r := range value {
		if r == ' ' || r == '\t' {
			return true
		}
	}
	return false
}
