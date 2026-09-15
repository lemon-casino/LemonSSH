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
	Timeout   time.Duration
	Sensitive bool
}

var (
	sleepCall      = regexp.MustCompile(`(?m)^\s*await nct\.session\.sleep\((\d+)\);\s*$`)
	sendLineCall   = regexp.MustCompile(`(?m)^\s*await nct\.screen\.sendLine\((.*)\);\s*$`)
	sendLineSens   = regexp.MustCompile(`(?m)^\s*await nct\.screen\.sendLine\((.*),\s*\{\s*sensitive:\s*true\s*\}\);\s*$`)
	waitPromptCall = regexp.MustCompile(`(?m)^\s*await nct\.screen\.waitForPrompt\((\d+)\);\s*$`)
	waitTextCall   = regexp.MustCompile(`(?m)^\s*await nct\.screen\.waitForText\((.*),\s*(\d+)\);\s*$`)
	unsupportedAPI = regexp.MustCompile(`nct\.dialog|nct\.progress|nct\.screen\.send\(|nct\.screen\.waitForRegex|nct\.screen\.waitForAny|nct\.screen\.getText|nct\.screen\.clear|nct\.session\.startLog|nct\.session\.stopLog|nct\.session\.disconnect`)
)

// ParseRecordedScript turns recorder-generated JS into replay ops.
// Scripts that use dialog/log/disconnect APIs are rejected instead of
// pretending the Node worker ran them.
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
		if match := sendLineSens.FindStringSubmatch(line); match != nil {
			value, err := unquoteJS(match[1])
			if err != nil {
				return nil, err
			}
			ops = append(ops, ReplayOp{Kind: "sendLine", Value: value, Sensitive: true})
			continue
		}
		if match := sendLineCall.FindStringSubmatch(line); match != nil {
			value, err := unquoteJS(match[1])
			if err != nil {
				return nil, err
			}
			ops = append(ops, ReplayOp{Kind: "sendLine", Value: value})
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
