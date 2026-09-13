package ssh

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"
)

// ProbeProxy checks that a proxy can be reached. HTTP/SOCKS5 dial the proxy
// listener; ProxyCommand is started with OpenSSH %h/%p tokens. A target host
// is only used for command expansion or an optional CONNECT through the proxy.
func ProbeProxy(ctx context.Context, kind, host string, port int, username, password, command, targetHost string, targetPort int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	switch kind {
	case "command":
		if command == "" {
			return fmt.Errorf("proxy command is empty")
		}
		probeHost := targetHost
		if probeHost == "" {
			probeHost = "127.0.0.1"
		}
		probePort := targetPort
		if probePort <= 0 {
			probePort = 1
		}
		conn, err := DialCommandProxy(ctx, command, net.JoinHostPort(probeHost, strconv.Itoa(probePort)))
		if err != nil {
			return err
		}
		_ = conn.Close()
		return nil
	case "http", "socks5":
		proxyURL, err := FormatProxyURL(kind, host, port, username, password)
		if err != nil {
			return err
		}
		endpoint := net.JoinHostPort(host, strconv.Itoa(port))
		dialer := &net.Dialer{Timeout: 8 * time.Second}
		raw, err := dialer.DialContext(ctx, "tcp", endpoint)
		if err != nil {
			return fmt.Errorf("proxy unreachable: %w", err)
		}
		_ = raw.Close()
		if targetHost == "" || targetPort <= 0 {
			return nil
		}
		via, err := ProxyDial(ctx, proxyURL)
		if err != nil {
			return err
		}
		if via == nil {
			return fmt.Errorf("proxy dialer unavailable")
		}
		conn, err := via(ctx, "tcp", net.JoinHostPort(targetHost, strconv.Itoa(targetPort)))
		if err != nil {
			return fmt.Errorf("proxy connect failed: %w", err)
		}
		_ = conn.Close()
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedProxy, kind)
	}
}
