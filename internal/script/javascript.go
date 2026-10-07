package script

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
)

const (
	scriptSyncTimeout = time.Second
	maxScriptSource   = 1 << 20
	maxScriptPending  = 128
	maxScriptLogs     = 512
)

var (
	mainFunction = regexp.MustCompile(`\b(?:async\s+)?function\s+main\s*\(`)
	mainCall     = regexp.MustCompile(`(?s)\s*;?\s*(?:await\s+)?main\s*\(\s*\)\s*;?\s*$`)
	asyncIIFE    = regexp.MustCompile(`^\(\s*async\s*(?:\(\s*\)\s*=>|function\s*\(\s*\))`)
)

func compileScript(source string) (*goja.Program, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, fmt.Errorf("Script content is empty")
	}
	if len(source) > maxScriptSource {
		return nil, fmt.Errorf("script exceeds the 1 MiB source limit")
	}
	if !asyncIIFE.MatchString(source) {
		if mainFunction.MatchString(source) {
			source = mainCall.ReplaceAllString(source, "") + "\nawait main();"
		}
		source = "(async () => {\n" + source + "\n})();"
	}
	return goja.Compile("lemonssh-script.js", source, true)
}

type scriptCompletion struct {
	resolve func(any) error
	reject  func(any) error
	value   any
	err     error
}

type scriptRuntime struct {
	vm          *goja.Runtime
	runner      *Runner
	run         *Run
	ctx         context.Context
	completions chan scriptCompletion
	pending     int
	logs        int
	operations  int
	observer    bool
	meta        SessionSnapshot
	screenState *ScreenSnapshot
	disconnect  atomic.Bool
	busySince   atomic.Int64
	unhandled   map[*goja.Promise]bool
}

func (r *Runner) executeJavaScript(ctx context.Context, cancel context.CancelFunc, run *Run, program *goja.Program, permissionMode string, meta SessionSnapshot) {
	defer cancel()
	defer func() {
		r.mu.Lock()
		delete(r.cancels, run.RunID)
		r.mu.Unlock()
	}()
	x := &scriptRuntime{
		vm: goja.New(), runner: r, run: run, ctx: ctx,
		completions: make(chan scriptCompletion, maxScriptPending),
		observer:    permissionMode == "observer", unhandled: make(map[*goja.Promise]bool),
		meta: meta,
	}
	x.vm.SetMaxCallStackSize(1024)
	x.vm.SetPromiseRejectionTracker(func(p *goja.Promise, operation goja.PromiseRejectionOperation) {
		if operation == goja.PromiseRejectionReject {
			x.unhandled[p] = true
		} else {
			delete(x.unhandled, p)
		}
	})
	done, watchdogDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(watchdogDone)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				x.vm.Interrupt(ctx.Err())
				return
			case <-ticker.C:
				if since := x.busySince.Load(); since != 0 && time.Since(time.Unix(0, since)) >= scriptSyncTimeout {
					x.vm.Interrupt(errors.New("script exceeded the synchronous execution time limit"))
					return
				}
			}
		}
	}()
	defer func() { close(done); <-watchdogDone }()
	err := x.execute(program)
	if x.disconnect.Load() && ctx.Err() == nil {
		r.finish(run, "completed", "")
	} else if ctx.Err() != nil {
		r.finish(run, "failed", "Stopped by user")
	} else if err != nil {
		r.finish(run, "failed", err.Error())
	} else {
		r.finish(run, "completed", "")
	}
}

