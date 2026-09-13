package main

import (
	"context"
	"time"

	"github.com/binaricat/netcatty/internal/terminal/ssh"
)

type ProxyProbeRequest struct {
	Kind       string `json:"kind"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Command    string `json:"command"`
	TargetHost string `json:"targetHost"`
	TargetPort int    `json:"targetPort"`
}

type ProxyProbeResult struct {
	Ok        bool   `json:"ok"`
	LatencyMs int64  `json:"latencyMs"`
	Error     string `json:"error,omitempty"`
}

// TestProxy probes HTTP/SOCKS5/ProxyCommand reachability without opening a session.
func (s *TerminalService) TestProxy(request ProxyProbeRequest) ProxyProbeResult {
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	err := ssh.ProbeProxy(
		ctx,
		request.Kind,
		request.Host,
		request.Port,
		request.Username,
		request.Password,
		request.Command,
		request.TargetHost,
		request.TargetPort,
	)
	result := ProxyProbeResult{LatencyMs: time.Since(started).Milliseconds()}
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Ok = true
	return result
}
