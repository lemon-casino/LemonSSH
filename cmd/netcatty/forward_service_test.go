package main

import "testing"

func TestParseForwardRuleID(t *testing.T) {
	if got := parseForwardRuleID("pf-rule-1-171000"); got != "rule-1" {
		t.Fatalf("got %q", got)
	}
}

func TestForwardStartFailsClosedWithoutSSH(t *testing.T) {
	service := NewForwardService(nil, nil)
	result := service.Start("pf-rule-1-1", "local", "127.0.0.1", 0, "127.0.0.1", 22, SSHConnectRequest{Hostname: "example.invalid"})
	if result.Success || result.Error == "" {
		t.Fatalf("missing pool must fail closed: %+v", result)
	}
}
