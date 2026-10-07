package script

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// SessionWriter writes bytes into an existing terminal session.
type SessionWriter func(sessionID string, data []byte) error

type SessionCloser func(sessionID string) error
type SessionLogStarter func(sessionID, filePath string) (string, error)
type SessionLogStopper func(sessionID string) error

type SessionSnapshot struct {
	Connected bool   `json:"connected"`
	Name      string `json:"name"`
	Hostname  string `json:"hostname"`
	Username  string `json:"username"`
	Rows      int    `json:"rows"`
	Cols      int    `json:"cols"`
}

type Run struct {
	RunID           string   `json:"runId"`
	ScriptID        string   `json:"scriptId,omitempty"`
	ScriptLabel     string   `json:"scriptLabel,omitempty"`
	SessionID       string   `json:"sessionId"`
	Status          string   `json:"status"`
	StartedAt       int64    `json:"startedAt"`
	EndedAt         int64    `json:"endedAt,omitempty"`
	Error           string   `json:"error,omitempty"`
	Logs            []RunLog `json:"logs"`
	ProgressMode    string   `json:"progressMode,omitempty"`
	ProgressLabel   string   `json:"progressLabel,omitempty"`
	ProgressCurrent int      `json:"progressCurrent,omitempty"`
	ProgressTotal   int      `json:"progressTotal,omitempty"`
	ActivityLabel   string   `json:"activityLabel,omitempty"`
	StepIndex       int      `json:"stepIndex"`
	ElapsedMs       int64    `json:"elapsedMs"`
}

type RunLog struct {
	At      int64  `json:"at"`
	Message string `json:"message"`
}

