package main

import "testing"

func TestScriptServiceRecordsAndStops(t *testing.T) {
	service := newScriptService()
	started := service.StartRecording("s1")
	if !started.OK {
		t.Fatalf("start: %+v", started)
	}
	appended := service.AppendRecordingStep("s1", ScriptStep{Type: "send", Value: "ls"})
	if !appended.OK {
		t.Fatalf("append: %+v", appended)
	}
	stopped := service.StopRecording("s1")
	if len(stopped.Steps) != 1 || stopped.Code == "" {
		t.Fatalf("stop: %+v", stopped)
	}
}
