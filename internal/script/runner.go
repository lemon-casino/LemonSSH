package script

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// SessionWriter writes bytes into an existing terminal session.
type SessionWriter func(sessionID string, data []byte) error

type Run struct {
	RunID       string   `json:"runId"`
	ScriptID    string   `json:"scriptId,omitempty"`
	ScriptLabel string   `json:"scriptLabel,omitempty"`
	SessionID   string   `json:"sessionId"`
	Status      string   `json:"status"`
	StartedAt   int64    `json:"startedAt"`
	EndedAt     int64    `json:"endedAt,omitempty"`
	Error       string   `json:"error,omitempty"`
	Logs        []RunLog `json:"logs"`
}

type RunLog struct {
	At      int64  `json:"at"`
	Message string `json:"message"`
}

type Runner struct {
	mu      sync.Mutex
	runs    map[string]*Run
	cancels map[string]context.CancelFunc
	output  map[string]*OutputWatch
	write   SessionWriter
	sleep   func(context.Context, time.Duration) error
}

func NewRunner(write SessionWriter) *Runner {
	return &Runner{
		runs:    make(map[string]*Run),
		cancels: make(map[string]context.CancelFunc),
		output:  make(map[string]*OutputWatch),
		write:   write,
		sleep: func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

func (r *Runner) SetWriter(write SessionWriter) {
	r.mu.Lock()
	r.write = write
	r.mu.Unlock()
}

func (r *Runner) ObserveOutput(sessionID string, data []byte) {
	if sessionID == "" || len(data) == 0 {
		return
	}
	r.mu.Lock()
	watch := r.output[sessionID]
	if watch == nil {
		watch = &OutputWatch{}
		r.output[sessionID] = watch
	}
	r.mu.Unlock()
	watch.Append(data)
}

func (r *Runner) watch(sessionID string) *OutputWatch {
	r.mu.Lock()
	defer r.mu.Unlock()
	watch := r.output[sessionID]
	if watch == nil {
		watch = &OutputWatch{}
		r.output[sessionID] = watch
	}
	return watch
}

type StartRunRequest struct {
	RunID       string
	ScriptID    string
	ScriptLabel string
	SessionID   string
	Content     string
}

func (r *Runner) Start(req StartRunRequest) (*Run, error) {
	ops, err := ParseRecordedScript(req.Content)
	if err != nil {
		return nil, err
	}
	sessionID := req.SessionID
	if sessionID == "" {
		return nil, fmt.Errorf("sessionId required")
	}
	runID := req.RunID
	if runID == "" {
		runID = newRunID()
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &Run{
		RunID:       runID,
		ScriptID:    req.ScriptID,
		ScriptLabel: req.ScriptLabel,
		SessionID:   sessionID,
		Status:      "running",
		StartedAt:   time.Now().UnixMilli(),
		Logs:        []RunLog{},
	}
	r.mu.Lock()
	r.runs[runID] = run
	r.cancels[runID] = cancel
	write := r.write
	r.mu.Unlock()
	go r.execute(ctx, run, ops, write)
	return cloneRun(run), nil
}

func (r *Runner) Stop(runID string) bool {
	r.mu.Lock()
	cancel := r.cancels[runID]
	run := r.runs[runID]
	r.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	if run != nil {
		r.finish(run, "failed", "Stopped by user")
	}
	return true
}

func (r *Runner) List(sessionID string) []Run {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Run, 0, len(r.runs))
	for _, run := range r.runs {
		if sessionID != "" && run.SessionID != sessionID {
			continue
		}
		out = append(out, *cloneRun(run))
	}
	return out
}

func (r *Runner) execute(ctx context.Context, run *Run, ops []ReplayOp, write SessionWriter) {
	defer func() {
		r.mu.Lock()
		delete(r.cancels, run.RunID)
		r.mu.Unlock()
	}()
	if write == nil {
		r.finish(run, "failed", "terminal writer unavailable")
		return
	}
	for _, op := range ops {
		if ctx.Err() != nil {
			return
		}
		switch op.Kind {
		case "sleep":
			r.log(run, fmt.Sprintf("sleep %s", op.Timeout))
			if err := r.sleep(ctx, op.Timeout); err != nil {
				return
			}
		case "sendLine":
			label := op.Value
			if op.Sensitive {
				label = "[sensitive]"
			}
			r.log(run, "→ "+label)
			if op.Value != "" {
				if err := write(run.SessionID, []byte(op.Value)); err != nil {
					r.finish(run, "failed", err.Error())
					return
				}
				if err := r.sleep(ctx, 30*time.Millisecond); err != nil {
					return
				}
			}
			if err := write(run.SessionID, []byte("\r")); err != nil {
				r.finish(run, "failed", err.Error())
				return
			}
		case "waitForPrompt", "waitForText":
			wait := op.Timeout
			if wait <= 0 {
				wait = 30 * time.Second
			}
			r.log(run, "wait "+op.Kind)
			if err := r.waitFor(ctx, run.SessionID, op, wait); err != nil {
				if ctx.Err() != nil {
					return
				}
				r.finish(run, "failed", err.Error())
				return
			}
		default:
			r.finish(run, "failed", "unsupported replay op "+op.Kind)
			return
		}
	}
	r.finish(run, "completed", "")
}

func (r *Runner) waitFor(ctx context.Context, sessionID string, op ReplayOp, timeout time.Duration) error {
	watch := r.watch(sessionID)
	deadline := time.Now().Add(timeout)
	for {
		text := validUTF8Tail(watch.snapshot())
		if op.Kind == "waitForText" {
			if containsFresh(text, op.Value) {
				return nil
			}
		} else if looksLikePrompt(text) {
			return nil
		}
		if time.Now().After(deadline) {
			if op.Kind == "waitForText" {
				return fmt.Errorf("timed out waiting for %q", op.Value)
			}
			return fmt.Errorf("timed out waiting for shell prompt")
		}
		remaining := time.Until(deadline)
		notify := watch.notify()
		timer := time.NewTimer(remaining)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-notify:
			timer.Stop()
		case <-timer.C:
			if op.Kind == "waitForText" {
				return fmt.Errorf("timed out waiting for %q", op.Value)
			}
			return fmt.Errorf("timed out waiting for shell prompt")
		}
	}
}

func (r *Runner) log(run *Run, message string) {
	r.mu.Lock()
	run.Logs = append(run.Logs, RunLog{At: time.Now().UnixMilli(), Message: message})
	r.mu.Unlock()
}

func (r *Runner) finish(run *Run, status, errText string) {
	r.mu.Lock()
	if run.EndedAt != 0 {
		r.mu.Unlock()
		return
	}
	run.Status = status
	run.EndedAt = time.Now().UnixMilli()
	run.Error = errText
	r.mu.Unlock()
}

func cloneRun(run *Run) *Run {
	copied := *run
	copied.Logs = append([]RunLog(nil), run.Logs...)
	return &copied
}

func newRunID() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return "run-" + hex.EncodeToString(buf[:])
}
