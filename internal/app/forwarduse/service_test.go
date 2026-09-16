package forwarduse

import (
	"testing"

	"github.com/binaricat/netcatty/internal/app/terminaluse"
)

// Characterization moved verbatim from cmd/netcatty/forward_service_test.go
// during the W04 forward use-case extraction; the assertions are unchanged.
func TestParseForwardRuleID(t *testing.T) {
	if got := parseForwardRuleID("pf-rule-1-171000"); got != "rule-1" {
		t.Fatalf("got %q", got)
	}
}

func TestForwardStartFailsClosedWithoutSSH(t *testing.T) {
	service := New(nil, nil)
	result := service.Start("pf-rule-1-1", "local", "127.0.0.1", 0, "127.0.0.1", 22, terminaluse.SSHConnectRequest{Hostname: "example.invalid"})
	if result.Success || result.Error == "" {
		t.Fatalf("missing pool must fail closed: %+v", result)
	}
}
