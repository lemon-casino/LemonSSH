package main

import (
	"testing"
	"time"
)

func TestScriptServiceRunsRecordedScript(t *testing.T) {
	service := newScriptService()
	wrote := make(chan string, 4)
	service.setWriter(func(sessionID string, data []byte) error {
		if sessionID != "s1" {
			t.Fatalf("session %q", sessionID)
		}
		wrote <- string(data)
		return nil
	})
	result := service.Run(ScriptRunRequest{
		SessionID: "s1",
		Content: `async function main() {
  await nct.screen.sendLine("ls");
  await nct.screen.waitForPrompt(1);
}
await main();`,
	})
	if !result.OK || result.RunID == "" {
		t.Fatalf("run: %+v", result)
	}
	select {
	case got := <-wrote:
		if got != "ls" && got != "\r" {
			t.Fatalf("write %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("no write")
	}
}

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
