package capability

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Grant is one stored approval rule, shared shape with
// electron/shared/permissionGrants.cjs (sanitizePermissionGrants output).
type Grant struct {
	ID             string            `json:"id"`
	CapabilityID   string            `json:"capabilityId"`
	SessionPattern string            `json:"sessionPattern"`
	CommandPattern string            `json:"commandPattern,omitempty"`
	ArgsPattern    map[string]string `json:"argsPattern,omitempty"`
	CreatedAt      int64             `json:"createdAt"`
	Note           string            `json:"note,omitempty"`
}

// MatchContext carries what a grant may match against: the capability under
// execution and the request arguments.
type MatchContext struct {
	CapabilityID string
	Args         map[string]any
}

// ---- pattern matching ----

// PatternMatches applies one grant pattern to one value. Patterns are "*",
// "host:"-prefixed globs, "/regex/flags" forms or plain globs.
func PatternMatches(pattern, value string) bool {
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "host:") {
		return globOrRegexMatch(strings.TrimPrefix(pattern, "host:"), value)
	}
	return globOrRegexMatch(pattern, value)
}

func globOrRegexMatch(pattern, value string) bool {
	if strings.HasPrefix(pattern, "/") {
		if lastSlash := strings.LastIndex(pattern, "/"); lastSlash > 0 {
			re, err := compileJSRegex(pattern[1:lastSlash], pattern[lastSlash+1:])
			if err != nil {
				return false
			}
			return re.MatchString(value)
		}
	}

	if !strings.ContainsAny(pattern, "*?") {
		return value == pattern
	}

	// Glob semantics with the OpenCode Wildcard.match trailing rule: a
	// pattern ending in " *" also matches the bare prefix (optional args).
	escaped := jsRegexEscape(pattern)
	escaped = strings.ReplaceAll(escaped, "*", ".*")
	escaped = strings.ReplaceAll(escaped, "?", ".")
	if strings.HasSuffix(escaped, " .*") {
		escaped = escaped[:len(escaped)-3] + "( .*)?"
	}
	re := regexp.MustCompile("(?s)^" + escaped + "$")
	return re.MatchString(value)
}

