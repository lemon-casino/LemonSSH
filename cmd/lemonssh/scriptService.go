package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lemon-casino/lemonssh/internal/script"
)

// ScriptService is the Wails facade for terminal script recording and
// JavaScript execution. Dialog answers stay on the existing host event.
type ScriptService struct {
	recorder *script.Recorder
	runner   *script.Runner
	emit     func(name string, payload any)
	mu       sync.Mutex
	screens  map[string]chan script.ScreenSnapshot
}

func newScriptService() *ScriptService {
	return &ScriptService{
		recorder: script.NewRecorder(),
		runner:   script.NewRunner(nil),
		screens:  make(map[string]chan script.ScreenSnapshot),
	}
}

func (s *ScriptService) setWriter(write script.SessionWriter) {
	s.runner.SetWriter(write)
}

func (s *ScriptService) setSessionCloser(closer script.SessionCloser) {
	s.runner.SetSessionCloser(closer)
}

func (s *ScriptService) setSessionLog(start script.SessionLogStarter, stop script.SessionLogStopper) {
	s.runner.SetSessionLog(start, stop)
}

func (s *ScriptService) setRunsListener(listener func(runs []script.Run)) {
	s.runner.SetRunsListener(listener)
}

func (s *ScriptService) broadcastRuns(runs []script.Run) {
	if s.emit == nil {
		return
	}
	s.emit("lemonssh:script:runs-updated", map[string]any{"runs": runs})
}

func (s *ScriptService) setDialogEmitter(emit func(name string, payload any)) {
	s.emit = emit
	s.runner.SetSessionSnapshot(nil, version)
	s.runner.SetScreenSnapshot(s.requestScreenSnapshot)
	s.runner.SetDialogResponder(func(ctx context.Context, request script.DialogRequest) (string, bool, error) {
		if emit == nil {
			return "", false, fmt.Errorf("dialog host unavailable")
		}
		emit("lemonssh:script:dialog-request", request)
		return "", false, nil
	})
}

// ResolveDialog forwards the renderer's answer to the waiting run.
func (s *ScriptService) ResolveDialog(requestID string, value string, cancelled bool) bool {
	return s.runner.ResolveDialog(requestID, value, cancelled)
}

func (s *ScriptService) requestScreenSnapshot(ctx context.Context, sessionID string) (script.ScreenSnapshot, error) {
	if s.emit == nil {
		return script.ScreenSnapshot{}, fmt.Errorf("screen snapshot host unavailable")
	}
	id := fmt.Sprintf("screen-%d", time.Now().UnixNano())
	answer := make(chan script.ScreenSnapshot, 1)
	s.mu.Lock()
	s.screens[id] = answer
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.screens, id); s.mu.Unlock() }()
	s.emit("lemonssh:script:screen-snapshot-request", map[string]any{"requestId": id, "sessionId": sessionID})
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case result := <-answer:
		return result, nil
	case <-ctx.Done():
		return script.ScreenSnapshot{}, ctx.Err()
	case <-timer.C:
		return script.ScreenSnapshot{}, fmt.Errorf("screen snapshot timed out")
	}
}

func (s *ScriptService) ResolveScreenSnapshot(requestID string, snapshot script.ScreenSnapshot) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	answer := s.screens[requestID]
	if answer == nil {
		return false
	}
	if snapshot.Rows <= 0 || snapshot.Cols <= 0 || len(snapshot.Lines) > 10000 {
		return false
	}
	delete(s.screens, requestID)
	answer <- snapshot
	return true
}

func (s *ScriptService) ObserveOutput(sessionID string, data []byte) {
	s.runner.ObserveOutput(sessionID, data)
}

type ScriptStep = script.Step

type ScriptRecordingStartResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type ScriptRecordingStopResult struct {
	Steps []ScriptStep `json:"steps"`
	Code  string       `json:"code"`
}

type ScriptRunRequest struct {
	RunID          string                  `json:"runId,omitempty"`
	ScriptID       string                  `json:"scriptId,omitempty"`
	ScriptLabel    string                  `json:"scriptLabel,omitempty"`
	SessionID      string                  `json:"sessionId"`
	Content        string                  `json:"content"`
	PermissionMode string                  `json:"permissionMode,omitempty"`
	SessionMeta    *script.SessionSnapshot `json:"sessionMeta,omitempty"`
}

type ScriptRunResult struct {
	OK     bool        `json:"ok"`
	Error  string      `json:"error,omitempty"`
	RunID  string      `json:"runId,omitempty"`
	RunIDs []string    `json:"runIds,omitempty"`
	Run    *script.Run `json:"run,omitempty"`
}

type ScriptOKResult struct {
	OK bool `json:"ok"`
}

func (s *ScriptService) StartRecording(sessionID string) ScriptRecordingStartResult {
	if err := s.recorder.Start(sessionID); err != nil {
		return ScriptRecordingStartResult{Error: err.Error()}
	}
	return ScriptRecordingStartResult{OK: true}
}

func (s *ScriptService) StopRecording(sessionID string) ScriptRecordingStopResult {
	stopped := s.recorder.Stop(sessionID)
	if stopped.Steps == nil {
		stopped.Steps = []ScriptStep{}
	}
	return ScriptRecordingStopResult{Steps: stopped.Steps, Code: stopped.Code}
}

func (s *ScriptService) AppendRecordingStep(sessionID string, step ScriptStep) script.AppendResult {
	return s.recorder.Append(sessionID, step)
}

func (s *ScriptService) ReleaseSession(sessionID string) {
	s.recorder.Release(sessionID)
}

func (s *ScriptService) Run(request ScriptRunRequest) ScriptRunResult {
	run, err := s.runner.Start(script.StartRunRequest{
		RunID:          request.RunID,
		ScriptID:       request.ScriptID,
		ScriptLabel:    request.ScriptLabel,
		SessionID:      request.SessionID,
		Content:        request.Content,
		PermissionMode: request.PermissionMode,
		SessionMeta:    request.SessionMeta,
	})
	if err != nil {
		return ScriptRunResult{Error: err.Error()}
	}
	return ScriptRunResult{OK: true, RunID: run.RunID, RunIDs: []string{run.RunID}, Run: run}
}

func (s *ScriptService) Stop(runID string) ScriptOKResult {
	return ScriptOKResult{OK: s.runner.Stop(runID)}
}

func (s *ScriptService) Pause(runID string) ScriptOKResult {
	return ScriptOKResult{OK: s.runner.Pause(runID)}
}

func (s *ScriptService) Resume(runID string) ScriptOKResult {
	return ScriptOKResult{OK: s.runner.Resume(runID)}
}

func (s *ScriptService) GetRuns(sessionID string) []script.Run {
	return s.runner.List(sessionID)
}
