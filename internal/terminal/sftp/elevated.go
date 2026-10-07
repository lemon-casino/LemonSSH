package sftp

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	pkgsftp "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// OpenElevated starts a binary SFTP channel without a PTY. Non-interactive sudo
// fails explicitly when the account needs authentication rather than hanging
// or silently falling back to an unprivileged subsystem.
func OpenElevated(ctx context.Context, client *ssh.Client) (*pkgsftp.Client, io.Closer, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, nil, err
	}
	fail := func(err error) (*pkgsftp.Client, io.Closer, error) {
		_ = session.Close()
		return nil, nil, fmt.Errorf("sudo sftp: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return fail(err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		return fail(err)
	}
	diagnostic := &boundedDiagnostic{}
	session.Stderr = diagnostic
	command := `for p in /usr/lib/openssh/sftp-server /usr/libexec/openssh/sftp-server /usr/lib/ssh/sftp-server /usr/libexec/sftp-server; do if [ -x "$p" ]; then exec sudo -n -- "$p"; fi; done; printf '%s\n' 'sftp-server executable not found' >&2; exit 127`
	if err := session.Start(command); err != nil {
		return fail(err)
	}
	raw, err := negotiateElevated(ctx, stdout, stdin, session.Close, diagnostic.String)
	if err != nil {
		return fail(err)
	}
	return raw, session, nil
}

type boundedDiagnostic struct {
	mu   sync.Mutex
	text strings.Builder
}

func (b *boundedDiagnostic) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if remaining := 8192 - b.text.Len(); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.text.Write(p)
	}
	return n, nil
}
func (b *boundedDiagnostic) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.text.String() }

func negotiateElevated(ctx context.Context, reader io.Reader, writer io.WriteCloser, closeChannel func() error, diagnostic func() string) (*pkgsftp.Client, error) {
	type result struct {
		client *pkgsftp.Client
		err    error
	}
	done := make(chan result, 1)
	go func() { client, err := pkgsftp.NewClientPipe(reader, writer); done <- result{client, err} }()
	select {
	case r := <-done:
		if r.err != nil {
			return nil, fmt.Errorf("sudo sftp negotiation: %w (%s)", r.err, strings.TrimSpace(diagnostic()))
		}
		return r.client, nil
	case <-ctx.Done():
		_ = closeChannel()
		go func() {
			r := <-done
			if r.client != nil {
				_ = r.client.Close()
			}
		}()
		return nil, fmt.Errorf("sudo sftp negotiation: %w", ctx.Err())
	}
}
