package terminaluse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// JobStatus is the lifecycle state of one queued command. ExitUnknown
// pairs with any non-completed terminal state: the command's outcome was
// not observed, and a zero exit code must never be fabricated (T32).
type JobStatus string

const (
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobStopped   JobStatus = "stopped"
	JobFailed    JobStatus = "failed"
)

// ExitUnknown reports that the process terminated without an observable
// exit status (cancel, deadline, transport loss).
const ExitUnknown = -1

// Job limits and defaults (engineering candidates; W14 locks them into
// config after load testing).
const (
	JobMaxOutput      = 4 << 20 // 4 MiB bounded output per job
	JobDefaultTimeout = 60 * time.Second
	JobMaxDeadline    = 30 * time.Minute
)

// Job is one queued command execution.
type Job struct {
	ID            string
	SessionID     string
	Owner         string // the principal that started it
	CommandDigest string // sha256 prefix of the command; the command itself is not retained
	StartedAtMS   int64
	DeadlineMS    int64

	mu              sync.Mutex
	status          JobStatus
	output          []byte
	exitCode        int
	exitKnown       bool
	cancel          context.CancelFunc
	cancelRequested bool
	done            chan struct{}
}

// Snapshot copies the observable job state.
func (j *Job) Snapshot() (JobStatus, []byte, int, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]byte, len(j.output))
	copy(out, j.output)
	return j.status, out, j.exitCode, j.exitKnown
}

func (j *Job) appendOutput(chunk []byte) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.output)+len(chunk) > JobMaxOutput {
		keep := JobMaxOutput - len(j.output)
		if keep > 0 {
			j.output = append(j.output, chunk[:keep]...)
		}
		return
	}
	j.output = append(j.output, chunk...)
}

// CommandRunner executes one command bound to a session's transport and
// reports the outcome. known is false when the run ended without an
// observable exit status.
type CommandRunner interface {
	Run(ctx context.Context, command string, sink func([]byte)) (exitCode int, known bool, err error)
}

// JobQueue serializes side-effect execution per terminal across chats,
// keeps bounded output for incremental polling and scopes stop/poll to the
// owning principal. Stop never queues behind normal execution.
type JobQueue struct {
	maxDeadline time.Duration
	now         func() time.Time
	// runnerFor binds a session to its transport-specific runner; nil
	// sessions fail with a typed error from the Service.
	runnerFor func(sessionID string) (CommandRunner, error)

	mu           sync.Mutex
	jobs         map[string]*Job
	sessionLocks map[string]*sync.Mutex
	counter      int
}

func NewJobQueue(runnerFor func(sessionID string) (CommandRunner, error)) *JobQueue {
	return &JobQueue{
		maxDeadline:  JobMaxDeadline,
		now:          time.Now,
		runnerFor:    runnerFor,
		jobs:         map[string]*Job{},
		sessionLocks: map[string]*sync.Mutex{},
	}
}

func (q *JobQueue) sessionLock(sessionID string) *sync.Mutex {
	q.mu.Lock()
	defer q.mu.Unlock()
	lock := q.sessionLocks[sessionID]
	if lock == nil {
		lock = &sync.Mutex{}
		q.sessionLocks[sessionID] = lock
	}
	return lock
}

func (q *JobQueue) newJobID() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.counter++
	return fmt.Sprintf("job_%06d", q.counter)
}

func commandDigest(command string) string {
	sum := sha256.Sum256([]byte(command))
	return hex.EncodeToString(sum[:])[:16]
}

// Exec runs a short command synchronously under the session's execution
// lock (cross-chat serialization per terminal, design §6.2) and returns
// the observed outcome.
func (q *JobQueue) Exec(ctx context.Context, sessionID, owner, command string, timeout time.Duration) (output []byte, exitCode int, known bool, err error) {
	if strings.TrimSpace(command) == "" {
		return nil, 0, false, fmt.Errorf("terminaluse: command is required")
	}
	if timeout <= 0 || timeout > q.maxDeadline {
		timeout = JobDefaultTimeout
	}
	runner, err := q.runnerFor(sessionID)
	if err != nil {
		return nil, 0, false, err
	}

	lock := q.sessionLock(sessionID)
	lock.Lock()
	defer lock.Unlock()

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var collected []byte
	code, known, runErr := runner.Run(runCtx, command, func(chunk []byte) {
		collected = append(collected, chunk...)
	})
	if runErr != nil {
		return nil, code, known, runErr
	}
	return collected, code, known, nil
}

// Start launches a long-running command and returns immediately. The job
// records owner, command digest and deadline; output accumulates in a
// bounded buffer for JobPoll.
func (q *JobQueue) Start(sessionID, owner, command string, deadline time.Duration) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("terminaluse: command is required")
	}
	if owner == "" {
		return "", fmt.Errorf("terminaluse: job owner is required")
	}
	if deadline <= 0 || deadline > q.maxDeadline {
		deadline = q.maxDeadline
	}
	runner, err := q.runnerFor(sessionID)
	if err != nil {
		return "", err
	}

	lock := q.sessionLock(sessionID)
	lock.Lock()
	defer lock.Unlock()

	started := q.now()
	runCtx, cancel := context.WithDeadline(context.Background(), started.Add(deadline))
	job := &Job{
		ID:            q.newJobID(),
		SessionID:     sessionID,
		Owner:         owner,
		CommandDigest: commandDigest(command),
		StartedAtMS:   started.UnixMilli(),
		DeadlineMS:    started.Add(deadline).UnixMilli(),
		status:        JobRunning,
		cancel:        cancel,
		done:          make(chan struct{}),
	}
	q.mu.Lock()
	q.jobs[job.ID] = job
	q.mu.Unlock()

	go func() {
		defer close(job.done)
		code, known, runErr := runner.Run(runCtx, command, job.appendOutput)
		job.mu.Lock()
		defer job.mu.Unlock()
		switch {
		case job.cancelRequested:
			job.status = JobStopped
			job.exitCode = ExitUnknown
		case runErr != nil && !known:
			job.status = JobFailed
			job.exitCode = ExitUnknown
		default:
			job.status = JobCompleted
			job.exitCode = code
			job.exitKnown = known
		}
	}()
	return job.ID, nil
}

