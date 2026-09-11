// Command proxy dialing (SSH-01): run a user-supplied ProxyCommand and ride
// its stdin/stdout as the SSH transport, matching OpenSSH ProxyCommand
// semantics. The command always runs through the system shell because
// ProxyCommand lines legitimately use pipes and redirection; it is the
// operator's own configuration, so this is the same trust level as Electron.
package ssh

import (
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// expandProxyTokens substitutes the OpenSSH ProxyCommand tokens: %h (host),
// %p (port) and %% (literal percent).
func expandProxyTokens(command, host, port string) string {
	replacer := strings.NewReplacer("%h", host, "%p", port, "%%", "%")
	return replacer.Replace(command)
}

func shellForCommand() (name, flag string) {
	if runtime.GOOS == "windows" {
		return "cmd", "/c"
	}
	return "sh", "-c"
}

// commandProxyAddr is a placeholder endpoint for the pipe transport.
type commandProxyAddr struct{ network, value string }

func (a commandProxyAddr) Network() string { return a.network }
func (a commandProxyAddr) String() string  { return a.value }

// CommandProxyConn adapts one proxy subprocess to net.Conn.
type CommandProxyConn struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  *strings.Builder
	done    chan struct{}
	waitErr error

	closeOnce sync.Once
}

// splitCommand breaks a command line into executable + arguments, respecting
// double quotes. This avoids cmd /c path resolution issues on Windows while
// still allowing simple multi-word commands.
func splitCommand(command string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false
	for i := 0; i < len(command); i++ {
		ch := command[i]
		switch {
		case ch == '"' && inQuote:
			inQuote = false
		case ch == '"':
			inQuote = true
		case ch == ' ' && !inQuote:
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// DialCommandProxy launches the proxy command bound to address and returns
// the pipe transport once the child has survived the immediate-exit window.
func DialCommandProxy(ctx context.Context, command, address string) (net.Conn, error) {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return nil, fmt.Errorf("proxy command is empty")
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		host, port = address, ""
	}
	expanded := expandProxyTokens(trimmed, host, port)
	parts := splitCommand(expanded)
	if len(parts) == 0 {
		return nil, fmt.Errorf("proxy command is empty")
	}
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("proxy command stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("proxy command stdout: %w", err)
	}
	stderr := &strings.Builder{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("proxy command start: %w", err)
	}
	conn := &CommandProxyConn{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		done:   make(chan struct{}),
	}
	go func() {
		conn.waitErr = cmd.Wait()
		close(conn.done)
	}()
	// Fail fast when the child dies immediately (unknown binary, bad args) so
	// the SSH handshake reports the proxy's own stderr instead of a timeout.
	select {
	case <-conn.done:
		_ = stdin.Close()
		return nil, fmt.Errorf("proxy command exited: %s: %w", strings.TrimSpace(conn.stderr.String()), conn.waitErr)
	case <-ctx.Done():
		_ = stdin.Close()
		return nil, ctx.Err()
	case <-time.After(300 * time.Millisecond):
	}
	return conn, nil
}

// ErrString exposes the child's stderr tail for connection diagnostics.
func (c *CommandProxyConn) ErrString() string {
	c.closeOnce.Do(func() {})
	return strings.TrimSpace(c.stderr.String())
}

func (c *CommandProxyConn) Read(p []byte) (int, error)  { return c.stdout.Read(p) }
func (c *CommandProxyConn) Write(p []byte) (int, error) { return c.stdin.Write(p) }

func (c *CommandProxyConn) Close() error {
	var firstErr error
	c.closeOnce.Do(func() {
		_ = c.stdin.Close()
		select {
		case <-c.done:
		case <-time.After(2 * time.Second):
			if c.cmd.Process != nil {
				_ = c.cmd.Process.Kill()
			}
		}
		if c.waitErr != nil && c.waitErr.Error() != "exit status 0" {
			firstErr = c.waitErr
		}
	})
	return firstErr
}

func (c *CommandProxyConn) LocalAddr() net.Addr {
	return commandProxyAddr{network: "proxy", value: "command"}
}

func (c *CommandProxyConn) RemoteAddr() net.Addr {
	return commandProxyAddr{network: "proxy", value: "command"}
}

// Deadline setters are accepted and ignored: the backing pipes do not support
// them and cancellation is enforced by the context bound to the process.
func (c *CommandProxyConn) SetDeadline(time.Time) error      { return nil }
func (c *CommandProxyConn) SetReadDeadline(time.Time) error  { return nil }
func (c *CommandProxyConn) SetWriteDeadline(time.Time) error { return nil }
