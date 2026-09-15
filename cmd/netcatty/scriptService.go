package main

import "github.com/binaricat/netcatty/internal/script"

// ScriptService is the Wails facade for terminal script recording.
// Execution (Node worker) stays unimplemented until a Go runner exists.
type ScriptService struct {
	recorder *script.Recorder
}

func newScriptService() *ScriptService {
	return &ScriptService{recorder: script.NewRecorder()}
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
