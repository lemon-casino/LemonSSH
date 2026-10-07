package terminaluse

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/lemon-casino/lemonssh/internal/terminal/ssh"
)

// ExecRequest is a one-shot SSH exec (ssh-copy-id style key export, remote
// checks): the shared dial fields plus the command to run. Interactive dial
// callbacks (passphrase prompts, changed host-key confirmation, MFA) match the
// terminal Connect path.
type ExecRequest struct {
	SSHConnectRequest
	Command   string `json:"command"`
	TimeoutMs int    `json:"timeoutMs"`
}

// ExecResult reports the command's streams and exit status. Code stays nil
// when the command produced no exit status (transport failure, cancellation).
type ExecResult struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Code   *int   `json:"code"`
}

// DefaultExecTimeout bounds one-shot execs whose caller sent no timeout.
const DefaultExecTimeout = 30 * time.Second

// execFailure builds the null-code failure result.
func execFailure(format string, args ...any) ExecResult {
	return ExecResult{Stderr: fmt.Sprintf(format, args...)}
}

// Exec dials SSH with the shared auth path, runs Command once and returns its
// streams. The transport is always closed before returning; exec timeout
// bounds the whole operation.
func (s *Service) Exec(request ExecRequest) ExecResult {
	if request.Command == "" {
		return execFailure("command is required")
	}
	if request.Hostname == "" || request.Username == "" {
		return execFailure("host and username are required")
	}
	if request.Port == 0 {
		request.Port = 22
	}
	timeout := DefaultExecTimeout
	if request.TimeoutMs > 0 {
		timeout = time.Duration(request.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	config, err := terminalSSHDialConfig(request.SSHConnectRequest, s.knownHosts, s.DialInteractive(ctx, request.SSHConnectRequest))
	if err != nil {
		return execFailure("%s", err.Error())
	}
	transport, err := ssh.Dial(ctx, config)
	if err != nil {
		s.notifyPassphraseRejected(err)
		return execFailure("ssh dial %s:%d: %s", request.Hostname, request.Port, err.Error())
	}
	defer transport.Close()
	session, err := transport.Client.NewSession()
	if err != nil {
		return execFailure("new session: %s", err.Error())
	}
	defer session.Close()
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	// Honor the exec timeout even while the remote command runs: closing the
	// session unblocks session.Run.
	stopOnDeadline := context.AfterFunc(ctx, func() { _ = session.Close() })
	defer stopOnDeadline()
	runErr := session.Run(request.Command)
	if runErr == nil {
		code := 0
		return ExecResult{Stdout: stdout.String(), Stderr: stderr.String(), Code: &code}
	}
	if code, ok := exitCodeOf(runErr); ok {
		return ExecResult{Stdout: stdout.String(), Stderr: stderr.String(), Code: &code}
	}
	stderrText := stderr.String()
	if stderrText == "" {
		stderrText = runErr.Error()
	}
	return ExecResult{Stdout: stdout.String(), Stderr: stderrText}
}

// exitCodeOf reports whether err is a remote command exit status.
func exitCodeOf(err error) (int, bool) {
	var exitErr *gossh.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitStatus(), true
	}
	return 0, false
}
