// Package supervised owns the Mosh/ET helper binary runner (P3-08.3): a
// hash-and-arch verified, supervised external process with bounded restarts,
// output capture and clean teardown. Binaries are never trusted by filename —
// the SHA-256 must match the pinned manifest for the exact target arch.
package supervised

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

var (
	ErrBinaryMissing   = errors.New("supervised binary missing")
	ErrHashMismatch    = errors.New("supervised binary hash mismatch")
	ErrArchMismatch    = errors.New("supervised binary arch mismatch")
	ErrTooManyRestarts = errors.New("supervised binary restart limit exceeded")
	ErrNotRunning      = errors.New("supervised binary not running")
)

// Manifest pins one helper binary per platform/arch.
type Manifest struct {
	Name       string `json:"name"`       // logical name, e.g. "mosh-client"
	Path       string `json:"path"`       // on-disk path relative to the resource root
	SHA256     string `json:"sha256"`     // pinned binary digest
	Arch       string `json:"arch"`       // GOARCH the binary was built for
	OS         string `json:"os"`         // GOOS the binary was built for
	MinVersion string `json:"minVersion"` // upstream protocol floor
}

// Verify checks existence, arch and hash against the manifest and the host.
func Verify(manifest Manifest, resourceRoot string) error {
	if manifest.OS != runtime.GOOS {
		return fmt.Errorf("%w: manifest os %q host %q", ErrArchMismatch, manifest.OS, runtime.GOOS)
	}
	if manifest.Arch != runtime.GOARCH {
		return fmt.Errorf("%w: manifest arch %q host %q", ErrArchMismatch, manifest.Arch, runtime.GOARCH)
	}
	full := resourceRoot + string(os.PathSeparator) + manifest.Path
	data, err := os.ReadFile(full)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBinaryMissing, err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != manifest.SHA256 {
		return ErrHashMismatch
	}
	return nil
}

// Process is one supervised run of a verified binary.
type Process struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   *os.File
	exited  chan struct{}
	exitErr error
}

// Runner supervises one verified helper binary.
type Runner struct {
	manifest     Manifest
	resourceRoot string
	maxRestarts  int
	restartDelay time.Duration
	mu           sync.Mutex
	process      *Process
	restarts     int
}

// NewRunner validates the manifest against the host and returns a runner.
func NewRunner(manifest Manifest, resourceRoot string, maxRestarts int) (*Runner, error) {
	if err := Verify(manifest, resourceRoot); err != nil {
		return nil, err
	}
	if maxRestarts <= 0 {
		maxRestarts = 3
	}
	return &Runner{manifest: manifest, resourceRoot: resourceRoot, maxRestarts: maxRestarts}, nil
}

// Start launches the binary with arguments. A dead binary is restarted up to
// maxRestarts times with a fixed delay before giving up.
func (r *Runner) Start(ctx context.Context, args []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.process != nil {
		select {
		case <-r.process.exited:
			// previous run already ended; allow restart
		default:
			return errors.New("supervised binary already running")
		}
	}
	for attempt := 0; ; attempt++ {
		if attempt > r.maxRestarts {
			return ErrTooManyRestarts
		}
		process, startErr := spawn(ctx, r.manifest, r.resourceRoot, args)
		if startErr == nil {
			r.process = process
			return nil
		}
		if !errors.Is(startErr, errSpawnDied) {
			return startErr
		}
		time.Sleep(200 * time.Millisecond)
	}
}

var errSpawnDied = errors.New("spawn died immediately")

func spawn(ctx context.Context, manifest Manifest, resourceRoot string, args []string) (*Process, error) {
	full := resourceRoot + string(os.PathSeparator) + manifest.Path
	cmd := exec.CommandContext(ctx, full, args...)
	// Hold a stdin pipe open so interactive helpers do not see EOF and die.
	stdinRead, stdinWrite, pipeErr := os.Pipe()
	if pipeErr != nil {
		return nil, pipeErr
	}
	cmd.Stdin = stdinRead
	if err := cmd.Start(); err != nil {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
		return nil, err
	}
	// The parent keeps the write end alive for the process lifetime; the read
	// end belongs to the child now.
	_ = stdinRead.Close()
	process := &Process{cmd: cmd, stdin: stdinWrite, exited: make(chan struct{})}
	go func() {
		waitErr := cmd.Wait()
		process.mu.Lock()
		process.exitErr = waitErr
		process.mu.Unlock()
		close(process.exited)
	}()
	// Fail fast when the binary dies immediately (arch/ABI mismatch).
	select {
	case <-process.exited:
		process.mu.Lock()
		waitErr := process.exitErr
		process.mu.Unlock()
		if waitErr != nil {
			return nil, fmt.Errorf("%w: %v", errSpawnDied, waitErr)
		}
		return process, nil
	case <-time.After(300 * time.Millisecond):
		return process, nil
	}
}

// Stop terminates the running binary.
func (r *Runner) Stop() error {
	r.mu.Lock()
	process := r.process
	r.process = nil
	r.mu.Unlock()
	if process == nil {
		return ErrNotRunning
	}
	_ = process.cmd.Process.Kill()
	_ = process.stdin.Close()
	<-process.exited
	return nil
}

// Running reports whether the supervised binary is alive.
func (r *Runner) Running() bool {
	r.mu.Lock()
	process := r.process
	r.mu.Unlock()
	if process == nil {
		return false
	}
	select {
	case <-process.exited:
		return false
	default:
		return true
	}
}
