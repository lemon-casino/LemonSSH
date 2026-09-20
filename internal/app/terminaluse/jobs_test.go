package terminaluse

import (
	"context"
	"errors"
	"github.com/binaricat/netcatty/internal/terminal/pty"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRunner is a programmable CommandRunner.
type fakeRunner struct {
	mu        sync.Mutex
	runs      int
	output    string
	exitCode  int
	known     bool
	err       error
	blockChan chan struct{}
	block     bool
	lastCmd   string
}

func (f *fakeRunner) Run(ctx context.Context, command string, sink func([]byte)) (int, bool, error) {
	f.mu.Lock()
	f.runs++
	f.lastCmd = command
	block := f.block
	output, code, known, runErr := f.output, f.exitCode, f.known, f.err
	f.mu.Unlock()

	if block {
		select {
		case <-ctx.Done():
			return ExitUnknown, false, ctx.Err()
		case <-f.blockChan:
			sink([]byte("late output"))
			return code, known, runErr
		}
	}
	if output != "" {
		sink([]byte(output))
	}
	return code, known, runErr
}

func newTestQueue(runner CommandRunner) *JobQueue {
	return NewJobQueue(func(sessionID string) (CommandRunner, error) {
		if sessionID == "missing" {
			return nil, errors.New("not connected")
		}
		return runner, nil
	})
}

func TestPollTextPagesUnicodeAndRejectsForeignOwners(t *testing.T) {
	text := strings.Repeat("a", 15999) + "😀结束"
	queue := newTestQueue(&fakeRunner{output: text, known: true})
	id, err := queue.Start("session", "chat", "echo", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-queue.lookup(id).done:
	case <-time.After(time.Second):
		t.Fatal("runner blocked")
	}
	page, err := queue.PollText(id, "chat", 0)
	if err != nil || page["nextOffset"] != 15999 {
		t.Fatalf("%v %v", page, err)
	}
	page, err = queue.PollText(id, "chat", page["nextOffset"].(int))
	if err != nil || page["output"] != "😀结束" || page["nextOffset"] != 16003 {
		t.Fatalf("%v %v", page, err)
	}
	if _, err = queue.PollText(id, "other", 0); err == nil {
		t.Fatal("foreign owner read job output")
	}
}

func TestLocalToolUsesConfiguredShellDirectoryAndEnvironment(t *testing.T) {
	shell, command := "/bin/sh", "printf '%s\n%s' \"$PWD\" \"$LEMONSSH_TOOL_FIXTURE\""
	if runtime.GOOS == "windows" {
		var err error
		shell, err = exec.LookPath("powershell.exe")
		if err != nil {
			t.Skip("PowerShell unavailable")
		}
		command = "Write-Output $PWD.Path; Write-Output $env:LEMONSSH_TOOL_FIXTURE"
	}
	dir := t.TempDir()
	runner := localRunner{config: pty.Config{Shell: shell, CWD: dir, Env: []string{"LEMONSSH_TOOL_FIXTURE=custom-value"}}}
	var out strings.Builder
	code, known, err := runner.Run(context.Background(), command, func(data []byte) { out.Write(data) })
	if err != nil || !known || code != 0 || !strings.Contains(out.String(), dir) || !strings.Contains(out.String(), "custom-value") {
		t.Fatalf("local execution: %q %d %v %v", out.String(), code, known, err)
	}
}

func TestCancelledQueuedExecNeverRuns(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	runner := &fakeRunner{block: true, blockChan: release}
	queue := newTestQueue(runner)
	id, _ := queue.Start("session", "first", "wait", time.Minute)
	go func() {
		for {
			runner.mu.Lock()
			runs := runner.runs
			runner.mu.Unlock()
			if runs > 0 {
				close(started)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("job did not start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, _, err := queue.Exec(ctx, "session", "second", "must not run", time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("queued exec: %v", err)
	}
	queue.CancelOwner("first")
	select {
	case <-queue.lookup(id).done:
	case <-time.After(time.Second):
		t.Fatal("job did not stop")
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.runs != 1 {
		t.Fatalf("cancelled command ran: %d", runner.runs)
	}
}

func TestExecReturnsOutcome(t *testing.T) {
	queue := newTestQueue(&fakeRunner{output: "done", exitCode: 3, known: true})
	output, code, known, err := queue.Exec(context.Background(), "sess-1", "owner", "ls", 0)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if string(output) != "done" || code != 3 || !known {
		t.Errorf("exec outcome = %q %d %v", output, code, known)
	}

	if _, _, _, err := queue.Exec(context.Background(), "missing", "owner", "ls", 0); err == nil {
		t.Errorf("exec on unconnected session must fail")
	}
	if _, _, _, err := queue.Exec(context.Background(), "sess-1", "owner", "  ", 0); err == nil {
		t.Errorf("blank command must fail")
	}
}

// TestExecSerializesPerSession pins §6.2: two chats executing on the same
// terminal wait for each other; different terminals run in parallel.
func TestExecSerializesPerSession(t *testing.T) {
	tracker := &serializedRunner{active: 0}
	queue := newTestQueue(tracker)

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _, _ = queue.Exec(context.Background(), "sess-1", "chat-a", "cmd", time.Second)
		}()
	}
	wg.Wait()
	if tracker.peak() > 1 {
		t.Errorf("same-session exec overlapped %d times", tracker.peak())
	}

	trackerB := &serializedRunner{active: 0}
	queueB := NewJobQueue(func(sessionID string) (CommandRunner, error) {
		return trackerB, nil
	})
	var wg2 sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			_, _, _, _ = queueB.Exec(context.Background(), "sess-b", "chat-b", "cmd", time.Second)
		}()
	}
	// Interleave on a different session: both must make progress.
	wg2.Wait()
	if trackerB.peak() != 1 {
		t.Errorf("cross-session executions serialized incorrectly (max %d)", trackerB.peak())
	}
}

type serializedRunner struct {
	mu            sync.Mutex
	active        int
	maxConcurrent int
}

func (r *serializedRunner) Run(ctx context.Context, command string, sink func([]byte)) (int, bool, error) {
	r.mu.Lock()
	r.active++
	if r.active > r.maxConcurrent {
		r.maxConcurrent = r.active
	}
	r.mu.Unlock()
	time.Sleep(20 * time.Millisecond)
	r.mu.Lock()
	r.active--
	r.mu.Unlock()
	return 0, true, nil
}

func (r *serializedRunner) peak() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.maxConcurrent
}

