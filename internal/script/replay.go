package script

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dlclark/regexp2"
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
	Patterns  []string
	Regexes   []*regexp2.Regexp
	RangeArgs bool
	Form      *DialogForm
	Extract   string
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
	startLogCall   = regexp.MustCompile(`(?m)^(?:await )?nct\.session\.startLog\((.*)\);\s*$`)
	stopLogCall    = regexp.MustCompile(`(?m)^(?:await )?nct\.session\.stopLog\(\);\s*$`)
	waitRegexCall  = regexp.MustCompile(`(?m)^\s*await nct\.screen\.waitForRegex\((.*)\);\s*$`)
	waitAnyCall    = regexp.MustCompile(`(?m)^\s*await nct\.screen\.waitForAny\((.*)\);\s*$`)
	sendCall       = regexp.MustCompile(`(?m)^\s*await nct\.screen\.send\((.*)\);\s*$`)
	clearCall      = regexp.MustCompile(`(?m)^\s*await nct\.screen\.clear\(\);\s*$`)
	getTextAssign  = regexp.MustCompile(`(?m)^\s*const ([A-Za-z_$][\w$]*) = await nct\.screen\.getText\((.*)\);\s*$`)
	confirmAssign  = regexp.MustCompile(`(?m)^\s*const ([A-Za-z_$][\w$]*) = await nct\.dialog\.confirm\((.*)\);\s*$`)
	formAssign     = regexp.MustCompile(`(?m)^\s*const ([A-Za-z_$][\w$]*) = await nct\.dialog\.form\((.*)\)\s*;\s*$`)
	selectAssign   = regexp.MustCompile(`(?m)^\s*const ([A-Za-z_$][\w$]*) = await nct\.dialog\.select\((.*)\)\s*;\s*$`)
	radioAssign    = regexp.MustCompile(`(?m)^\s*const ([A-Za-z_$][\w$]*) = await nct\.dialog\.radio\((.*)\)\s*;\s*$`)
	checkboxAssign = regexp.MustCompile(`(?m)^\s*const ([A-Za-z_$][\w$]*) = await nct\.dialog\.checkbox\((.*)\)\s*;\s*$`)
	formBare       = regexp.MustCompile(`(?m)^\s*await nct\.dialog\.form\((.*)\)\s*;\s*$`)
	selectBare     = regexp.MustCompile(`(?m)^\s*await nct\.dialog\.select\((.*)\)\s*;\s*$`)
	radioBare      = regexp.MustCompile(`(?m)^\s*await nct\.dialog\.radio\((.*)\)\s*;\s*$`)
	checkboxBare   = regexp.MustCompile(`(?m)^\s*await nct\.dialog\.checkbox\((.*)\)\s*;\s*$`)
)

