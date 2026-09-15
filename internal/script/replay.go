package script

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ReplayOp is one recorded-script action the Go runner can execute.
type ReplayOp struct {
	Kind      string
	Value     string
	Var       string
	Label     string
	Current   int
	Total     int
	Timeout   time.Duration
	Sensitive bool
}

var (
	sleepCall      = regexp.MustCompile(`(?m)^\s*await nct\.session\.sleep\((\d+)\);\s*$`)
	sendLineCall   = regexp.MustCompile(`(?m)^\s*await nct\.screen\.sendLine\((.*)\);\s*$`)
	sendLineSens   = regexp.MustCompile(`(?m)^\s*await nct\.screen\.sendLine\((.*),\s*\{\s*sensitive:\s*true\s*\}\);\s*$`)
	waitPromptCall = regexp.MustCompile(`(?m)^\s*await nct\.screen\.waitForPrompt\((\d+)\);\s*$`)
	waitTextCall   = regexp.MustCompile(`(?m)^\s*await nct\.screen\.waitForText\((.*),\s*(\d+)\);\s*$`)
	promptAssign   = regexp.MustCompile(`(?m)^\s*const ([A-Za-z_$][\w$]*) = await nct\.dialog\.prompt\((.*)\);\s*$`)
	logCall        = regexp.MustCompile(`(?m)^(?:nct\.log|(?:await )?nct\.dialog\.alert)\((.*)\);\s*$`)
	progressCall   = regexp.MustCompile(`(?m)^(?:await )?nct\.progress\.(start|set|step|done)\((.*)\);\s*$`)
	disconnectCall = regexp.MustCompile(`(?m)^(?:await )?nct\.session\.disconnect\(\);\s*$`)
	unsupportedAPI = regexp.MustCompile(`nct\.dialog\.(confirm|form|select|radio|checkbox)|nct\.screen\.send\(|nct\.screen\.waitForRegex|nct\.screen\.waitForAny|nct\.screen\.getText|nct\.screen\.clear|nct\.session\.startLog|nct\.session\.stopLog`)
)

// ParseRecordedScript turns recorder-generated JS into replay ops.
// Dialog scripts other than nct.dialog.prompt, log, and disconnect calls are
// rejected instead of pretending the Node worker ran them.
func ParseRecordedScript(source string) ([]ReplayOp, error) {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return nil, fmt.Errorf("Script content is empty")
	}
	if unsupportedAPI.MatchString(trimmed) {
		return nil, fmt.Errorf("this script uses APIs that are not migrated to the Wails runner yet")
	}
	var ops []ReplayOp
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") || line == "async function main() {" || line == "}" || line == "await main();" {
			continue
		}
		if match := sleepCall.FindStringSubmatch(line); match != nil {
			ms, _ := strconv.Atoi(match[1])
			ops = append(ops, ReplayOp{Kind: "sleep", Timeout: time.Duration(ms) * time.Millisecond})
			continue
		}
		if match := promptAssign.FindStringSubmatch(line); match != nil {
			args := splitJSArgs(match[2])
			if len(args) < 1 {
				return nil, fmt.Errorf("dialog.prompt needs a message: %s", line)
			}
			message, err := unquoteJS(args[0])
			if err != nil {
				return nil, err
			}
			ops = append(ops, ReplayOp{Kind: "prompt", Var: match[1], Value: message, Sensitive: strings.Contains(match[2], "sensitive")})
			continue
		}
		if match := sendLineSens.FindStringSubmatch(line); match != nil {
			op, err := sendLineArg(match[1])
			if err != nil {
				return nil, err
			}
			op.Sensitive = true
			ops = append(ops, op)
			continue
		}
		if match := sendLineCall.FindStringSubmatch(line); match != nil {
			op, err := sendLineArg(match[1])
			if err != nil {
				return nil, err
			}
			ops = append(ops, op)
			continue
		}
		if match := logCall.FindStringSubmatch(line); match != nil {
			value, err := unquoteJS(match[1])
			if err != nil {
				return nil, err
			}
			kind := "log"
			if strings.HasPrefix(strings.TrimSpace(line), "await nct.dialog.alert") {
				kind = "alert"
			}
			ops = append(ops, ReplayOp{Kind: kind, Value: value})
			continue
		}
		if match := disconnectCall.FindStringSubmatch(line); match != nil {
			ops = append(ops, ReplayOp{Kind: "disconnect"})
			continue
		}
		if match := progressCall.FindStringSubmatch(line); match != nil {
			op, err := progressOp(match[1], match[2])
			if err != nil {
				return nil, err
			}
			ops = append(ops, op)
			continue
		}
		if match := waitPromptCall.FindStringSubmatch(line); match != nil {
			ms, _ := strconv.Atoi(match[1])
			ops = append(ops, ReplayOp{Kind: "waitForPrompt", Timeout: time.Duration(ms) * time.Millisecond})
			continue
		}
		if match := waitTextCall.FindStringSubmatch(line); match != nil {
			value, err := unquoteJS(match[1])
			if err != nil {
				return nil, err
			}
			ms, _ := strconv.Atoi(match[2])
			ops = append(ops, ReplayOp{Kind: "waitForText", Value: value, Timeout: time.Duration(ms) * time.Millisecond})
			continue
		}
		return nil, fmt.Errorf("unsupported script line: %s", line)
	}
	if len(ops) == 0 {
		return nil, fmt.Errorf("Script content is empty")
	}
	return ops, nil
}

