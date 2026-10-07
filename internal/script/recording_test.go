package script

import (
	"strings"
	"testing"
	"time"
)

func TestStepsToJavaScriptSensitiveAndWait(t *testing.T) {
	code := StepsToJavaScript([]Step{
		{Type: "send", Value: "secret", Sensitive: true},
		{Type: "waitForPrompt", TimeoutMs: 30000},
	}, "2026-06-27")
	if !strings.Contains(code, `const sensitiveValue0 = await nct.dialog.prompt("Enter sensitive value", "", { sensitive: true });`) {
		t.Fatalf("missing sensitive prompt:\n%s", code)
	}
	if !strings.Contains(code, `await nct.screen.sendLine(sensitiveValue0, { sensitive: true });`) {
		t.Fatalf("missing sensitive send:\n%s", code)
	}
}

func TestStepsToJavaScriptSendAndWaitForText(t *testing.T) {
	code := StepsToJavaScript([]Step{
		{Type: "waitForPrompt", TimeoutMs: 30000},
		{Type: "send", Value: "ls -la"},
		{Type: "waitFor", Value: "DONE", TimeoutMs: 5000},
		{Type: "waitForPrompt", TimeoutMs: 30000},
	}, "2026-06-27")
	if !strings.Contains(code, `sendLine("ls -la")`) {
		t.Fatalf("missing sendLine:\n%s", code)
	}
	if !strings.Contains(code, `waitForText("DONE", 5000)`) {
		t.Fatalf("missing waitForText:\n%s", code)
	}
	if !strings.Contains(code, `waitForPrompt(30000)`) {
		t.Fatalf("missing waitForPrompt:\n%s", code)
	}
	if strings.Contains(code, `waitFor("DONE"`) {
		t.Fatalf("legacy waitFor helper leaked:\n%s", code)
	}
}

func TestRecorderStartStopAndSleepGap(t *testing.T) {
	rec := NewRecorder()
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	rec.now = func() time.Time { return now }
	if err := rec.Start("s1"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(1500 * time.Millisecond)
	result := rec.Append("s1", Step{Type: "send", Value: "ls"})
	if !result.OK {
		t.Fatalf("append: %+v", result)
	}
	stopped := rec.Stop("s1")
	if len(stopped.Steps) != 2 {
		t.Fatalf("steps = %#v", stopped.Steps)
	}
	if stopped.Steps[0].Type != "sleep" {
		t.Fatalf("expected sleep gap, got %#v", stopped.Steps[0])
	}
	if !strings.Contains(stopped.Code, "session.sleep(1500)") {
		t.Fatalf("code missing sleep:\n%s", stopped.Code)
	}
}

func TestRecorderRejectsEmptySessionAndUnknownAppend(t *testing.T) {
	rec := NewRecorder()
	if err := rec.Start(" "); err == nil {
		t.Fatal("empty session must fail")
	}
	result := rec.Append("missing", Step{Type: "send", Value: "x"})
	if result.OK || result.Error != "Recording not started" {
		t.Fatalf("unexpected %+v", result)
	}
}

func TestRecorderStopsAtStepLimit(t *testing.T) {
	rec := NewRecorder()
	if err := rec.Start("s1"); err != nil {
		t.Fatal(err)
	}
	rec.mu.Lock()
	rec.recordings["s1"].steps = make([]Step, MaxRecordingSteps)
	rec.mu.Unlock()
	result := rec.Append("s1", Step{Type: "send", Value: "x"})
	if !result.Stopped || result.Reason != "limit" || result.Error != LimitError {
		t.Fatalf("limit result %+v", result)
	}
	if rec.Stop("s1").Code != "" {
		t.Fatal("limit must release the recording")
	}
}