// ParseRecordedScript turns recorder-generated JS into replay ops.
// Unknown nct APIs are rejected instead of pretending the Node worker ran them.
func ParseRecordedScript(source string) ([]ReplayOp, error) {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return nil, fmt.Errorf("Script content is empty")
	}
	var ops []ReplayOp
	for _, line := range scriptStatements(trimmed) {
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
			raw := strings.TrimSpace(match[1])
			kind := "log"
			if strings.HasPrefix(strings.TrimSpace(line), "await nct.dialog.alert") {
				kind = "alert"
			}
			if identifier.MatchString(raw) {
				ops = append(ops, ReplayOp{Kind: kind, Var: raw})
				continue
			}
			value, err := unquoteJS(raw)
			if err != nil {
				return nil, err
			}
			ops = append(ops, ReplayOp{Kind: kind, Value: value})
			continue
		}
		if match := disconnectCall.FindStringSubmatch(line); match != nil {
			ops = append(ops, ReplayOp{Kind: "disconnect"})
			continue
		}
		if match := startLogCall.FindStringSubmatch(line); match != nil {
			op := ReplayOp{Kind: "startLog"}
			if raw := strings.TrimSpace(match[1]); raw != "" {
				value, err := unquoteJS(raw)
				if err != nil {
					return nil, err
				}
				op.Value = value
			}
			ops = append(ops, op)
			continue
		}
		if stopLogCall.FindStringSubmatch(line) != nil {
			ops = append(ops, ReplayOp{Kind: "stopLog"})
			continue
		}
		if match := waitRegexCall.FindStringSubmatch(line); match != nil {
			args := splitJSArgs(match[1])
			if len(args) == 0 {
				return nil, fmt.Errorf("waitForRegex needs a pattern")
			}
			re, err := compileJSPattern(args[0])
			if err != nil {
				return nil, err
			}
			op := ReplayOp{Kind: "waitForRegex", Patterns: []string{args[0]}, Regexes: []*regexp2.Regexp{re}}
			if len(args) > 1 {
				op.Timeout = time.Duration(intArg(args[1])) * time.Millisecond
			}
			ops = append(ops, op)
			continue
		}
		if match := waitAnyCall.FindStringSubmatch(line); match != nil {
			args := splitJSArgs(match[1])
			if len(args) == 0 || !strings.HasPrefix(args[0], "[") || !strings.HasSuffix(args[0], "]") {
				return nil, fmt.Errorf("waitForAny needs a patterns array: %s", line)
			}
			items := splitJSArgs(args[0][1 : len(args[0])-1])
			if len(items) == 0 {
				return nil, fmt.Errorf("waitForAny needs a non-empty patterns array")
			}
			op := ReplayOp{Kind: "waitForAny"}
			for _, item := range items {
				re, err := compileJSPattern(item)
				if err != nil {
					return nil, err
				}
				op.Patterns = append(op.Patterns, item)
				op.Regexes = append(op.Regexes, re)
			}
			if len(args) > 1 {
				op.Timeout = time.Duration(intArg(args[1])) * time.Millisecond
			}
			ops = append(ops, op)
			continue
		}
		if match := sendCall.FindStringSubmatch(line); match != nil {
			op, err := sendArg(match[1])
			if err != nil {
				return nil, err
			}
			op.Kind = "send"
			ops = append(ops, op)
			continue
		}
		if clearCall.FindStringSubmatch(line) != nil {
			ops = append(ops, ReplayOp{Kind: "clear"})
			continue
		}
		if match := getTextAssign.FindStringSubmatch(line); match != nil {
			op := ReplayOp{Kind: "getText", Var: match[1]}
			if rawArgs := strings.TrimSpace(match[2]); rawArgs != "" {
				args := splitJSArgs(rawArgs)
				if len(args) != 2 {
					return nil, fmt.Errorf("screen.getText needs zero or two row arguments: %s", line)
				}
				op.Current = intArg(args[0])
				op.Total = intArg(args[1])
				op.RangeArgs = true
			}
			ops = append(ops, op)
			continue
		}
		if match := confirmAssign.FindStringSubmatch(line); match != nil {
			message, err := unquoteJS(strings.TrimSpace(match[2]))
			if err != nil {
				return nil, err
			}
			ops = append(ops, ReplayOp{Kind: "confirm", Var: match[1], Value: message})
			continue
		}
		if match := formAssign.FindStringSubmatch(line); match != nil {
			op, err := formOp(match[2], match[1], "")
			if err != nil {
				return nil, err
			}
			ops = append(ops, op)
			continue
		}
		if match := selectAssign.FindStringSubmatch(line); match != nil {
			op, err := choiceOp("select", match[2], match[1])
			if err != nil {
				return nil, err
			}
			ops = append(ops, op)
			continue
		}
		if match := radioAssign.FindStringSubmatch(line); match != nil {
			op, err := choiceOp("radio", match[2], match[1])
			if err != nil {
				return nil, err
			}
			ops = append(ops, op)
			continue
		}
		if match := checkboxAssign.FindStringSubmatch(line); match != nil {
			op, err := checkboxOp(match[2], match[1])
			if err != nil {
				return nil, err
			}
			ops = append(ops, op)
			continue
		}
		if match := formBare.FindStringSubmatch(line); match != nil {
			op, err := formOp(match[1], "", "")
			if err != nil {
				return nil, err
			}
			ops = append(ops, op)
			continue
		}
		if match := selectBare.FindStringSubmatch(line); match != nil {
			op, err := choiceOp("select", match[1], "")
			if err != nil {
				return nil, err
			}
			ops = append(ops, op)
			continue
		}
		if match := radioBare.FindStringSubmatch(line); match != nil {
			op, err := choiceOp("radio", match[1], "")
			if err != nil {
				return nil, err
			}
			ops = append(ops, op)
			continue
		}
		if match := checkboxBare.FindStringSubmatch(line); match != nil {
			op, err := checkboxOp(match[1], "")
			if err != nil {
				return nil, err
			}
			ops = append(ops, op)
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

// sendArg accepts a string literal or variable reference with an optional
// trailing { sensitive: true } options object.
func sendArg(raw string) (ReplayOp, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ReplayOp{}, fmt.Errorf("send needs a value")
	}
	sensitive := false
	if match := regexp.MustCompile(`^(.*),\s*\{\s*sensitive:\s*true\s*\}$`).FindStringSubmatch(raw); match != nil {
		raw = strings.TrimSpace(match[1])
		sensitive = true
	}
	if raw != "" && raw[0] == '"' {
		value, err := unquoteJS(raw)
		if err != nil {
			return ReplayOp{}, err
		}
		return ReplayOp{Value: value, Sensitive: sensitive}, nil
	}
	if identifier.MatchString(raw) {
		return ReplayOp{Var: raw, Sensitive: sensitive}, nil
	}
	return ReplayOp{}, fmt.Errorf("unsupported send argument: %s", raw)
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

// compileJSPattern mirrors the Electron buffer semantics: a string is an
// escaped literal, a /body/flags form compiles as a regex with i/m/s honored.
// Patterns run on the regexp2 engine so JavaScript backreferences and
// lookarounds keep working.
func compileJSPattern(raw string) (*regexp2.Regexp, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "/") && strings.HasSuffix(raw, "/") {
		return nil, fmt.Errorf("bare regex literals are not supported here; quote the pattern: %s", raw)
	}
	if len(raw) >= 3 && raw[0] == '"' {
		value, err := strconv.Unquote(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern string: %s", raw)
		}
		if slash := regexp.MustCompile(`^/(.+)/([a-z]*)$`).FindStringSubmatch(value); slash != nil {
			return compileRegex(slash[1], slash[2])
		}
		return regexp2.Compile(regexp.QuoteMeta(value), regexp2.None)
	}
	return nil, fmt.Errorf("waitFor pattern must be a quoted string: %s", raw)
}

