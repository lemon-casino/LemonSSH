//go:build !windows

package pty

import (
	"context"
	"os"
	"os/exec"

	"github.com/creack/pty/v2"
)

type unixBackend struct{}

func NewPlatformBackend() Backend { return &unixBackend{} }

type unixProcess struct {
	cmd  *exec.Cmd
	file *os.File
}

func (*unixBackend) Start(ctx context.Context, config Config) (Process, error) {
	cmd := exec.CommandContext(ctx, config.Shell, config.Args...)
	cmd.Dir = config.CWD
	cmd.Env = config.Env
	file, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: config.Cols, Rows: config.Rows})
	if err != nil {
		return nil, err
	}
	return &unixProcess{cmd: cmd, file: file}, nil
}

func (p *unixProcess) Read(data []byte) (int, error)  { return p.file.Read(data) }
func (p *unixProcess) Write(data []byte) (int, error) { return p.file.Write(data) }
func (p *unixProcess) Close() error                   { return p.file.Close() }
func (p *unixProcess) Resize(cols, rows uint16) error {
	return pty.Setsize(p.file, &pty.Winsize{Cols: cols, Rows: rows})
}
func (p *unixProcess) Interrupt() error { return p.Write([]byte{3}) }
func (p *unixProcess) Kill() error {
	if p.cmd.Process == nil {
		return os.ErrProcessDone
	}
	return p.cmd.Process.Kill()
}
func (p *unixProcess) Wait() error { return p.cmd.Wait() }
func (p *unixProcess) PID() int {
	if p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}
