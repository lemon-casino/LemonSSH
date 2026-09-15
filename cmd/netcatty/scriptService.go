package main

import "github.com/binaricat/netcatty/internal/script"

// ScriptService is the Wails facade for terminal script recording and
// recorded-script replay. Arbitrary JS (dialogs, Node worker APIs) stays
// unimplemented until a non-Node runner exists.
type ScriptService struct {
	recorder *script.Recorder
	runner   *script.Runner
}

func newScriptService() *ScriptService {
	return &ScriptService{
		recorder: script.NewRecorder(),
		runner:   script.NewRunner(nil),
	}
}

func (s *ScriptService) setWriter(write script.SessionWriter) {
	s.runner.SetWriter(write)
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
	RunID       string `json:"runId,omitempty"`
	ScriptID    string `json:"scriptId,omitempty"`
	ScriptLabel string `json:"scriptLabel,omitempty"`
	SessionID   string `json:"sessionId"`
	Content     string `json:"content"`
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
		RunID:       request.RunID,
		ScriptID:    request.ScriptID,
		ScriptLabel: request.ScriptLabel,
		SessionID:   request.SessionID,
		Content:     request.Content,
	})
	if err != nil {
		return ScriptRunResult{Error: err.Error()}
	}
	return ScriptRunResult{OK: true, RunID: run.RunID, RunIDs: []string{run.RunID}, Run: run}
}

func (s *ScriptService) Stop(runID string) ScriptOKResult {
	return ScriptOKResult{OK: s.runner.Stop(runID)}
}

func (s *ScriptService) GetRuns(sessionID string) []script.Run {
	return s.runner.List(sessionID)
}
