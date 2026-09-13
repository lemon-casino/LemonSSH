package main

import "testing"

func TestProxyProbeReportsUnreachableSOCKS(t *testing.T) {
	service := &TerminalService{}
	result := service.TestProxy(ProxyProbeRequest{Kind: "socks5", Host: "127.0.0.1", Port: 1})
	if result.Ok || result.Error == "" {
		t.Fatalf("dead socks proxy must fail: %+v", result)
	}
	if result.LatencyMs < 0 {
		t.Fatalf("latency: %d", result.LatencyMs)
	}
}

func TestProxyProbeRejectsEmptyCommand(t *testing.T) {
	service := &TerminalService{}
	result := service.TestProxy(ProxyProbeRequest{Kind: "command"})
	if result.Ok || result.Error == "" {
		t.Fatalf("empty command must fail: %+v", result)
	}
}