type DialogRequest struct {
	RequestID    string      `json:"requestId"`
	Type         string      `json:"type"`
	Message      string      `json:"message"`
	DefaultValue string      `json:"defaultValue,omitempty"`
	Sensitive    bool        `json:"sensitive,omitempty"`
	Pattern      string      `json:"pattern,omitempty"`
	TimeoutMs    int64       `json:"timeoutMs,omitempty"`
	Form         *DialogForm `json:"form,omitempty"`
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
	closer         SessionCloser
	startLog       SessionLogStarter
	stopLog        SessionLogStopper
	snapshot       func(string) SessionSnapshot
	version        string
	screenSnapshot func(context.Context, string) (ScreenSnapshot, error)
	dialog         DialogResponder
	onRunsUpdated  func([]Run)
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

func (r *Runner) SetSessionCloser(closer SessionCloser) {
	r.mu.Lock()
	r.closer = closer
	r.mu.Unlock()
}

func (r *Runner) SetSessionLog(start SessionLogStarter, stop SessionLogStopper) {
	r.mu.Lock()
	r.startLog, r.stopLog = start, stop
	r.mu.Unlock()
}

func (r *Runner) SetSessionSnapshot(snapshot func(string) SessionSnapshot, version string) {
	r.mu.Lock()
	r.snapshot, r.version = snapshot, version
	r.mu.Unlock()
}

func (r *Runner) SetRunsListener(listener func([]Run)) {
	r.mu.Lock()
	r.onRunsUpdated = listener
	r.mu.Unlock()
	r.broadcast()
}

func (r *Runner) broadcast() {
	r.mu.Lock()
	listener := r.onRunsUpdated
	if listener == nil {
		r.mu.Unlock()
		return
	}
	snapshot := make([]Run, 0, len(r.runs))
	for _, run := range r.runs {
		snapshot = append(snapshot, *cloneRun(run))
	}
	r.mu.Unlock()
	listener(snapshot)
}

func (r *Runner) SetDialogResponder(respond DialogResponder) {
	r.mu.Lock()
	r.dialog = respond
	r.mu.Unlock()
}

func (r *Runner) ResolveDialog(requestID string, value string, cancelled bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	waiting := r.pendingDialogs[requestID]
	if waiting == nil {
		return false
	}
	delete(r.pendingDialogs, requestID)
	waiting <- DialogAnswer{Value: value, Cancelled: cancelled}
	return true
}

type DialogAnswer struct {
	Value     string
	Cancelled bool
}

var dialogWaitTimeout = 120 * time.Second

func (r *Runner) ObserveOutput(sessionID string, data []byte) {
	if sessionID != "" && len(data) != 0 {
		r.watch(sessionID).Append(data)
	}
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
	RunID          string
	ScriptID       string
	ScriptLabel    string
	SessionID      string
	Content        string
	PermissionMode string
	SessionMeta    *SessionSnapshot
}

func (r *Runner) Start(req StartRunRequest) (*Run, error) {
	program, err := compileScript(req.Content)
	if err != nil {
		return nil, err
	}
	if req.SessionID == "" {
		return nil, fmt.Errorf("sessionId required")
	}
	runID := req.RunID
	if runID == "" {
		runID = newRunID()
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &Run{
		RunID: runID, ScriptID: req.ScriptID, ScriptLabel: req.ScriptLabel,
		SessionID: req.SessionID, Status: "running", StartedAt: time.Now().UnixMilli(), Logs: []RunLog{},
	}
	r.mu.Lock()
	if r.runs[runID] != nil {
		r.mu.Unlock()
		cancel()
		return nil, fmt.Errorf("runId already exists")
	}
	if len(r.cancels) >= 32 {
		r.mu.Unlock()
		cancel()
		return nil, fmt.Errorf("too many active script runs")
	}
	r.runs[runID], r.cancels[runID] = run, cancel
	snapshot := cloneRun(run)
	r.mu.Unlock()
	r.broadcast()
	meta := SessionSnapshot{Connected: true, Rows: 24, Cols: 80}
	if req.SessionMeta != nil {
		meta = *req.SessionMeta
		if meta.Rows <= 0 {
			meta.Rows = 24
		}
		if meta.Cols <= 0 {
			meta.Cols = 80
		}
	}
	go r.executeJavaScript(ctx, cancel, run, program, req.PermissionMode, meta)
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

func (r *Runner) ReleaseSession(sessionID string) {
	r.mu.Lock()
	var ids []string
	for id, run := range r.runs {
		if run.SessionID == sessionID && r.cancels[id] != nil {
			ids = append(ids, id)
		}
	}
	delete(r.output, sessionID)
	r.mu.Unlock()
	for _, id := range ids {
		r.Stop(id)
	}
}

func (r *Runner) List(sessionID string) []Run {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Run, 0, len(r.runs))
	for _, run := range r.runs {
		if sessionID == "" || run.SessionID == sessionID {
			out = append(out, *cloneRun(run))
		}
	}
	return out
}

func (r *Runner) Pause(runID string) bool {
	r.mu.Lock()
	run := r.runs[runID]
	if run == nil || run.Status != "running" || r.paused[runID] != nil {
		r.mu.Unlock()
		return false
	}
	r.paused[runID] = make(chan struct{})
	run.Status = "paused"
	r.mu.Unlock()
	r.broadcast()
	return true
}

func (r *Runner) Resume(runID string) bool {
	r.mu.Lock()
	done, run := r.paused[runID], r.runs[runID]
	if done == nil || run == nil || run.EndedAt != 0 {
		r.mu.Unlock()
		return false
	}
	close(done)
	delete(r.paused, runID)
	run.Status = "running"
	r.mu.Unlock()
	r.broadcast()
	return true
}

func (r *Runner) waitIfPaused(ctx context.Context, runID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
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

func (r *Runner) step(run *Run) {
	r.mu.Lock()
	if run.EndedAt == 0 {
		run.StepIndex++
		run.ElapsedMs = time.Now().UnixMilli() - run.StartedAt
	}
	r.mu.Unlock()
	r.broadcast()
}

func (r *Runner) log(run *Run, message string) {
	r.mu.Lock()
	if run.EndedAt == 0 {
		run.Logs = append(run.Logs, RunLog{At: time.Now().UnixMilli(), Message: message})
	}
	r.mu.Unlock()
	r.broadcast()
}

func (r *Runner) askDialog(ctx context.Context, run *Run, request DialogRequest) (string, bool, error) {
	r.mu.Lock()
	respond := r.dialog
	r.mu.Unlock()
	if respond == nil {
		return "", false, fmt.Errorf("dialog host unavailable")
	}
	request.RequestID = "dlg-" + newRunID()
	answered := make(chan DialogAnswer, 1)
	r.mu.Lock()
	r.pendingDialogs[request.RequestID] = answered
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.pendingDialogs, request.RequestID)
		r.mu.Unlock()
	}()
	r.log(run, "dialog: "+request.Message)
	if _, _, err := respond(ctx, request); err != nil {
		return "", false, err
	}
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
	run.Status, run.EndedAt, run.Error = status, time.Now().UnixMilli(), errText
	run.ElapsedMs = run.EndedAt - run.StartedAt
	if paused := r.paused[run.RunID]; paused != nil {
		close(paused)
		delete(r.paused, run.RunID)
	}
	r.mu.Unlock()
	r.broadcast()
}

func cloneRun(run *Run) *Run {
	copied := *run
	copied.Logs = append([]RunLog(nil), run.Logs...)
	return &copied
}

func clampProgress(current, total int) int {
	if current < 0 {
		return 0
	}
	if total > 0 && current > total {
		return total
	}
	return current
}

func sliceRows(text string, startRow, endRow int) string {
	lines := strings.Split(text, "\n")
	if startRow < 0 {
		startRow = 0
	}
	if endRow >= len(lines) {
		endRow = len(lines) - 1
	}
	if startRow > endRow {
		return ""
	}
	return strings.Join(lines[startRow:endRow+1], "\n")
}

func newRunID() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return "run-" + hex.EncodeToString(buf[:])
}