func (x *scriptRuntime) execute(program *goja.Program) error {
	if err := x.installAPI(); err != nil {
		return err
	}
	x.busySince.Store(time.Now().UnixNano())
	value, err := x.vm.RunProgram(program)
	x.busySince.Store(0)
	if err != nil {
		return err
	}
	promise, _ := value.Export().(*goja.Promise)
	for {
		if promise != nil && promise.State() == goja.PromiseStateRejected {
			return scriptValueError(promise.Result())
		}
		if x.disconnect.Load() {
			return nil
		}
		if x.pending == 0 {
			for p := range x.unhandled {
				return scriptValueError(p.Result())
			}
			if promise != nil && promise.State() == goja.PromiseStatePending {
				return fmt.Errorf("script is waiting on a promise without a host operation")
			}
			return nil
		}
		select {
		case <-x.ctx.Done():
			return x.ctx.Err()
		case c := <-x.completions:
			x.pending--
			if text, ok := c.value.(scriptTextResult); ok {
				x.screenState = text.snapshot
				c.value = text.text
			}
			x.busySince.Store(time.Now().UnixNano())
			if c.err != nil {
				err = c.reject(x.vm.NewGoError(c.err))
			} else {
				err = c.resolve(c.value)
			}
			x.busySince.Store(0)
			if err != nil {
				return err
			}
		}
	}
}

func scriptValueError(value goja.Value) error {
	if object, ok := value.(*goja.Object); ok {
		if message := object.Get("message"); message != nil && !goja.IsUndefined(message) {
			return errors.New(message.String())
		}
	}
	return errors.New(value.String())
}

func (x *scriptRuntime) async(work func() (any, error)) goja.Value {
	if x.pending >= maxScriptPending {
		panic(x.vm.NewTypeError("script exceeded the pending host request limit"))
	}
	x.operations++
	if x.operations > 20000 {
		panic(x.vm.NewTypeError("script exceeded the host operation limit"))
	}
	x.runner.step(x.run)
	promise, resolve, reject := x.vm.NewPromise()
	x.pending++
	go func() {
		var value any
		err := x.runner.waitIfPaused(x.ctx, x.run.RunID)
		if err == nil && !x.disconnect.Load() {
			value, err = work()
		}
		select {
		case x.completions <- scriptCompletion{resolve: resolve, reject: reject, value: value, err: err}:
		case <-x.ctx.Done():
		}
	}()
	return x.vm.ToValue(promise)
}

func (x *scriptRuntime) writeAllowed(operation string) error {
	if x.observer {
		return fmt.Errorf("Observer mode: %s is disabled. Switch to Confirm or Auto mode.", operation)
	}
	return x.ctx.Err()
}

func jsValueString(value goja.Value) string {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return ""
	}
	return value.String()
}

func jsDuration(value goja.Value, fallback time.Duration) time.Duration {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return fallback
	}
	number := value.ToFloat()
	if math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
		return 0
	}
	return time.Duration(math.Min(number, float64(24*time.Hour/time.Millisecond))) * time.Millisecond
}

