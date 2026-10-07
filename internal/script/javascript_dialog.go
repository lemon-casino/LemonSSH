package script

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
)

func (x *scriptRuntime) dialogFunction(method string) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		request := DialogRequest{Type: method, Message: jsValueString(call.Argument(0))}
		var err error
		switch method {
		case "prompt":
			request.DefaultValue = jsValueString(call.Argument(1))
			if options, ok := call.Argument(2).(*goja.Object); ok {
				if sensitive := options.Get("sensitive"); sensitive != nil {
					request.Sensitive = sensitive.ToBoolean()
				}
			}
		case "form":
			var spec map[string]any
			err = exportScriptJSON(call.Argument(0), &spec)
			if err == nil {
				request.Form, err = normalizeDialogForm(spec)
			}
		case "select", "radio", "checkbox":
			var options, defaultValue any
			if method == "checkbox" {
				defaultValue = call.Argument(1).ToBoolean()
			} else {
				err = exportScriptJSON(call.Argument(1), &options)
				if !goja.IsUndefined(call.Argument(2)) {
					defaultValue = call.Argument(2).Export()
				}
			}
			if err == nil {
				request.Form, err = normalizeDialogForm(map[string]any{
					"message": request.Message,
					"fields":  []any{map[string]any{"type": method, "name": "value", "label": request.Message, "options": options, "defaultValue": defaultValue}},
				})
			}
		}
		if err != nil {
			panic(x.vm.NewTypeError(err.Error()))
		}
		if request.Form != nil {
			request.Type, request.Message = "form", request.Form.Message
		}
		return x.async(func() (any, error) {
			value, cancelled, err := x.runner.askDialog(x.ctx, x.run, request)
			if err != nil {
				return nil, err
			}
			if cancelled {
				if method == "confirm" {
					return false, nil
				}
				return nil, errors.New("Dialog cancelled")
			}
			switch method {
			case "alert":
				return nil, nil
			case "confirm":
				return value == "true", nil
			case "form", "select", "radio", "checkbox":
				var values map[string]any
				if err := json.Unmarshal([]byte(value), &values); err != nil {
					return nil, fmt.Errorf("invalid dialog response: %w", err)
				}
				if method == "form" {
					return values, nil
				}
				return values["value"], nil
			default:
				return value, nil
			}
		})
	}
}

func exportScriptJSON(value goja.Value, target any) error {
	if value == nil || goja.IsUndefined(value) {
		return errors.New("missing dialog argument")
	}
	data, err := json.Marshal(value.Export())
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func (x *scriptRuntime) waitFunction(method string) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		var patterns []waitPattern
		var err error
		timeout := jsDuration(call.Argument(1), 30*time.Second)
		if method == "waitForPrompt" {
			timeout = jsDuration(call.Argument(0), 60*time.Second)
			pattern, compileErr := regexWaitPattern(promptEnd.String(), "")
			err = compileErr
			patterns = []waitPattern{pattern}
		} else if method == "waitForAny" {
			array, ok := call.Argument(0).(*goja.Object)
			if !ok || array.ClassName() != "Array" || array.Get("length").ToInteger() == 0 {
				panic(x.vm.NewTypeError("waitForAny requires a non-empty patterns array"))
			}
			if array.Get("length").ToInteger() > 128 {
				panic(x.vm.NewTypeError("too many wait patterns"))
			}
			for i := int64(0); i < array.Get("length").ToInteger(); i++ {
				pattern, compileErr := x.waitPattern(array.Get(fmt.Sprint(i)), false, false)
				if compileErr != nil {
					err = compileErr
					break
				}
				patterns = append(patterns, pattern)
			}
		} else {
			pattern, compileErr := x.waitPattern(call.Argument(0), method == "waitForRegex", method == "waitForText")
			err = compileErr
			patterns = []waitPattern{pattern}
		}
		if err != nil {
			panic(x.vm.NewTypeError(err.Error()))
		}
		label := jsValueString(call.Argument(0))
		if method == "waitForPrompt" {
			label = "shell prompt"
		}
		return x.async(func() (any, error) {
			for {
				text, index, err := x.runner.watch(x.run.SessionID).waitFor(x.ctx, patterns, timeout, method == "waitForPrompt")
				if err == nil {
					if method == "waitForAny" || method == "waitForPrompt" {
						return index, nil
					}
					return text, nil
				}
				if x.ctx.Err() != nil || !strings.Contains(err.Error(), "timed out") {
					return nil, err
				}
				x.runner.mu.Lock()
				hasDialog := x.runner.dialog != nil
				x.runner.mu.Unlock()
				if !hasDialog {
					return nil, err
				}
				answer, cancelled, dialogErr := x.runner.askDialog(x.ctx, x.run, DialogRequest{
					Type: "waitForTimeout", Message: label, Pattern: label, TimeoutMs: int64(timeout / time.Millisecond),
				})
				if dialogErr != nil {
					return nil, dialogErr
				}
				if !cancelled && answer == "retry" {
					continue
				}
				if !cancelled && answer == "skip" {
					if method == "waitForAny" || method == "waitForPrompt" {
						return -1, nil
					}
					return "", nil
				}
				return nil, errors.New("Script stopped by user")
			}
		})
	}
}

func (x *scriptRuntime) waitPattern(value goja.Value, regex, literal bool) (waitPattern, error) {
	if object, ok := value.(*goja.Object); ok && object.ClassName() == "RegExp" && !literal {
		return regexWaitPattern(object.Get("source").String(), object.Get("flags").String())
	}
	text := jsValueString(value)
	if literal {
		return literalWaitPattern(text), nil
	}
	return stringWaitPattern(text, regex)
}