func TestJobLifecycleWithPollOffsets(t *testing.T) {
	queue := newTestQueue(&fakeRunner{output: "hello world", exitCode: 0, known: true})
	jobID, err := queue.Start("sess-1", "chat-1", "build.sh", time.Minute)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		output, status, code, known, err := queue.Poll(jobID, "chat-1", 0)
		if err != nil {
			t.Fatalf("poll: %v", err)
		}
		if status == JobCompleted {
			if string(output) != "hello world" || code != 0 || !known {
				t.Errorf("final poll = %q %d %v", output, code, known)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job never completed")
		}
		time.Sleep(time.Millisecond)
	}

	// Offset polls return only the tail.
	tail, status, _, known, err := queue.Poll(jobID, "chat-1", 6)
	if err != nil {
		t.Fatalf("tail poll: %v", err)
	}
	if string(tail) != "world" || status != JobCompleted || !known {
		t.Errorf("tail = %q %v %v", tail, status, known)
	}

	// Out-of-range offset fails instead of lying about content.
	if _, _, _, _, err := queue.Poll(jobID, "chat-1", 99); err == nil {
		t.Errorf("out-of-range offset must fail")
	}
}

func TestJobOwnerScoping(t *testing.T) {
	queue := newTestQueue(&fakeRunner{output: "x", exitCode: 0, known: true})
	jobID, err := queue.Start("sess-1", "chat-1", "cmd", time.Minute)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, _, _, _, err := queue.Poll(jobID, "chat-2", 0); err == nil {
		t.Errorf("foreign principal must not poll")
	}
	if _, err := queue.Stop(jobID, "chat-2"); err == nil {
		t.Errorf("foreign principal must not stop")
	}
}

func TestJobStopConvergesToStopped(t *testing.T) {
	gate := make(chan struct{})
	runner := &fakeRunner{block: true, blockChan: gate, exitCode: 0, known: true}
	queue := newTestQueue(runner)
	jobID, err := queue.Start("sess-1", "chat-1", "long-runner", time.Minute)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	status, err := queue.Stop(jobID, "chat-1")
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if status != JobStopped {
		t.Errorf("status = %q, want stopped", status)
	}
	close(gate) // let the runner goroutine drain

	// Terminal state stays stable across further polls.
	_, status, _, _, err = queue.Poll(jobID, "chat-1", 0)
	if err != nil || status != JobStopped {
		t.Errorf("post-stop poll = %q (%v)", status, err)
	}
}

// TestJobDeadlineOutcomeUnknown pins T32: a deadline expiry must surface
// as failed/unknown, never a fabricated zero exit code.
func TestJobDeadlineOutcomeUnknown(t *testing.T) {
	runner := &fakeRunner{block: true, blockChan: make(chan struct{})}
	queue := newTestQueue(runner)
	jobID, err := queue.Start("sess-1", "chat-1", "hangs", 30*time.Millisecond)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, status, code, known, err := queue.Poll(jobID, "chat-1", 0)
		if err != nil {
			t.Fatalf("poll: %v", err)
		}
		if status == JobFailed {
			if known || code != ExitUnknown {
				t.Errorf("deadline outcome must be unknown, got code=%d known=%v", code, known)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("deadline never observed")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestExecOutcomeUnknownOnTimeout(t *testing.T) {
	runner := &fakeRunner{block: true, blockChan: make(chan struct{})}
	queue := newTestQueue(runner)
	_, code, known, err := queue.Exec(context.Background(), "sess-1", "chat-1", "hangs", 30*time.Millisecond)
	if err == nil {
		t.Fatal("timed-out exec must fail")
	}
	if known || code != ExitUnknown {
		t.Errorf("timed-out exec must not fabricate an exit code, got %d/%v", code, known)
	}
}

func TestJobOutputBounded(t *testing.T) {
	huge := strings.Repeat("x", JobMaxOutput+1024)
	runner := &fakeRunner{output: huge, exitCode: 0, known: true}
	queue := newTestQueue(runner)
	jobID, err := queue.Start("sess-1", "chat-1", "spew", time.Minute)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		output, status, _, _, err := queue.Poll(jobID, "chat-1", 0)
		if err != nil {
			t.Fatalf("poll: %v", err)
		}
		if status == JobCompleted {
			if len(output) > JobMaxOutput {
				t.Errorf("output = %d bytes, exceeds bound", len(output))
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job never completed")
		}
		time.Sleep(time.Millisecond)
	}
}