func (x *scriptRuntime) installAPI() error {
	vm := x.vm
	r := x.runner
	r.mu.Lock()
	write, closer, startLog, stopLog, snapshot, version := r.write, r.closer, r.startLog, r.stopLog, r.snapshot, r.version
	r.mu.Unlock()
	if version == "" {
		version = "0.0.0"
	}
	nct, session, screen, dialog, progress := vm.NewObject(), vm.NewObject(), vm.NewObject(), vm.NewObject(), vm.NewObject()
	_ = nct.Set("session", session)
	_ = nct.Set("screen", screen)
	_ = nct.Set("dialog", dialog)
	_ = nct.Set("progress", progress)
	_ = nct.Set("version", version)
	_ = vm.Set("nct", nct)
	_ = session.Set("sleep", func(call goja.FunctionCall) goja.Value {
		d := jsDuration(call.Argument(0), 0)
		return x.async(func() (any, error) { return nil, r.sleep(x.ctx, d) })
	})
	_ = nct.Set("sleep", session.Get("sleep"))
	for _, key := range []string{"connected", "name", "hostname", "username"} {
		_ = session.DefineAccessorProperty(key, vm.ToValue(func(goja.FunctionCall) goja.Value {
			info := x.meta
			if snapshot != nil {
				info = snapshot(x.run.SessionID)
			}
			switch key {
			case "connected":
				return vm.ToValue(info.Connected && !x.disconnect.Load())
			case "name":
				return vm.ToValue(info.Name)
			case "hostname":
				return vm.ToValue(info.Hostname)
			default:
				return vm.ToValue(info.Username)
			}
		}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
	for _, key := range []string{"rows", "cols", "currentRow"} {
		_ = screen.DefineAccessorProperty(key, vm.ToValue(func(goja.FunctionCall) goja.Value {
			info := x.meta
			if snapshot != nil {
				info = snapshot(x.run.SessionID)
			}
			if x.screenState != nil {
				info.Rows, info.Cols = x.screenState.Rows, x.screenState.Cols
				if key == "currentRow" {
					return vm.ToValue(x.screenState.CurrentRow)
				}
			}
			if key == "rows" {
				return vm.ToValue(max(1, info.Rows))
			}
			if key == "cols" {
				return vm.ToValue(max(1, info.Cols))
			}
			return vm.ToValue(strings.Count(r.watch(x.run.SessionID).snapshot(), "\n"))
		}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
	_ = session.Set("startLog", func(call goja.FunctionCall) goja.Value {
		path := jsValueString(call.Argument(0))
		return x.async(func() (any, error) {
			if err := x.writeAllowed("session.startLog"); err != nil {
				return nil, err
			}
			if startLog == nil {
				return nil, errors.New("session log owner unavailable")
			}
			return startLog(x.run.SessionID, path)
		})
	})
	_ = session.Set("stopLog", func(goja.FunctionCall) goja.Value {
		return x.async(func() (any, error) {
			if stopLog == nil {
				return nil, errors.New("session log owner unavailable")
			}
			return nil, stopLog(x.run.SessionID)
		})
	})
	_ = session.Set("disconnect", func(goja.FunctionCall) goja.Value {
		return x.async(func() (any, error) {
			if err := x.writeAllowed("session.disconnect"); err != nil {
				return nil, err
			}
			if closer == nil {
				return nil, errors.New("session closer unavailable")
			}
			err := closer(x.run.SessionID)
			if err == nil {
				x.disconnect.Store(true)
			}
			return nil, err
		})
	})
	for _, method := range []string{"send", "sendLine", "clear"} {
		_ = screen.Set(method, func(call goja.FunctionCall) goja.Value {
			text := jsValueString(call.Argument(0))
			sensitive := false
			if option, ok := call.Argument(1).(*goja.Object); ok {
				if value := option.Get("sensitive"); value != nil {
					sensitive = value.ToBoolean()
				}
			}
			if method == "clear" {
				text = "\x1b[2J\x1b[H"
			}
			return x.async(func() (any, error) {
				if err := x.writeAllowed("screen." + method); err != nil {
					return nil, err
				}
				if write == nil {
					return nil, errors.New("terminal writer unavailable")
				}
				label := text
				if sensitive {
					label = "[sensitive]"
				}
				r.log(x.run, "→ "+label)
				watch := r.watch(x.run.SessionID)
				before := watch.position()
				if text != "" {
					if err := write(x.run.SessionID, []byte(text)); err != nil {
						return nil, err
					}
				}
				if method == "clear" {
					watch.Reset()
				}
				if method == "sendLine" {
					if text != "" {
						if err := r.sleep(x.ctx, 30*time.Millisecond); err != nil {
							return nil, err
						}
					}
					if err := x.ctx.Err(); err != nil {
						return nil, err
					}
					if err := write(x.run.SessionID, []byte("\r")); err != nil {
						return nil, err
					}
					watch.consumeThrough(before)
				}
				return nil, nil
			})
		})
	}
	_ = screen.Set("getText", func(call goja.FunctionCall) goja.Value {
		start, end := 0, -1
		if !goja.IsUndefined(call.Argument(0)) {
			start = int(call.Argument(0).ToInteger())
		}
		if !goja.IsUndefined(call.Argument(1)) {
			end = int(call.Argument(1).ToInteger())
		}
		return x.async(func() (any, error) {
			text := validUTF8Tail(r.watch(x.run.SessionID).snapshot())
			var screen *ScreenSnapshot
			r.mu.Lock()
			read := r.screenSnapshot
			r.mu.Unlock()
			if read != nil {
				if snapshot, err := read(x.ctx, x.run.SessionID); err == nil {
					screen = &snapshot
					if len(snapshot.Lines) > 0 {
						text = strings.Join(snapshot.Lines, "\n")
					}
				}
			}
			if end < 0 {
				end = strings.Count(text, "\n")
			}
			return scriptTextResult{text: sliceRows(text, start, end), snapshot: screen}, nil
		})
	})
	for _, method := range []string{"waitFor", "waitForText", "waitForRegex", "waitForPrompt", "waitForAny"} {
		_ = screen.Set(method, x.waitFunction(method))
	}
	for _, method := range []string{"alert", "confirm", "prompt", "form", "select", "radio", "checkbox"} {
		_ = dialog.Set(method, x.dialogFunction(method))
	}
	for _, method := range []string{"start", "set", "step", "done"} {
		_ = progress.Set(method, func(call goja.FunctionCall) goja.Value {
			x.operations++
			if x.operations > 20000 {
				panic(vm.NewTypeError("script exceeded the host operation limit"))
			}
			label, detail := jsValueString(call.Argument(0)), jsValueString(call.Argument(1))
			current, total := 0, 1
			if method == "set" {
				current = int(call.Argument(0).ToInteger())
			}
			if method == "start" && !goja.IsUndefined(call.Argument(1)) {
				total = max(1, int(call.Argument(1).ToInteger()))
			}
			r.mu.Lock()
			switch method {
			case "start":
				x.run.ProgressMode, x.run.ProgressLabel, x.run.ActivityLabel = "determinate", label, label
				x.run.ProgressTotal, x.run.ProgressCurrent = total, 0
			case "set":
				x.run.ProgressCurrent = clampProgress(current, x.run.ProgressTotal)
				if detail != "" {
					x.run.ActivityLabel = detail
				}
			case "step":
				x.run.ProgressCurrent = clampProgress(x.run.ProgressCurrent+1, x.run.ProgressTotal)
				if label != "" {
					x.run.ActivityLabel = label
				}
			case "done":
				x.run.ProgressCurrent = x.run.ProgressTotal
			}
			r.mu.Unlock()
			r.broadcast()
			return goja.Undefined()
		})
	}
	logger := func(call goja.FunctionCall) goja.Value {
		x.logs++
		if x.logs > maxScriptLogs {
			panic(vm.NewTypeError("script exceeded the log notification limit"))
		}
		x.runner.step(x.run)
		parts := make([]string, 0, len(call.Arguments))
		for _, arg := range call.Arguments {
			parts = append(parts, jsValueString(arg))
		}
		message := strings.Join(parts, " ")
		if len(message) > 16*1024 {
			message = message[:16*1024]
		}
		r.log(x.run, message)
		return goja.Undefined()
	}
	_ = nct.Set("log", logger)
	_ = vm.Set("console", map[string]any{"log": logger})
	_, err := vm.RunString(`
		const disabled = () => { throw new Error("Dynamic code generation is disabled"); };
		for (const ctor of [Function, Object.getPrototypeOf(async function(){}).constructor,
			Object.getPrototypeOf(function*(){}).constructor]) {
			Object.defineProperty(ctor.prototype, "constructor", {value: disabled, configurable: false});
		}
		globalThis.eval = disabled;
		globalThis.Function = disabled;
		globalThis.SharedArrayBuffer = undefined;
	`)
	return err
}
