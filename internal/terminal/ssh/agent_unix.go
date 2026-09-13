//go:build !windows

package ssh

import (
	"net"
	"time"
)

func dialAgent(address string) (net.Conn, error) {
	return net.DialTimeout("unix", address, 3*time.Second)
}
