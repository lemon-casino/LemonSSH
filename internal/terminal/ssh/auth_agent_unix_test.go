//go:build !windows

package ssh

import (
	"net"
	"path/filepath"
	"testing"
)

func newAuthAgentListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("unix", filepath.Join(t.TempDir(), "agent.sock"))
	if err != nil {
		t.Fatal(err)
	}
	return listener
}
