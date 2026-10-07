package script

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func awaitScriptRun(t *testing.T, r *Runner, id string) Run {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, run := range r.List("") {
			if run.RunID == id && run.EndedAt != 0 {
				return run
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("script %s did not finish: %+v", id, r.List(""))
	return Run{}
}

func TestJavaScriptLoopsFunctionsAndComputedArguments(t *testing.T) {
	var writes strings.Builder
	r := NewRunner(func(_ string, data []byte) error { writes.Write(data); return nil })
	run, err := r.Start(StartRunRequest{SessionID: "s", Content: `
async function main() {
  const items = [1, 2, 3].map(x => x * 2);
  for (const item of items) {
    if (item > 2) await nct.screen.sendLine("echo " + item);
  }
  await Promise.all([nct.sleep(1), nct.session.sleep(2)]);
  nct.log(JSON.stringify({ok: true, count: items.length}));
}`})
	if err != nil {
		t.Fatal(err)
	}
	result := awaitScriptRun(t, r, run.RunID)
	if result.StepIndex != 5 {
		t.Fatalf("operation count = %d, want 5", result.StepIndex)
	}
	if result.Status != "completed" || writes.String() != "echo 4\recho 6\r" {
		t.Fatalf("%+v %q", result, writes.String())
	}
}

func TestJavaScriptDialogValuesRetainTypes(t *testing.T) {
	r := NewRunner(func(string, []byte) error { return nil })
	r.SetDialogResponder(func(_ context.Context, req DialogRequest) (string, bool, error) {
		if req.Type == "confirm" {
			r.ResolveDialog(req.RequestID, "false", false)
		} else {
			r.ResolveDialog(req.RequestID, `{"value":false}`, false)
		}
		return "", false, nil
	})
	run, err := r.Start(StartRunRequest{SessionID: "s", Content: `
const ok = await nct.dialog.confirm("continue?");
const checked = await nct.dialog.checkbox("restart?", false);
if (ok !== false || checked !== false) throw new Error("lost boolean type");
nct.log("correct");`})
	if err != nil {
		t.Fatal(err)
	}
	if result := awaitScriptRun(t, r, run.RunID); result.Status != "completed" {
		t.Fatalf("%+v", result)
	}
}

func TestJavaScriptWaitResultsAndConsumedCursor(t *testing.T) {
	r := NewRunner(func(string, []byte) error { return nil })
	r.ObserveOutput("s", []byte("你好 build OK\r\ndeploy ok\r\n"))
	run, err := r.Start(StartRunRequest{SessionID: "s", Content: `
const match = await nct.screen.waitForRegex(/(?<=build )ok/i, 100);
if (match !== "OK") throw new Error("missing regex result");
const index = await nct.screen.waitForAny(["failed", /deploy ok/], 100);
if (index !== 1) throw new Error("wrong pattern index");
try { await nct.screen.waitForText("build OK", 5); throw new Error("reused old output"); }
catch (error) { if (!error.message.includes("timed out")) throw error; }`})
	if err != nil {
		t.Fatal(err)
	}
	if result := awaitScriptRun(t, r, run.RunID); result.Status != "completed" {
		t.Fatalf("%+v", result)
	}
}

func TestJavaScriptObserverCannotWriteViaComputedProperty(t *testing.T) {
	writes := 0
	r := NewRunner(func(string, []byte) error { writes++; return nil })
	run, err := r.Start(StartRunRequest{SessionID: "s", PermissionMode: "observer", Content: `await nct.screen["send" + "Line"]("blocked");`})
	if err != nil {
		t.Fatal(err)
	}
	result := awaitScriptRun(t, r, run.RunID)
	if result.Status != "failed" || !strings.Contains(result.Error, "Observer mode") || writes != 0 {
		t.Fatalf("%+v writes=%d", result, writes)
	}
}

func TestJavaScriptCanStopCPUAndAsyncLoops(t *testing.T) {
	for _, source := range []string{`while (true) {}`, `while (true) { await nct.sleep(10000); }`} {
		t.Run(source, func(t *testing.T) {
			r := NewRunner(nil)
			run, err := r.Start(StartRunRequest{SessionID: "s", Content: source})
			if err != nil {
				t.Fatal(err)
			}
			time.Sleep(10 * time.Millisecond)
			if !r.Stop(run.RunID) {
				t.Fatal("stop failed")
			}
			result := awaitScriptRun(t, r, run.RunID)
			if result.Status != "failed" || result.Error != "Stopped by user" {
				t.Fatalf("%+v", result)
			}
			deadline := time.Now().Add(time.Second)
			for {
				r.mu.Lock()
				active := len(r.cancels)
				r.mu.Unlock()
				if active == 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("executor was not interrupted")
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
}

func TestJavaScriptSynchronousTimeoutAndIsolation(t *testing.T) {
	for _, source := range []string{`while (true) {}`, `eval("1")`, `Function("return 1")()`, `nct.log.constructor("return process")()`} {
		r := NewRunner(nil)
		run, err := r.Start(StartRunRequest{SessionID: "s", Content: source})
		if err != nil {
			t.Fatal(err)
		}
		if result := awaitScriptRun(t, r, run.RunID); result.Status != "failed" {
			t.Fatalf("%s: %+v", source, result)
		}
	}
}

func TestJavaScriptSessionMetadataAndDuplicateRunID(t *testing.T) {
	r := NewRunner(nil)
	run, err := r.Start(StartRunRequest{RunID: "known", SessionID: "s", SessionMeta: &SessionSnapshot{Connected: true, Name: "Prod", Hostname: "host", Username: "user"}, Content: `
if (nct.session.hostname !== "host" || nct.session.name !== "Prod" || nct.screen.rows !== 24) throw new Error("missing metadata");
await nct.sleep(5);`})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Start(StartRunRequest{RunID: "known", SessionID: "s", Content: `nct.log("no");`}); err == nil {
		t.Fatal("duplicate run allowed")
	}
	if result := awaitScriptRun(t, r, run.RunID); result.Status != "completed" {
		t.Fatalf("%+v", result)
	}
}

func TestJavaScriptScreenSnapshotAndTimeoutRecovery(t *testing.T) {
	r := NewRunner(nil)
	r.SetScreenSnapshot(func(context.Context, string) (ScreenSnapshot, error) {
		return ScreenSnapshot{Rows: 40, Cols: 120, CurrentRow: 7, Lines: []string{"one", "two", "three"}}, nil
	})
	r.SetDialogResponder(func(_ context.Context, req DialogRequest) (string, bool, error) {
		if req.Type != "waitForTimeout" || req.Pattern != "missing" {
			t.Fatalf("unexpected dialog: %+v", req)
		}
		r.ResolveDialog(req.RequestID, "skip", false)
		return "", false, nil
	})
	run, err := r.Start(StartRunRequest{SessionID: "s", Content: `
const text = await nct.screen.getText(1, 2);
if (text !== "two\nthree" || nct.screen.rows !== 40 || nct.screen.currentRow !== 7) throw new Error("screen mismatch");
if (await nct.screen.waitForText("missing", 1) !== "") throw new Error("skip mismatch");`})
	if err != nil {
		t.Fatal(err)
	}
	if result := awaitScriptRun(t, r, run.RunID); result.Status != "completed" {
		t.Fatalf("%+v", result)
	}
}

func TestJavaScriptRepeatedDialogAnswerDoesNotBlock(t *testing.T) {
	r := NewRunner(nil)
	var wg sync.WaitGroup
	r.SetDialogResponder(func(_ context.Context, req DialogRequest) (string, bool, error) {
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); r.ResolveDialog(req.RequestID, "ok", false) }()
		}
		return "", false, nil
	})
	run, err := r.Start(StartRunRequest{SessionID: "s", Content: `await nct.dialog.prompt("input", "initial", {});`})
	if err != nil {
		t.Fatal(err)
	}
	if result := awaitScriptRun(t, r, run.RunID); result.Status != "completed" {
		t.Fatalf("%+v", result)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("duplicate answers blocked")
	}
}
