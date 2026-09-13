package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/binaricat/netcatty/internal/platform/monitoring"
	gossh "golang.org/x/crypto/ssh"
)

// MonitoringResult keeps the existing renderer success/error envelope. Payload
// keys are collection-specific and omitted on failure.
type MonitoringResult struct {
	Success      bool           `json:"success"`
	Error        string         `json:"error,omitempty"`
	Stats        any            `json:"stats,omitempty"`
	Capabilities monitoring.Row `json:"capabilities,omitempty"`
	Processes    any            `json:"processes,omitempty"`
	Sessions     any            `json:"sessions,omitempty"`
	Containers   any            `json:"containers,omitempty"`
	Images       any            `json:"images,omitempty"`
}
type DockerStatsOptions struct {
	SessionID string   `json:"sessionId"`
	IDs       []string `json:"ids,omitempty"`
}
type monitoringSSHChannel struct{ *gossh.Session }

func (c monitoringSSHChannel) SetOutput(out, stderr io.Writer) { c.Stdout = out; c.Stderr = stderr }
func (s *TerminalService) monitoringExec(ctx context.Context, id, command string) (string, error) {
	term, ok := s.lookup(id)
	if !ok {
		return "", fmt.Errorf("terminal session is not connected")
	}
	if term.local != nil {
		return monitoring.ExecuteLocal(ctx, command)
	}
	if term.transport == nil || term.transport.Client == nil {
		return "", fmt.Errorf("monitoring is unsupported for this terminal transport; an SSH session is required")
	}
	return monitoring.Execute(ctx, func() (monitoring.Channel, error) {
		ch, e := term.transport.Client.NewSession()
		if e != nil {
			return nil, e
		}
		return monitoringSSHChannel{ch}, nil
	}, command)
}
func monitoringFailure(err error) MonitoringResult { return MonitoringResult{Error: err.Error()} }
func (s *TerminalService) GetServerStats(ctx context.Context, sessionID string) MonitoringResult {
	text, e := s.monitoringExec(ctx, sessionID, monitoring.StatsCommand)
	if e != nil {
		return monitoringFailure(e)
	}
	stats, e := monitoring.ParseStats(text)
	if e != nil {
		return monitoringFailure(e)
	}
	return MonitoringResult{Success: true, Stats: stats}
}
func (s *TerminalService) ProbeSystemCapabilities(ctx context.Context, sessionID string) MonitoringResult {
	text, e := s.monitoringExec(ctx, sessionID, monitoring.ProbeCommand)
	if e != nil {
		return monitoringFailure(e)
	}
	caps, e := monitoring.ParseCapabilities(text)
	if e != nil {
		return monitoringFailure(e)
	}
	return MonitoringResult{Success: true, Capabilities: caps}
}
func (s *TerminalService) ListSystemProcesses(ctx context.Context, sessionID string) MonitoringResult {
	text, e := s.monitoringExec(ctx, sessionID, monitoring.ProcessesCommand)
	if e != nil {
		return monitoringFailure(e)
	}
	rows, e := monitoring.ParseProcesses(text)
	if e != nil {
		return monitoringFailure(e)
	}
	return MonitoringResult{Success: true, Processes: rows}
}
func (s *TerminalService) ListTmuxSessions(ctx context.Context, sessionID string) MonitoringResult {
	text, e := s.monitoringExec(ctx, sessionID, monitoring.TmuxCommand)
	if e != nil {
		return monitoringFailure(e)
	}
	rows, e := monitoring.ParseTmux(text)
	if e != nil {
		return monitoringFailure(e)
	}
	return MonitoringResult{Success: true, Sessions: rows}
}
func (s *TerminalService) ListDockerContainers(ctx context.Context, sessionID string) MonitoringResult {
	text, e := s.monitoringExec(ctx, sessionID, monitoring.ContainersCommand)
	if e != nil {
		return monitoringFailure(e)
	}
	rows, e := monitoring.ParseDocker("containers", text)
	if e != nil {
		return monitoringFailure(e)
	}
	return MonitoringResult{Success: true, Containers: rows}
}
func (s *TerminalService) ListDockerImages(ctx context.Context, sessionID string) MonitoringResult {
	text, e := s.monitoringExec(ctx, sessionID, monitoring.ImagesCommand)
	if e != nil {
		return monitoringFailure(e)
	}
	rows, e := monitoring.ParseDocker("images", text)
	if e != nil {
		return monitoringFailure(e)
	}
	return MonitoringResult{Success: true, Images: rows}
}
func (s *TerminalService) GetDockerStats(ctx context.Context, options DockerStatsOptions) MonitoringResult {
	text, e := s.monitoringExec(ctx, options.SessionID, monitoring.DockerStatsCommand)
	if e != nil {
		return monitoringFailure(e)
	}
	rows, e := monitoring.ParseDocker("stats", text)
	if e != nil {
		return monitoringFailure(e)
	}
	// Filtering is local: IDs never enter a shell command, including hostile input.
	if len(options.IDs) > 0 {
		filtered := []monitoring.Row{}
		for _, row := range rows {
			for _, id := range options.IDs {
				if id != "" && (row["name"] == id || strings.HasPrefix(row["id"].(string), id)) {
					filtered = append(filtered, row)
					break
				}
			}
		}
		rows = filtered
	}
	return MonitoringResult{Success: true, Stats: rows}
}
