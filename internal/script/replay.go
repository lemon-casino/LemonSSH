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
	unsupportedAPI = regexp.MustCompile(`nct\.dialog\.(alert|confirm|form|select|radio|checkbox)|nct\.progress|nct\.screen\.send\(|nct\.screen\.waitForRegex|nct\.screen\.waitForAny|nct\.screen\.getText|nct\.screen\.clear|nct\.session\.startLog|nct\.session\.stopLog|nct\.session\.disconnect`)
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

func unquoteJS(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == '"' {
		value, err := strconv.Unquote(raw)
		if err != nil {
			return "", fmt.Errorf("invalid script string: %s", raw)
		}
		return value, nil
	}
	return "", fmt.Errorf("script argument must be a string literal: %s", raw)
}
