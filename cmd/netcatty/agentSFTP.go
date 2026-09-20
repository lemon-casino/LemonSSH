package main

import (
	"context"
	"fmt"

	"github.com/binaricat/netcatty/internal/capability"
	"github.com/binaricat/netcatty/internal/terminal/sftp"
)

type SFTPWriter interface {
	WriteText(sessionID, path, content string) error
	Mkdir(sessionID, path string) error
	Remove(sessionID, path string) error
	Rename(sessionID, oldPath, newPath string) error
	Chmod(sessionID, path, mode string) error
}

func (h *AgentHost) sftpMutationHandler(writer SFTPWriter) capability.Handler {
	return func(ctx context.Context, params map[string]any, def *capability.Definition) (any, error) {
		activeWriter := writer
		if scoped, ok := h.sftpForContext(ctx).(SFTPWriter); ok {
			activeWriter = scoped
		}
		id, path := hostSessionAndPath(params)
		if err := h.checkSession(id); err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id = h.state.nativeID(id)
		var err error
		if path == "" && def.ID != "sftp.rename" {
			return nil, fmt.Errorf("path is required")
		}
		switch def.ID {
		case "sftp.write":
			content, ok := params["content"].(string)
			if !ok {
				return nil, fmt.Errorf("content must be a string")
			}
			err = activeWriter.WriteText(id, path, content)
		case "sftp.mkdir":
			err = activeWriter.Mkdir(id, path)
		case "sftp.delete":
			err = activeWriter.Remove(id, path)
		case "sftp.rename":
			oldPath, _ := params["oldPath"].(string)
			newPath, _ := params["newPath"].(string)
			if oldPath == "" || newPath == "" {
				return nil, fmt.Errorf("oldPath and newPath are required")
			}
			err = activeWriter.Rename(id, oldPath, newPath)
		case "sftp.chmod":
			mode, _ := params["mode"].(string)
			err = activeWriter.Chmod(id, path, mode)
		}
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	}
}

// Tools receive terminal IDs. Lease a channel on the existing SSH transport
// and release it after each operation, including failures.
type agentSFTP struct {
	service *SFTPService
	ctx     context.Context
}

func (s agentSFTP) WithContext(ctx context.Context) SFTPReader { s.ctx = ctx; return s }

func (h *AgentHost) sftpForContext(ctx context.Context) SFTPReader {
	if scoped, ok := h.sftp.(interface {
		WithContext(context.Context) SFTPReader
	}); ok {
		return scoped.WithContext(ctx)
	}
	return h.sftp
}

func withAgentSFTP[T any](s agentSFTP, session string, run func(string) (T, error)) (T, error) {
	id, err := s.service.OpenForTerminal(session)
	if err != nil {
		var zero T
		return zero, err
	}
	defer s.service.Close(id)
	if s.ctx != nil {
		if err := s.ctx.Err(); err != nil {
			var zero T
			return zero, err
		}
		stop := context.AfterFunc(s.ctx, func() { _ = s.service.Close(id) })
		defer stop()
	}
	return run(id)
}

func (s agentSFTP) List(id, path string) ([]sftp.Entry, error) {
	return withAgentSFTP(s, id, func(channel string) ([]sftp.Entry, error) { return s.service.List(channel, path) })
}
func (s agentSFTP) Stat(id, path string) (sftp.FileInfo, error) {
	return withAgentSFTP(s, id, func(channel string) (sftp.FileInfo, error) { return s.service.Stat(channel, path) })
}
func (s agentSFTP) Read(id, path string) (string, error) {
	return withAgentSFTP(s, id, func(channel string) (string, error) { return s.service.Read(channel, path) })
}
func (s agentSFTP) HomeDir(id string) (string, error) { return withAgentSFTP(s, id, s.service.HomeDir) }
func (s agentSFTP) Download(id, remote, local string) (int64, error) {
	return withAgentSFTP(s, id, func(channel string) (int64, error) { return s.service.Download(channel, remote, local) })
}
func (s agentSFTP) Upload(id, local, remote string) (int64, error) {
	return withAgentSFTP(s, id, func(channel string) (int64, error) { return s.service.Upload(channel, local, remote) })
}
func (s agentSFTP) mutate(id string, run func(string) error) error {
	_, err := withAgentSFTP(s, id, func(channel string) (bool, error) { return true, run(channel) })
	return err
}
func (s agentSFTP) WriteText(id, path, content string) error {
	return s.mutate(id, func(channel string) error { return s.service.WriteText(channel, path, content) })
}
func (s agentSFTP) Mkdir(id, path string) error {
	return s.mutate(id, func(channel string) error { return s.service.Mkdir(channel, path) })
}
func (s agentSFTP) Remove(id, path string) error {
	return s.mutate(id, func(channel string) error { return s.service.Remove(channel, path) })
}
func (s agentSFTP) Rename(id, oldPath, newPath string) error {
	return s.mutate(id, func(channel string) error { return s.service.Rename(channel, oldPath, newPath) })
}
func (s agentSFTP) Chmod(id, path, mode string) error {
	return s.mutate(id, func(channel string) error { return s.service.Chmod(channel, path, mode) })
}