func compileRegex(body, flags string) (*regexp2.Regexp, error) {
	options := regexp2.None
	for _, flag := range flags {
		switch flag {
		case 'i':
			options |= regexp2.IgnoreCase
		case 'm':
			options |= regexp2.Multiline
		case 's':
			options |= regexp2.Singleline
		case 'g', 'y', 'u':
			// Global/sticky don't change wait semantics; unicode is default.
		default:
			return nil, fmt.Errorf("unsupported regex flag: %c", flag)
		}
	}
	re, err := regexp2.Compile(body, options)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %v", err)
	}
	// Guard against catastrophic backtracking hanging the run.
	re.MatchTimeout = 2 * time.Second
	return re, nil
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

func scriptStatements(source string) []string {
	var stmts []string
	var pending strings.Builder
	depth := 0
	inString := false
	var quote byte
	escape := false
	flushPending := func() {
		line := strings.TrimSpace(pending.String())
		pending.Reset()
		depth = 0
		inString = false
		escape = false
		if line != "" {
			stmts = append(stmts, line)
		}
	}
	isWrapper := func(line string) bool {
		return line == "" || strings.HasPrefix(line, "//") || line == "async function main() {" || line == "}" || line == "await main();"
	}
	for _, raw := range strings.Split(source, "\n") {
		line := strings.TrimSpace(raw)
		if pending.Len() == 0 && isWrapper(line) {
			if line != "" {
				stmts = append(stmts, line)
			}
			continue
		}
		if pending.Len() > 0 {
			pending.WriteByte(' ')
		}
		for i := 0; i < len(line); i++ {
			ch := line[i]
			pending.WriteByte(ch)
			if inString {
				if escape {
					escape = false
					continue
				}
				if ch == '\\' {
					escape = true
					continue
				}
				if ch == quote {
					inString = false
				}
				continue
			}
			if ch == '"' || ch == '\'' {
				inString = true
				quote = ch
				continue
			}
			switch ch {
			case '{', '[', '(':
				depth++
			case '}', ']', ')':
				if depth > 0 {
					depth--
				}
			}
		}
		if !inString && depth == 0 {
			flushPending()
		}
	}
	flushPending()
	return stmts
}