// jsRegexEscape mirrors the CJS character escape: only . + ^ $ { } ( ) | [ ]
// \ are escaped, so * and ? survive as glob markers.
func jsRegexEscape(pattern string) string {
	replacer := strings.NewReplacer(
		".", `\.`, "+", `\+`, "^", `\^`, "$", `\$`,
		"{", `\{`, "}", `\}`, "(", `\(`, ")", `\)`,
		"|", `\|`, "[", `\[`, "]", `\]`, `\`, `\\`,
	)
	return replacer.Replace(pattern)
}

// compileJSRegex compiles a JavaScript RegExp body with the subset of JS
// flags the CJS matcher can produce. Unknown or unsupported flags fail
// closed, matching the CJS try/catch that swallows RegExp construction
// errors. Bodies using JS-only syntax (backrefs, lookaround) fail RE2
// compilation and therefore also fail closed.
func compileJSRegex(body, flags string) (*regexp.Regexp, error) {
	for _, flag := range flags {
		if !strings.ContainsRune("dgimsuvy", flag) {
			return nil, fmt.Errorf("unsupported regex flag %q", flag)
		}
	}
	inline := ""
	if strings.ContainsRune(flags, 'i') {
		inline += "i"
	}
	if strings.ContainsRune(flags, 'm') {
		inline += "m"
	}
	if strings.ContainsRune(flags, 's') {
		inline += "s"
	}
	if inline != "" {
		inline = "(?" + inline + ")"
	}
	return regexp.Compile(inline + body)
}

func jsStringify(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(v)
	}
}

// ArgsPatternMatches requires every pattern key to be present in args and to
// match; an empty pattern object approves any args.
func ArgsPatternMatches(argsPattern map[string]string, args map[string]any) bool {
	if len(argsPattern) == 0 {
		return true
	}
	if args == nil {
		return false
	}
	for key, pattern := range argsPattern {
		raw, present := args[key]
		if !present {
			return false
		}
		if !PatternMatches(pattern, jsStringify(raw)) {
			return false
		}
	}
	return true
}

// ---- shell command segmentation ----

var cwdCommands = map[string]bool{
	"cd": true, "chdir": true, "popd": true, "pushd": true,
	"push-location": true, "set-location": true,
}

var shellTokenPattern = regexp.MustCompile(`(?:[^\s"']+|"[^"]*"|'[^']*')+`)

func unquoteShellToken(token string) string {
	if len(token) >= 2 {
		first := token[0]
		last := token[len(token)-1]
		if (first == '"' || first == '\'') && first == last {
			return token[1 : len(token)-1]
		}
	}
	return token
}

func tokenizeShellCommand(command string) []string {
	matches := shellTokenPattern.FindAllString(command, -1)
	tokens := make([]string, len(matches))
	for i, match := range matches {
		tokens[i] = unquoteShellToken(match)
	}
	return tokens
}

func isJSWhitespace(r rune) bool { return unicode.IsSpace(r) }

func lastNonWhitespaceChar(value string) string {
	for len(value) > 0 {
		r, size := utf8.DecodeLastRuneInString(value)
		if !isJSWhitespace(r) {
			return value[len(value)-size:]
		}
		value = value[:len(value)-size]
	}
	return ""
}

func readArithmeticExpansionEnd(segment string, startIndex int) int {
	if startIndex+2 >= len(segment) ||
		segment[startIndex] != '$' ||
		segment[startIndex+1] != '(' ||
		segment[startIndex+2] != '(' {
		return -1
	}

	depth := 1
	quote := byte(0)

	for index := startIndex + 3; index < len(segment); index++ {
		char := segment[index]
		var next byte
		if index+1 < len(segment) {
			next = segment[index+1]
		}

		if quote != 0 {
			if char == '\\' && quote != '\'' && next != 0 {
				index++
				continue
			}
			if char == quote {
				quote = 0
			}
			continue
		}

		if char == '\\' && next != 0 {
			index++
			continue
		}
		if char == '"' || char == '\'' || char == '`' {
			quote = char
			continue
		}
		if char == '(' {
			depth++
			continue
		}
		if char == ')' {
			if depth == 1 && next == ')' {
				return index + 2
			}
			if depth > 1 {
				depth--
			}
		}
	}

	return len(segment)
}

func hasExecutableShellExpansion(segment string) bool {
	quote := byte(0)

	for index := 0; index < len(segment); index++ {
		char := segment[index]
		var next byte
		if index+1 < len(segment) {
			next = segment[index+1]
		}

		if quote != 0 {
			if char == '\\' && quote == '"' && next != 0 {
				index++
				continue
			}
			if char == quote {
				quote = 0
				continue
			}
			if quote == '"' && char == '$' && next == '(' {
				return true
			}
			if quote == '"' && char == '`' {
				return true
			}
			continue
		}

		if char == '\\' && next != 0 {
			index++
			continue
		}
		if char == '\'' {
			quote = char
			continue
		}
		if char == '"' {
			quote = char
			continue
		}
		if char == '`' {
			return true
		}
		if char == '$' && next == '(' {
			return true
		}
		if (char == '<' || char == '>') && next == '(' {
			return true
		}
	}

	return false
}

func readEscapeDigits(value string, startIndex, maxLength int, digitOK func(byte) bool) (digits string, endIndex int, ok bool) {
	end := startIndex
	for end < len(value) && end < startIndex+maxLength && digitOK(value[end]) {
		end++
	}
	if end == startIndex {
		return "", startIndex, false
	}
	return value[startIndex:end], end - 1, true
}

func codePointToString(codePoint int) string {
	if codePoint < 0 || codePoint > 0x10FFFF {
		return ""
	}
	return string(rune(codePoint))
}

func readAnsiCEscape(value string, backslashIndex int) (text string, endIndex int) {
	escapeIndex := backslashIndex + 1
	if escapeIndex >= len(value) {
		return "\\", backslashIndex
	}
	char := value[escapeIndex]

	switch char {
	case 'a':
		return "\a", escapeIndex
	case 'b':
		return "\b", escapeIndex
	case 'e', 'E':
		return "\x1b", escapeIndex
	case 'f':
		return "\f", escapeIndex
	case 'n':
		return "\n", escapeIndex
	case 'r':
		return "\r", escapeIndex
	case 't':
		return "\t", escapeIndex
	case 'v':
		return "\v", escapeIndex
	case '\\', '\'', '"', '?':
		return string(char), escapeIndex
	}

	isHex := func(b byte) bool {
		return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F'
	}
	isOctal := func(b byte) bool { return b >= '0' && b <= '7' }

	if char == 'x' {
		if digits, end, ok := readEscapeDigits(value, escapeIndex+1, 2, isHex); ok {
			return codePointToString(parseRadixDigits(digits, 16)), end
		}
		return "\\x", escapeIndex
	}

	if char == 'u' || char == 'U' {
		maxLen := 4
		if char == 'U' {
			maxLen = 8
		}
		if digits, end, ok := readEscapeDigits(value, escapeIndex+1, maxLen, isHex); ok {
			return codePointToString(parseRadixDigits(digits, 16)), end
		}
		return "\\" + string(char), escapeIndex
	}

	if isOctal(char) {
		if digits, end, ok := readEscapeDigits(value, escapeIndex, 3, isOctal); ok {
			return codePointToString(parseRadixDigits(digits, 8)), end
		}
	}

	return "\\" + string(char), escapeIndex
}

func parseRadixDigits(digits string, base int) int {
	parsed, err := strconv.ParseUint(digits, base, 32)
	if err != nil {
		return -1
	}
	return int(parsed)
}

type hereDocTerminator struct {
	text             string
	stripLeadingTabs bool
}

// readHereDocDelimiterWord consumes one delimiter word and returns its text
// plus the index where scanning stopped (the last processed character, or
// the separator that broke scanning). The index — not len(text) — is the
// resume point, because escapes consume more input than they emit.
func readHereDocDelimiterWord(segment string, startIndex int) (string, int) {
	index := startIndex
	for index < len(segment) && isJSWhitespace(rune(segment[index])) {
		index++
	}

	var text strings.Builder
	quote := byte(0)
	ansiQuote := false

	for ; index < len(segment); index++ {
		char := segment[index]
		var next byte
		if index+1 < len(segment) {
			next = segment[index+1]
		}

		if quote != 0 {
			if ansiQuote && char == '\\' {
				escape, end := readAnsiCEscape(segment, index)
				text.WriteString(escape)
				index = end
				continue
			}
			if char == '\\' && quote != '\'' && next != 0 {
				text.WriteByte(next)
				index++
				continue
			}
			if char == quote {
				quote = 0
				ansiQuote = false
				continue
			}
			text.WriteByte(char)
			continue
		}

		if isJSWhitespace(rune(char)) || char == ';' || char == '|' || char == '&' {
			break
		}

		if char == '\\' && next != 0 {
			text.WriteByte(next)
			index++
			continue
		}

		if char == '$' && (next == '\'' || next == '"') {
			quote = next
			ansiQuote = next == '\''
			index++
			continue
		}

		if char == '"' || char == '\'' || char == '`' {
			quote = char
			ansiQuote = false
			continue
		}

		text.WriteByte(char)
	}

	if text.Len() == 0 {
		return "", index
	}
	return text.String(), index
}

func extractHereDocTerminators(segment string) []hereDocTerminator {
	var terminators []hereDocTerminator
	quote := byte(0)

	for index := 0; index < len(segment); index++ {
		char := segment[index]
		var next byte
		if index+1 < len(segment) {
			next = segment[index+1]
		}

		if quote != 0 {
			if char == '\\' && quote != '\'' && next != 0 {
				index++
				continue
			}
			if char == quote {
				quote = 0
			}
			continue
		}

		if arithmeticEnd := readArithmeticExpansionEnd(segment, index); arithmeticEnd >= 0 {
			index = arithmeticEnd - 1
			continue
		}

		if char == '\\' && next != 0 {
			index++
			continue
		}

		if char == '"' || char == '\'' || char == '`' {
			quote = char
			continue
		}

		if char != '<' || next != '<' {
			continue
		}
		if index+2 < len(segment) && segment[index+2] == '<' {
			index += 2
			continue
		}

		stripLeadingTabs := index+2 < len(segment) && segment[index+2] == '-'
		delimiterStart := index + 2
		if stripLeadingTabs {
			delimiterStart = index + 3
		}
		delimiter, delimiterEnd := readHereDocDelimiterWord(segment, delimiterStart)
		if delimiter == "" {
			continue
		}
		terminators = append(terminators, hereDocTerminator{text: delimiter, stripLeadingTabs: stripLeadingTabs})
		index = delimiterEnd - 1
	}

	return terminators
}

func skipHereDocBodies(command string, startIndex int, terminators []hereDocTerminator) int {
	cursor := startIndex

	for _, terminator := range terminators {
		for cursor < len(command) {
			lineEnd := strings.IndexByte(command[cursor:], '\n')
			end := len(command)
			if lineEnd != -1 {
				end = cursor + lineEnd
			}
			rawLine := command[cursor:end]
			line := rawLine
			if terminator.stripLeadingTabs {
				line = strings.TrimLeft(line, "\t")
			}
			if lineEnd == -1 {
				cursor = len(command)
			} else {
				cursor = end + 1
			}
			if line == terminator.text {
				break
			}
		}
	}

	return cursor
}

func splitShellCommandSegments(command string) []string {
	var segments []string
	var current strings.Builder
	quote := byte(0)
	inComment := false
	var lineHereDocTerminators []hereDocTerminator

	flush := func() {
		segment := strings.TrimSpace(current.String())
		if segment != "" {
			segments = append(segments, segment)
		}
		current.Reset()
	}

	flushCommandSegment := func() {
		lineHereDocTerminators = append(lineHereDocTerminators, extractHereDocTerminators(current.String())...)
		flush()
	}

	finishLine := func(bodyStartIndex int) int {
		flushCommandSegment()
		terminators := lineHereDocTerminators
		lineHereDocTerminators = nil
		if len(terminators) == 0 {
			return bodyStartIndex
		}
		return skipHereDocBodies(command, bodyStartIndex, terminators)
	}

	for index := 0; index < len(command); index++ {
		char := command[index]
		var next byte
		if index+1 < len(command) {
			next = command[index+1]
		}

		if inComment {
			if char == '\n' {
				inComment = false
				index = finishLine(index+1) - 1
			}
			continue
		}

		if quote != 0 {
			current.WriteByte(char)
			if char == '\\' && quote != '\'' && next != 0 {
				current.WriteByte(next)
				index++
				continue
			}
			if char == quote {
				quote = 0
			}
			continue
		}

		if char == '\\' && next == '\n' {
			index++
			continue
		}

		if char == '\\' && next != 0 {
			current.WriteByte(char)
			current.WriteByte(next)
			index++
			continue
		}

		if char == '"' || char == '\'' || char == '`' {
			quote = char
			current.WriteByte(char)
			continue
		}

		if arithmeticEnd := readArithmeticExpansionEnd(command, index); arithmeticEnd >= 0 {
			current.WriteString(command[index:arithmeticEnd])
			index = arithmeticEnd - 1
			continue
		}

		if char == '#' {
			previous := current.Len()
			if previous == 0 || isJSWhitespace(rune(current.String()[previous-1])) {
				inComment = true
				continue
			}
		}

		if char == '\n' {
			index = finishLine(index+1) - 1
			continue
		}

		if char == ';' {
			flushCommandSegment()
			continue
		}

		if char == '&' && next == '&' {
			flushCommandSegment()
			index++
			continue
		}

		if char == '|' && next == '|' {
			flushCommandSegment()
			index++
			continue
		}

		if char == '&' {
			previous := lastNonWhitespaceChar(current.String())
			if next == '>' || previous == ">" || previous == "<" {
				current.WriteByte(char)
				continue
			}
			flushCommandSegment()
			continue
		}

		if char == '|' {
			flushCommandSegment()
			if next == '&' {
				index++
			}
			continue
		}

		current.WriteByte(char)
	}

	flush()
	return segments
}

func extractGrantableShellCommandSegments(command string) []string {
	var grantable []string
	for _, segment := range splitShellCommandSegments(command) {
		tokens := tokenizeShellCommand(segment)
		if len(tokens) == 0 {
			continue
		}
		cmd := strings.ToLower(tokens[0])
		if cwdCommands[cmd] && !hasExecutableShellExpansion(segment) {
			continue
		}
		grantable = append(grantable, segment)
	}
	return grantable
}

// ---- grant matching ----

func matchCommandPatternGrants(rules []Grant, capabilityID, command string, args map[string]any) *Grant {
	segments := extractGrantableShellCommandSegments(command)
	if len(segments) == 0 {
		return nil
	}

	var eligible []*Grant
	for i := range rules {
		rule := &rules[i]
		if rule.CapabilityID == capabilityID && rule.CommandPattern != "" && ArgsPatternMatches(rule.ArgsPattern, args) {
			eligible = append(eligible, rule)
		}
	}
	if len(eligible) == 0 {
		return nil
	}

	var first *Grant
	for _, segment := range segments {
		var matched *Grant
		for _, rule := range eligible {
			if PatternMatches(rule.CommandPattern, segment) {
				matched = rule
				break
			}
		}
		if matched == nil {
			return nil
		}
		if first == nil {
			first = matched
		}
	}

	return first
}

// MatchPermissionGrant resolves the grant that satisfies the request, or
// nil. Command-pattern grants must cover every grantable shell segment.
func MatchPermissionGrant(rules []Grant, ctx MatchContext) *Grant {
	if len(rules) == 0 {
		return nil
	}

	args := ctx.Args
	command := ""
	if s, ok := args["command"].(string); ok {
		command = s
	}
	if command != "" {
		if matched := matchCommandPatternGrants(rules, ctx.CapabilityID, command, args); matched != nil {
			return matched
		}
	}

	for i := range rules {
		rule := &rules[i]
		if rule.CapabilityID == "" || rule.CapabilityID != ctx.CapabilityID {
			continue
		}
		if rule.CommandPattern != "" {
			continue
		}
		if !ArgsPatternMatches(rule.ArgsPattern, args) {
			continue
		}
		return rule
	}

	return nil
}

// SanitizePermissionGrants normalizes persisted grant records: unknown
// entries are dropped, strings trimmed, caps enforced and missing IDs or
// timestamps generated.
func SanitizePermissionGrants(raw []Grant) []Grant {
	if len(raw) == 0 {
		return nil
	}

	var result []Grant
	for _, entry := range raw {
		capabilityID := strings.TrimSpace(entry.CapabilityID)
		if capabilityID == "" {
			continue
		}

		rule := Grant{
			ID:           strings.TrimSpace(entry.ID),
			CapabilityID: capabilityID,
			SessionPattern: func() string {
				if trimmed := strings.TrimSpace(entry.SessionPattern); trimmed != "" {
					return trimmed
				}
				return "*"
			}(),
			CreatedAt: entry.CreatedAt,
		}
		if rule.ID == "" {
			rule.ID = generateGrantID()
		}
		if len(rule.ID) > 64 {
			rule.ID = rule.ID[:64]
		}
		if rule.CreatedAt <= 0 {
			rule.CreatedAt = time.Now().UnixMilli()
		}

		if trimmed := strings.TrimSpace(entry.CommandPattern); trimmed != "" {
			rule.CommandPattern = trimmed
		}
		if len(entry.ArgsPattern) > 0 {
			argsPattern := make(map[string]string, len(entry.ArgsPattern))
			for key, value := range entry.ArgsPattern {
				if trimmed := strings.TrimSpace(value); trimmed != "" {
					argsPattern[key] = trimmed
				}
			}
			if len(argsPattern) > 0 {
				rule.ArgsPattern = argsPattern
			}
		}
		if trimmed := strings.TrimSpace(entry.Note); trimmed != "" {
			rule.Note = trimmed
			if len(rule.Note) > 240 {
				rule.Note = rule.Note[:240]
			}
		}

		result = append(result, rule)
	}

	return result
}

func generateGrantID() string {
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Sprintf("grant_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("grant_%d_%s", time.Now().UnixMilli(), hex.EncodeToString(random[:]))
}