// Poll returns incremental output after the byte offset plus the current
// job state. Owner must match the starter.
func (q *JobQueue) Poll(jobID, owner string, offset int) ([]byte, JobStatus, int, bool, error) {
	job := q.lookup(jobID)
	if job == nil {
		return nil, "", 0, false, fmt.Errorf("terminaluse: job %q not found", jobID)
	}
	if job.Owner != owner {
		return nil, "", 0, false, fmt.Errorf("terminaluse: job %q is owned by another principal", jobID)
	}
	status, output, exitCode, known := job.Snapshot()
	if offset < 0 || offset > len(output) {
		return nil, "", 0, false, fmt.Errorf("terminaluse: offset %d out of range (0..%d)", offset, len(output))
	}
	return output[offset:], status, exitCode, known, nil
}

// Stop cancels a job the principal owns and waits for the runner to
// return. It never queues behind session execution (control path).
func (q *JobQueue) Stop(jobID, owner string) (JobStatus, error) {
	job := q.lookup(jobID)
	if job == nil {
		return "", fmt.Errorf("terminaluse: job %q not found", jobID)
	}
	if job.Owner != owner {
		return "", fmt.Errorf("terminaluse: job %q is owned by another principal", jobID)
	}
	job.mu.Lock()
	cancel := job.cancel
	job.cancelRequested = true
	job.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	select {
	case <-job.done:
	case <-time.After(10 * time.Second):
		return "", fmt.Errorf("terminaluse: job %q did not converge after stop", jobID)
	}
	status, _, _, _ := job.Snapshot()
	return status, nil
}

func (q *JobQueue) lookup(jobID string) *Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.jobs[jobID]
}

// sshRunner runs commands on a dedicated SSH exec channel — never the
// interactive PTY — and reads the real exit status.
type sshRunner struct {
	client *gossh.Client
}

func (r sshRunner) Run(ctx context.Context, command string, sink func([]byte)) (int, bool, error) {
	session, err := r.client.NewSession()
	if err != nil {
		return ExitUnknown, false, fmt.Errorf("terminaluse: exec channel open failed: %w", err)
	}
	defer session.Close()

	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			_ = session.Close()
		case <-stopped:
		}
	}()

	output := &limitedSink{sink: sink}
	session.Stdout = output
	session.Stderr = output
	runErr := session.Run(command)
	if ctx.Err() != nil {
		return ExitUnknown, false, ctx.Err()
	}
	if exitErr, ok := runErr.(*gossh.ExitError); ok {
		return exitErr.ExitStatus(), true, nil
	}
	if runErr != nil {
		return ExitUnknown, false, runErr
	}
	return 0, true, nil
}

type limitedSink struct {
	sink func([]byte)
	seen int
}

func (l *limitedSink) Write(p []byte) (int, error) {
	if l.seen+len(p) > JobMaxOutput {
		keep := JobMaxOutput - l.seen
		if keep > 0 {
			l.seen += keep
			l.sink(p[:keep])
		}
		return len(p), nil // drop the rest silently; output is bounded
	}
	l.seen += len(p)
	l.sink(p)
	return len(p), nil
}

// localRunner runs commands in a fresh noninteractive shell — never the
// visible PTY — so agent commands and the user's shell state stay
// independent (design §6.2).
type localRunner struct{}

func (localRunner) Run(ctx context.Context, command string, sink func([]byte)) (int, bool, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}
	cmd.WaitDelay = 250 * time.Millisecond
	output := &limitedSink{sink: sink}
	cmd.Stdout = output
	cmd.Stderr = output
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return ExitUnknown, false, ctx.Err()
	}
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		return exitErr.ExitCode(), true, nil
	}
	if runErr != nil {
		return ExitUnknown, false, runErr
	}
	return 0, true, nil
}

// RunnerFor binds a session to its transport-specific command runner:
// SSH sessions use dedicated exec channels, local sessions use a fresh
// noninteractive shell, other transports fail typed (T32: no fake
// semantics for unsupported transports).
func (s *Service) RunnerFor(sessionID string) (CommandRunner, error) {
	term, ok := s.lookup(sessionID)
	if !ok {
		return nil, fmt.Errorf("terminaluse: session %q is not connected", sessionID)
	}
	if term.transport != nil && term.transport.Client != nil {
		return sshRunner{client: term.transport.Client}, nil
	}
	if term.local != nil {
		return localRunner{}, nil
	}
	return nil, fmt.Errorf("terminaluse: command execution is unsupported for this terminal transport")
}