type DialogOption struct {
	Label       string `json:"label"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	Disabled    bool   `json:"disabled,omitempty"`
}

type DialogCondition struct {
	Field     string `json:"field"`
	Equals    any    `json:"equals,omitempty"`
	NotEquals any    `json:"notEquals,omitempty"`
	Truthy    *bool  `json:"truthy,omitempty"`
	Falsy     *bool  `json:"falsy,omitempty"`
}

type DialogField struct {
	Type         string           `json:"type"`
	Name         string           `json:"name"`
	Label        string           `json:"label"`
	Description  string           `json:"description,omitempty"`
	Required     bool             `json:"required"`
	VisibleWhen  *DialogCondition `json:"visibleWhen,omitempty"`
	Options      []DialogOption   `json:"options,omitempty"`
	DefaultValue any              `json:"defaultValue,omitempty"`
	Placeholder  string           `json:"placeholder,omitempty"`
	Min          *float64         `json:"min,omitempty"`
	Max          *float64         `json:"max,omitempty"`
	Step         *float64         `json:"step,omitempty"`
}

type DialogForm struct {
	Title       string        `json:"title,omitempty"`
	Message     string        `json:"message"`
	SubmitLabel string        `json:"submitLabel,omitempty"`
	CancelLabel string        `json:"cancelLabel,omitempty"`
	Fields      []DialogField `json:"fields"`
}

func formOp(raw, varName, extract string) (ReplayOp, error) {
	form, err := parseDialogForm(raw)
	if err != nil {
		return ReplayOp{}, err
	}
	return ReplayOp{Kind: "form", Var: varName, Value: form.Message, Form: form, Extract: extract}, nil
}

func choiceOp(fieldType, raw, varName string) (ReplayOp, error) {
	args := splitJSArgs(raw)
	if len(args) < 2 {
		return ReplayOp{}, fmt.Errorf("dialog.%s needs a message and options", fieldType)
	}
	message, err := unquoteJS(args[0])
	if err != nil {
		return ReplayOp{}, err
	}
	options, err := parseJSValue(args[1])
	if err != nil {
		return ReplayOp{}, err
	}
	var defaultValue any
	if len(args) > 2 && args[2] != "undefined" {
		defaultValue, err = parseJSValue(args[2])
		if err != nil {
			return ReplayOp{}, err
		}
	}
	form, err := normalizeDialogForm(map[string]any{
		"message": message,
		"fields": []any{map[string]any{
			"type":         fieldType,
			"name":         "value",
			"label":        message,
			"options":      options,
			"defaultValue": defaultValue,
		}},
	})
	if err != nil {
		return ReplayOp{}, err
	}
	return ReplayOp{Kind: "form", Var: varName, Value: form.Message, Form: form, Extract: "value"}, nil
}

func checkboxOp(raw, varName string) (ReplayOp, error) {
	args := splitJSArgs(raw)
	if len(args) < 1 {
		return ReplayOp{}, fmt.Errorf("dialog.checkbox needs a message")
	}
	message, err := unquoteJS(args[0])
	if err != nil {
		return ReplayOp{}, err
	}
	defaultValue := any(false)
	if len(args) > 1 && args[1] != "undefined" {
		defaultValue, err = parseJSValue(args[1])
		if err != nil {
			return ReplayOp{}, err
		}
	}
	form, err := normalizeDialogForm(map[string]any{
		"message": message,
		"fields": []any{map[string]any{
			"type":         "checkbox",
			"name":         "value",
			"label":        message,
			"defaultValue": defaultValue,
		}},
	})
	if err != nil {
		return ReplayOp{}, err
	}
	return ReplayOp{Kind: "form", Var: varName, Value: form.Message, Form: form, Extract: "value"}, nil
}

func parseDialogForm(raw string) (*DialogForm, error) {
	value, err := parseJSValue(raw)
	if err != nil {
		return nil, err
	}
	spec, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("dialog.form needs an object spec")
	}
	return normalizeDialogForm(spec)
}

func parseJSValue(raw string) (any, error) {
	converted, err := jsLiteralToJSON(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal([]byte(converted), &value); err != nil {
		return nil, fmt.Errorf("invalid dialog argument: %w", err)
	}
	return value, nil
}

type jsParser struct {
	s   string
	pos int
}

func jsLiteralToJSON(raw string) (string, error) {
	p := jsParser{s: raw}
	var b strings.Builder
	if err := p.writeValue(&b); err != nil {
		return "", err
	}
	p.skipSpace()
	if p.pos < len(p.s) {
		return "", fmt.Errorf("unexpected trailing dialog argument: %s", p.s[p.pos:])
	}
	return b.String(), nil
}

func (p *jsParser) writeValue(b *strings.Builder) error {
	p.skipSpace()
	if p.pos >= len(p.s) {
		return fmt.Errorf("unexpected end of dialog argument")
	}
	switch p.s[p.pos] {
	case '{':
		return p.writeObject(b)
	case '[':
		return p.writeArray(b)
	case '"', '\'':
		value, err := p.readString()
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		b.Write(encoded)
		return nil
	}
	if strings.HasPrefix(p.s[p.pos:], "true") && p.atomEnd(p.pos+4) {
		b.WriteString("true")
		p.pos += 4
		return nil
	}
	if strings.HasPrefix(p.s[p.pos:], "false") && p.atomEnd(p.pos+5) {
		b.WriteString("false")
		p.pos += 5
		return nil
	}
	if strings.HasPrefix(p.s[p.pos:], "null") && p.atomEnd(p.pos+4) {
		b.WriteString("null")
		p.pos += 4
		return nil
	}
	start := p.pos
	if p.s[p.pos] == '-' {
		p.pos++
	}
	digits := 0
	for p.pos < len(p.s) && p.s[p.pos] >= '0' && p.s[p.pos] <= '9' {
		p.pos++
		digits++
	}
	if p.pos < len(p.s) && p.s[p.pos] == '.' {
		p.pos++
		for p.pos < len(p.s) && p.s[p.pos] >= '0' && p.s[p.pos] <= '9' {
			p.pos++
			digits++
		}
	}
	if digits > 0 && p.atomEnd(p.pos) {
		b.WriteString(p.s[start:p.pos])
		return nil
	}
	return fmt.Errorf("unsupported dialog argument: %s", p.s[start:])
}

func (p *jsParser) writeObject(b *strings.Builder) error {
	p.pos++
	b.WriteByte('{')
	first := true
	for {
		p.skipSpace()
		if p.pos >= len(p.s) {
			return fmt.Errorf("unterminated dialog object")
		}
		if p.s[p.pos] == '}' {
			p.pos++
			b.WriteByte('}')
			return nil
		}
		if p.s[p.pos] == ',' {
			p.pos++
			continue
		}
		key, err := p.readKey()
		if err != nil {
			return err
		}
		p.skipSpace()
		if p.pos >= len(p.s) || p.s[p.pos] != ':' {
			return fmt.Errorf("dialog object key %q needs a value", key)
		}
		p.pos++
		if !first {
			b.WriteByte(',')
		}
		encoded, err := json.Marshal(key)
		if err != nil {
			return err
		}
		b.Write(encoded)
		b.WriteByte(':')
		if err := p.writeValue(b); err != nil {
			return err
		}
		first = false
		p.skipSpace()
		if p.pos < len(p.s) && p.s[p.pos] == ',' {
			p.pos++
		}
	}
}

func (p *jsParser) writeArray(b *strings.Builder) error {
	p.pos++
	b.WriteByte('[')
	first := true
	for {
		p.skipSpace()
		if p.pos >= len(p.s) {
			return fmt.Errorf("unterminated dialog array")
		}
		if p.s[p.pos] == ']' {
			p.pos++
			b.WriteByte(']')
			return nil
		}
		if p.s[p.pos] == ',' {
			p.pos++
			continue
		}
		if !first {
			b.WriteByte(',')
		}
		if err := p.writeValue(b); err != nil {
			return err
		}
		first = false
		p.skipSpace()
		if p.pos < len(p.s) && p.s[p.pos] == ',' {
			p.pos++
		}
	}
}

func (p *jsParser) readKey() (string, error) {
	p.skipSpace()
	if p.pos >= len(p.s) {
		return "", fmt.Errorf("dialog object is missing a key")
	}
	if p.s[p.pos] == '"' || p.s[p.pos] == '\'' {
		return p.readString()
	}
	start := p.pos
	if !isIdentStart(p.s[p.pos]) {
		return "", fmt.Errorf("invalid dialog object key: %s", p.s[p.pos:])
	}
	p.pos++
	for p.pos < len(p.s) && isIdentPart(p.s[p.pos]) {
		p.pos++
	}
	return p.s[start:p.pos], nil
}

func (p *jsParser) readString() (string, error) {
	quote := p.s[p.pos]
	p.pos++
	var b strings.Builder
	escape := false
	for p.pos < len(p.s) {
		ch := p.s[p.pos]
		p.pos++
		if escape {
			b.WriteByte(ch)
			escape = false
			continue
		}
		if ch == '\\' {
			escape = true
			continue
		}
		if ch == quote {
			return b.String(), nil
		}
		b.WriteByte(ch)
	}
	return "", fmt.Errorf("unterminated dialog string")
}

func (p *jsParser) skipSpace() {
	for p.pos < len(p.s) && (p.s[p.pos] == ' ' || p.s[p.pos] == '\t' || p.s[p.pos] == '\n' || p.s[p.pos] == '\r') {
		p.pos++
	}
}

func (p *jsParser) atomEnd(pos int) bool {
	if pos >= len(p.s) {
		return true
	}
	ch := p.s[pos]
	return ch == ',' || ch == '}' || ch == ']' || ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func isIdentStart(ch byte) bool {
	return ch == '_' || ch == '$' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9')
}

func normalizeDialogForm(spec map[string]any) (*DialogForm, error) {
	rawFields, ok := spec["fields"].([]any)
	if !ok || len(rawFields) == 0 {
		return nil, fmt.Errorf("dialog.form requires at least one field")
	}
	seen := make(map[string]int)
	fields := make([]DialogField, 0, len(rawFields))
	for _, rawField := range rawFields {
		fieldMap, ok := rawField.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("dialog form field must be an object")
		}
		field, err := normalizeDialogField(fieldMap, seen)
		if err != nil {
			return nil, err
		}
		fields = append(fields, field)
		seen[field.Name] = len(fields) - 1
	}
	for index, field := range fields {
		if field.VisibleWhen == nil {
			continue
		}
		dep, ok := seen[field.VisibleWhen.Field]
		if !ok {
			return nil, fmt.Errorf("dialog visibleWhen references unknown field: %s", field.VisibleWhen.Field)
		}
		if dep >= index {
			return nil, fmt.Errorf("dialog visibleWhen must reference an earlier field: %s", field.Name)
		}
	}
	form := &DialogForm{Fields: fields}
	if title, ok := spec["title"]; ok && title != nil {
		form.Title = fmt.Sprint(title)
	}
	if message, ok := spec["message"]; ok && message != nil {
		form.Message = fmt.Sprint(message)
	}
	if label, ok := spec["submitLabel"]; ok && label != nil {
		form.SubmitLabel = fmt.Sprint(label)
	}
	if label, ok := spec["cancelLabel"]; ok && label != nil {
		form.CancelLabel = fmt.Sprint(label)
	}
	return form, nil
}

func normalizeDialogField(field map[string]any, seen map[string]int) (DialogField, error) {
	fieldType := ""
	if field["type"] != nil {
		fieldType = strings.TrimSpace(fmt.Sprint(field["type"]))
	}
	switch fieldType {
	case "select", "checkbox", "radio", "textarea", "number":
	default:
		return DialogField{}, fmt.Errorf("unsupported dialog field type: %s", fieldType)
	}
	name := strings.TrimSpace(stringValue(field["name"]))
	if name == "" {
		return DialogField{}, fmt.Errorf("dialog form field name is required")
	}
	if name == "__proto__" || name == "prototype" || name == "constructor" {
		return DialogField{}, fmt.Errorf("dialog form field name is reserved: %s", name)
	}
	if _, exists := seen[name]; exists {
		return DialogField{}, fmt.Errorf("duplicate dialog form field name: %s", name)
	}
	label := stringValue(field["label"])
	if label == "" {
		label = name
	}
	required := fieldType != "checkbox"
	if raw, ok := field["required"]; ok {
		required = asBool(raw)
		if fieldType == "checkbox" {
			required = raw == true
		}
	}
	out := DialogField{
		Type:        fieldType,
		Name:        name,
		Label:       label,
		Description: stringValue(field["description"]),
		Required:    required,
	}
	if raw, ok := field["visibleWhen"]; ok && raw != nil {
		condition, err := normalizeDialogCondition(raw)
		if err != nil {
			return DialogField{}, err
		}
		out.VisibleWhen = condition
	}
	switch fieldType {
	case "checkbox":
		out.Required = field["required"] == true
		out.DefaultValue = asBool(field["defaultValue"])
	case "textarea":
		out.Placeholder = stringValue(field["placeholder"])
		out.DefaultValue = stringValue(field["defaultValue"])
	case "number":
		out.Placeholder = stringValue(field["placeholder"])
		if field["defaultValue"] != nil && field["defaultValue"] != "" {
			number, err := asFiniteNumber(field["defaultValue"], "defaultValue", name)
			if err != nil {
				return DialogField{}, err
			}
			out.DefaultValue = number
		}
		min, err := optionalNumber(field["min"], "min", name)
		if err != nil {
			return DialogField{}, err
		}
		max, err := optionalNumber(field["max"], "max", name)
		if err != nil {
			return DialogField{}, err
		}
		step, err := optionalNumber(field["step"], "step", name)
		if err != nil {
			return DialogField{}, err
		}
		if step != nil && *step <= 0 {
			return DialogField{}, fmt.Errorf("dialog number field step must be a positive finite number: %s", name)
		}
		if min != nil && max != nil && *min > *max {
			return DialogField{}, fmt.Errorf("dialog number field min cannot be greater than max: %s", name)
		}
		out.Min, out.Max, out.Step = min, max, step
	default:
		options, defaultValue, err := normalizeChoiceOptions(fieldType, field["options"], field["defaultValue"])
		if err != nil {
			return DialogField{}, err
		}
		out.Options = options
		out.DefaultValue = defaultValue
	}
	return out, nil
}

func normalizeDialogCondition(raw any) (*DialogCondition, error) {
	spec, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("dialog visibleWhen must be an object")
	}
	field := strings.TrimSpace(stringValue(spec["field"]))
	if field == "" {
		return nil, fmt.Errorf("dialog visibleWhen field is required")
	}
	operators := make([]string, 0, 4)
	for _, name := range []string{"equals", "notEquals", "truthy", "falsy"} {
		if _, ok := spec[name]; ok {
			operators = append(operators, name)
		}
	}
	if len(operators) != 1 {
		return nil, fmt.Errorf("dialog visibleWhen requires exactly one condition operator")
	}
	condition := &DialogCondition{Field: field}
	switch operators[0] {
	case "truthy":
		if spec["truthy"] != true {
			return nil, fmt.Errorf("dialog visibleWhen truthy must be true")
		}
		yes := true
		condition.Truthy = &yes
	case "falsy":
		if spec["falsy"] != true {
			return nil, fmt.Errorf("dialog visibleWhen falsy must be true")
		}
		yes := true
		condition.Falsy = &yes
	case "equals":
		condition.Equals = spec["equals"]
	default:
		condition.NotEquals = spec["notEquals"]
	}
	return condition, nil
}

func normalizeChoiceOptions(fieldType string, rawOptions any, defaultValue any) ([]DialogOption, string, error) {
	items, ok := rawOptions.([]any)
	if !ok || len(items) == 0 {
		return nil, "", fmt.Errorf("dialog %s field requires at least one option", fieldType)
	}
	options := make([]DialogOption, 0, len(items))
	seen := make(map[string]struct{})
	for _, item := range items {
		option, err := normalizeDialogOption(item)
		if err != nil {
			return nil, "", err
		}
		if _, exists := seen[option.Value]; exists {
			return nil, "", fmt.Errorf("dialog %s field option values must be unique: %s", fieldType, option.Value)
		}
		seen[option.Value] = struct{}{}
		options = append(options, option)
	}
	var firstEnabled string
	for _, option := range options {
		if !option.Disabled {
			firstEnabled = option.Value
			break
		}
	}
	if firstEnabled == "" {
		return nil, "", fmt.Errorf("dialog %s field requires at least one enabled option", fieldType)
	}
	selected := firstEnabled
	if defaultValue != nil {
		want := fmt.Sprint(defaultValue)
		for _, option := range options {
			if option.Value == want && !option.Disabled {
				selected = option.Value
				break
			}
		}
	}
	return options, selected, nil
}

func normalizeDialogOption(raw any) (DialogOption, error) {
	if text, ok := raw.(string); ok {
		if text == "" {
			return DialogOption{}, fmt.Errorf("dialog option value is required")
		}
		return DialogOption{Label: text, Value: text}, nil
	}
	spec, ok := raw.(map[string]any)
	if !ok {
		return DialogOption{}, fmt.Errorf("dialog option must be a string or object")
	}
	value := stringValue(spec["value"])
	if value == "" {
		return DialogOption{}, fmt.Errorf("dialog option value is required")
	}
	label := stringValue(spec["label"])
	if label == "" {
		label = value
	}
	return DialogOption{
		Label:       label,
		Value:       value,
		Description: stringValue(spec["description"]),
		Disabled:    asBool(spec["disabled"]),
	}, nil
}

func stringValue(raw any) string {
	if raw == nil {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return value
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case bool:
		if value {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(raw))
	}
}

func asBool(raw any) bool {
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		return value == "true"
	default:
		return false
	}
}

func optionalNumber(raw any, field, name string) (*float64, error) {
	if raw == nil || raw == "" {
		return nil, nil
	}
	number, err := asFiniteNumber(raw, field, name)
	if err != nil {
		return nil, err
	}
	return &number, nil
}

func asFiniteNumber(raw any, field, name string) (float64, error) {
	switch value := raw.(type) {
	case float64:
		return value, nil
	case json.Number:
		number, err := value.Float64()
		if err != nil {
			return 0, fmt.Errorf("dialog number field %s must be a finite number: %s", field, name)
		}
		return number, nil
	case string:
		number, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, fmt.Errorf("dialog number field %s must be a finite number: %s", field, name)
		}
		return number, nil
	default:
		return 0, fmt.Errorf("dialog number field %s must be a finite number: %s", field, name)
	}
}
