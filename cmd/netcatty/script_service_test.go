package main

import (
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/script"
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

func TestScriptServiceDialogPromptResolvesSensitiveSend(t *testing.T) {
	service := newScriptService()
	_ = service.setWriter // writer presence check
	wrote := make(chan string, 4)
	service.setWriter(func(sessionID string, data []byte) error {
		wrote <- string(data)
		return nil
	})
	var emitted []any
	service.setDialogEmitter(func(name string, payload any) {
		if name != "netcatty:script:dialog-request" {
			t.Fatalf("event %q", name)
		}
		emitted = append(emitted, payload)
		go func() {
			request := payload.(script.DialogRequest)
			service.ResolveDialog(request.RequestID, "sekret", false)
		}()
	})
	result := service.Run(ScriptRunRequest{
		SessionID: "s1",
		Content: `async function main() {
  const sensitiveValue0 = await nct.dialog.prompt("Enter sensitive value", "", { sensitive: true });
  await nct.screen.sendLine(sensitiveValue0, { sensitive: true });
}
await main();`,
	})
	if !result.OK || result.RunID == "" {
		t.Fatalf("run: %+v", result)
	}
	select {
	case got := <-wrote:
		if got != "sekret" && got != "\r" {
			t.Fatalf("write %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no write")
	}
	if len(emitted) != 1 {
		t.Fatalf("emitted %d dialog requests", len(emitted))
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
