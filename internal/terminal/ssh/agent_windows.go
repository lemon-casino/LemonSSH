//go:build windows

package ssh

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
)

func dialAgent(address string) (net.Conn, error) {
	if strings.HasPrefix(address, `\\.\pipe\`) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return winio.DialPipeContext(ctx, address)
	}
	return net.DialTimeout("unix", address, 3*time.Second)
}