// sendLineArg accepts either a string literal or a variable reference
// (sensitive recorded steps read their value from a dialog prompt result).
func sendLineArg(raw string) (ReplayOp, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ReplayOp{}, fmt.Errorf("sendLine needs a value")
	}
	if raw[0] == '"' {
		value, err := unquoteJS(raw)
		if err != nil {
			return ReplayOp{}, err
		}
		return ReplayOp{Kind: "sendLine", Value: value}, nil
	}
	if identifier.MatchString(raw) {
		return ReplayOp{Kind: "sendLine", Var: raw}, nil
	}
	return ReplayOp{}, fmt.Errorf("unsupported sendLine argument: %s", raw)
}

var identifier = regexp.MustCompile(`^[A-Za-z_$][\w$]*$`)

// splitJSArgs splits comma-separated arguments at the top nesting level only.
func splitJSArgs(raw string) []string {
	var args []string
	depth := 0
	inString := false
	var quote byte
	start := 0
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			if ch == '\\' {
				i++
				continue
			}
			if ch == quote {
				inString = false
			}
			continue
		}
		switch ch {
		case '"', '\'':
			inString = true
			quote = ch
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(raw[start:i]))
				start = i + 1
			}
		}
	}
	if rest := strings.TrimSpace(raw[start:]); rest != "" {
		args = append(args, rest)
	}
	return args
}

// progressOp builds a progress op from literal args; computed values are
// rejected as unsupported lines rather than guessed.
func progressOp(method, rawArgs string) (ReplayOp, error) {
	args := splitJSArgs(rawArgs)
	switch method {
	case "start":
		if len(args) == 0 {
			return ReplayOp{Kind: "progressStart"}, nil
		}
		label, err := unquoteJS(args[0])
		if err != nil {
			return ReplayOp{}, err
		}
		total := 1
		if len(args) > 1 {
			total = intArg(args[1])
		}
		return ReplayOp{Kind: "progressStart", Value: label, Total: total}, nil
	case "set":
		if len(args) == 0 {
			return ReplayOp{}, fmt.Errorf("progress.set needs a current value")
		}
		current := intArg(args[0])
		op := ReplayOp{Kind: "progressSet", Current: current}
		if len(args) > 1 {
			label, err := unquoteJS(args[1])
			if err != nil {
				return ReplayOp{}, err
			}
			op.Label = label
		}
		return op, nil
	case "step":
		op := ReplayOp{Kind: "progressStep"}
		if len(args) > 0 {
			label, err := unquoteJS(args[0])
			if err != nil {
				return ReplayOp{}, err
			}
			op.Label = label
		}
		return op, nil
	case "done":
		return ReplayOp{Kind: "progressDone"}, nil
	}
	return ReplayOp{}, fmt.Errorf("unsupported progress method: %s", method)
}

func intArg(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0
	}
	return value
}

func unquoteJS(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') {
		if raw[0] == '\'' {
			raw = `"` + strings.ReplaceAll(raw[1:len(raw)-1], `"`, `\"`) + `"`
		}
		value, err := strconv.Unquote(raw)
		if err != nil {
			return "", fmt.Errorf("invalid script string: %s", raw)
		}
		return value, nil
	}
	return "", fmt.Errorf("script argument must be a string literal: %s", raw)
}
