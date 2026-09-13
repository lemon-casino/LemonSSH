//go:build windows

package ssh

import (
	"crypto/rand"
	"github.com/Microsoft/go-winio"
	"net"
	"testing"
)

func newAuthAgentListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := winio.ListenPipe(`\\.\pipe\netcatty-agent-test-`+rand.Text(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return listener
}
