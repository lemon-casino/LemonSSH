// Command proxy dialing (SSH-01): run a user-supplied ProxyCommand and ride
// its stdin/stdout as the SSH transport, matching OpenSSH ProxyCommand
// semantics. The command always runs through the system shell because
// ProxyCommand lines legitimately use pipes and redirection; it is the
// operator's own configuration, so this is the same trust level as Electron.
package ssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	// commandProxyStartWindow is how long DialCommandProxy observes the child
	// for an immediate exit (unknown binary, bad arguments) before handing the
	// transport to the SSH handshake.
	commandProxyStartWindow = 300 * time.Millisecond
	// commandProxyKillDelay bounds how long Close waits for a graceful child
	// exit before killing the process.
	commandProxyKillDelay = 2 * time.Second
	// commandProxyWaitDelay bounds how long cmd.Wait may keep waiting on the
	// child's I/O pipes after the child process has exited. Orphaned
	// descendants can hold those pipes open indefinitely; without this bound
	// Wait never returns, done never closes, and CommandProxyConn.Close then
	// wedges every SSH teardown path (handshake deadline, transport close)
	// because they all funnel through its sync.Once.
	commandProxyWaitDelay = 2 * time.Second
)

// syncBuffer is a concurrency-safe stderr sink: with WaitDelay set, Wait can
// return while os/exec's stderr copier is still draining the pipe, so reads
// must not race the copier's writes.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

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
	stderr  *syncBuffer
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
	if strings.ContainsAny(expanded, "|&<>") || runtime.GOOS != "windows" {
		shell, flag := shellForCommand()
		cmd = exec.CommandContext(ctx, shell, flag, expanded)
	}
	// Bound cmd.Wait itself: once the child has exited, os/exec closes the I/O
	// pipes after WaitDelay instead of waiting for an EOF that orphaned
	// descendants may never deliver. Without this, Wait (and therefore done)
	// can remain blocked forever even though the child is long dead.
	cmd.WaitDelay = commandProxyWaitDelay
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("proxy command stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("proxy command stdout: %w", err)
	}
	stderr := &syncBuffer{}
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
		_ = conn.Close()
		return nil, ctx.Err()
	case <-time.After(commandProxyStartWindow):
	}
	return conn, nil
}

// ErrString exposes the child's stderr tail for connection diagnostics.
func (c *CommandProxyConn) ErrString() string {
	select {
	case <-c.done:
		return strings.TrimSpace(c.stderr.String())
	default:
		return ""
	}
}

func (c *CommandProxyConn) Read(p []byte) (int, error) {
	n, err := c.stdout.Read(p)
	// Surface a dead child as the read error so the SSH handshake failure
	// carries the proxy's own diagnosis instead of a bare EOF. ErrString never
	// blocks: while the child is (or may still be) running it returns empty.
	if n == 0 && errors.Is(err, io.EOF) {
		if msg := c.ErrString(); msg != "" {
			return n, fmt.Errorf("proxy command exited: %s", msg)
		}
	}
	return n, err
}

func (c *CommandProxyConn) Write(p []byte) (int, error) { return c.stdin.Write(p) }

func (c *CommandProxyConn) Close() error {
	var firstErr error
	c.closeOnce.Do(func() {
		_ = c.stdin.Close()
		_ = c.stdout.Close()
		select {
		case <-c.done:
		case <-time.After(commandProxyKillDelay):
			if c.cmd.Process != nil {
				_ = c.cmd.Process.Kill()
			}
			select {
			case <-c.done:
			case <-time.After(commandProxyWaitDelay):
				// The child's exit could not be observed even after Kill
				// (for example an orphaned descendant keeps the I/O pipes
				// open and blocks Wait). Give up instead of blocking forever:
				// the SSH handshake deadline, transport close, and
				// x/crypto/ssh's own goroutines all close this connection and
				// serialize on closeOnce, so an unbounded wait here wedges
				// the whole dial path.
				firstErr = errors.New("proxy command did not exit after kill")
				return
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
