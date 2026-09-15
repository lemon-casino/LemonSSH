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

// DialogRequest mirrors the renderer's ScriptDialogRequest contract so the
// existing dialog host renders it unchanged.
type DialogRequest struct {
	RequestID    string `json:"requestId"`
	Type         string `json:"type"`
	Message      string `json:"message"`
	DefaultValue string `json:"defaultValue,omitempty"`
	Sensitive    bool   `json:"sensitive,omitempty"`
}

type DialogResponder func(ctx context.Context, request DialogRequest) (value string, cancelled bool, err error)

type Runner struct {
	mu             sync.Mutex
	runs           map[string]*Run
	cancels        map[string]context.CancelFunc
	output         map[string]*OutputWatch
	paused         map[string]chan struct{}
	pendingDialogs map[string]chan DialogAnswer
	write          SessionWriter
	dialog         DialogResponder
	sleep          func(context.Context, time.Duration) error
}

func NewRunner(write SessionWriter) *Runner {
	return &Runner{
		runs:           make(map[string]*Run),
		cancels:        make(map[string]context.CancelFunc),
		output:         make(map[string]*OutputWatch),
		paused:         make(map[string]chan struct{}),
		pendingDialogs: make(map[string]chan DialogAnswer),
		write:          write,
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

func (r *Runner) SetDialogResponder(respond DialogResponder) {
	r.mu.Lock()
	r.dialog = respond
	r.mu.Unlock()
}

// ResolveDialog routes a renderer answer to the waiting prompt.
func (r *Runner) ResolveDialog(requestID string, value string, cancelled bool) bool {
	r.mu.Lock()
	waiting := r.pendingDialogs[requestID]
	r.mu.Unlock()
	if waiting == nil {
		return false
	}
	waiting <- DialogAnswer{Value: value, Cancelled: cancelled}
	return true
}

type DialogAnswer struct {
	Value     string
	Cancelled bool
}

type dialogWaiter struct {
	ch chan DialogAnswer
}

var dialogWaitTimeout = 120 * time.Second

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
	snapshot := cloneRun(run)
	r.mu.Unlock()
	go r.execute(ctx, run, ops, write)
	return snapshot, nil
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

// Pause flags a running run; the executor pauses before the next op.
func (r *Runner) Pause(runID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[runID]
	if run == nil || run.Status != "running" || r.paused[runID] != nil {
		return false
	}
	r.paused[runID] = make(chan struct{})
	run.Status = "paused"
	return true
}

// Resume releases a paused run.
func (r *Runner) Resume(runID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	done := r.paused[runID]
	run := r.runs[runID]
	if done == nil || run == nil {
		return false
	}
	close(done)
	delete(r.paused, runID)
	if run.Status == "paused" {
		run.Status = "running"
	}
	return true
}

func (r *Runner) waitIfPaused(ctx context.Context, runID string) error {
	r.mu.Lock()
	done := r.paused[runID]
	r.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
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
	vars := make(map[string]string)
	for _, op := range ops {
		if ctx.Err() != nil {
			return
		}
		if err := r.waitIfPaused(ctx, run.RunID); err != nil {
			return
		}
		switch op.Kind {
		case "sleep":
			r.log(run, fmt.Sprintf("sleep %s", op.Timeout))
			if err := r.sleep(ctx, op.Timeout); err != nil {
				return
			}
		case "prompt":
			value, cancelled, err := r.askDialog(ctx, run, op)
			if err != nil {
				r.finish(run, "failed", err.Error())
				return
			}
			if cancelled {
				r.finish(run, "failed", "Dialog cancelled")
				return
			}
			vars[op.Var] = value
		case "log":
			r.log(run, op.Value)
		case "alert":
			if _, _, err := r.askDialog(ctx, run, ReplayOp{Kind: "alert", Value: op.Value}); err != nil && ctx.Err() == nil {
				r.finish(run, "failed", err.Error())
				return
			}
		case "sendLine":
			value := op.Value
			if op.Var != "" {
				resolved, ok := vars[op.Var]
				if !ok {
					r.finish(run, "failed", "variable "+op.Var+" has no value")
					return
				}
				value = resolved
			}
			label := value
			if op.Sensitive {
				label = "[sensitive]"
			}
			r.log(run, "→ "+label)
			if value != "" {
				if err := write(run.SessionID, []byte(value)); err != nil {
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

// askDialog emits the renderer dialog contract and blocks for the answer.
func (r *Runner) askDialog(ctx context.Context, run *Run, op ReplayOp) (string, bool, error) {
	r.mu.Lock()
	respond := r.dialog
	r.mu.Unlock()
	if respond == nil {
		return "", false, fmt.Errorf("dialog host unavailable")
	}
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	request := DialogRequest{
		RequestID:    "dlg-" + hex.EncodeToString(buf[:]),
		Type:         "prompt",
		Message:      op.Value,
		DefaultValue: "",
		Sensitive:    op.Sensitive,
	}
	answered := make(chan DialogAnswer, 1)
	r.mu.Lock()
	r.pendingDialogs[request.RequestID] = answered
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.pendingDialogs, request.RequestID)
		r.mu.Unlock()
	}()
	r.log(run, "dialog: "+op.Value)
	_, _, _ = respond(ctx, request)
	timer := time.NewTimer(dialogWaitTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return "", false, ctx.Err()
	case answer := <-answered:
		return answer.Value, answer.Cancelled, nil
	case <-timer.C:
		return "", false, fmt.Errorf("Dialog timed out")
	}
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
