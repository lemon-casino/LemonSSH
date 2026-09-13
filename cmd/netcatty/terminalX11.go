package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/binaricat/netcatty/internal/terminal/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// startTerminalX11 runs before Shell so sshd can install the remote DISPLAY and
// its spoofed authority. The terminal owns the returned closer for its lifetime.
func startTerminalX11(ctx context.Context, client *gossh.Client, session *gossh.Session, enabled bool, spec string) (*ssh.X11Forwarder, error) {
	if !enabled {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(spec) == "" {
		spec = os.Getenv("DISPLAY")
	}
	if strings.TrimSpace(spec) == "" {
		spec = ":0"
	}
	display, err := ssh.ParseX11Display(spec, runtime.GOOS)
	if err != nil {
		return nil, err
	}
	cookie, err := ssh.ReadX11Cookie(ctx, display)
	if err != nil {
		return nil, err
	}
	if client == nil || session == nil {
		return nil, fmt.Errorf("X11: SSH session unavailable")
	}
	return ssh.StartX11Forwarding(ctx, client, session, display, cookie)
}
