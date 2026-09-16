package main

import (
	"testing"

	"github.com/binaricat/netcatty/internal/app/terminaluse"
	"github.com/binaricat/netcatty/internal/terminal/dataplane"
	gossh "golang.org/x/crypto/ssh"
)

// newSeamTerminalService builds the terminal facade with a live core seeded for
// cross-domain seam fixtures (SFTP/clipboard). transportSessions maps session
// IDs to established SSH clients; plainSessions installs empty live sessions.
// Production wiring goes through NewTerminalService instead.
func newSeamTerminalService(t *testing.T, plainSessions []string, transportSessions map[string]*gossh.Client) *TerminalService {
	t.Helper()
	controller := dataplane.NewRouteController()
	service := &TerminalService{core: terminaluse.New(controller, dataplane.NewServer(controller, ""), nil)}
	for _, id := range plainSessions {
		service.core.SeedSessionForTest(id)
	}
	for id, client := range transportSessions {
		service.core.SeedTransportSessionForTest(id, client)
	}
	return service
}
