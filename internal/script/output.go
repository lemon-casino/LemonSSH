package script

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/dlclark/regexp2"
)

const outputCap = 1024 * 1024

var promptEnd = regexp2.MustCompile(`(?:[#$%>]\s*|[@][^\n]{0,120}[:][^\n]{0,120}[#$%]\s*)$`, regexp2.None)

// OutputWatch retains display text separately from the consumed wait cursor.
type OutputWatch struct {
	mu       sync.Mutex
	text     string
	base     int64
	consumed int64
	changed  chan struct{}
}

func (w *OutputWatch) Append(data []byte) {
	if len(data) == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.text += string(data)
	if len(w.text) > outputCap {
		drop := len(w.text) - outputCap/2
		for drop < len(w.text) && !utf8.RuneStart(w.text[drop]) {
			drop++
		}
		w.text = w.text[drop:]
		w.base += int64(drop)
	}
	if w.changed != nil {
		close(w.changed)
	}
	w.changed = make(chan struct{})
}

func (w *OutputWatch) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.base += int64(len(w.text))
	w.text = ""
	w.consumed = w.base
	if w.changed != nil {
		close(w.changed)
	}
	w.changed = make(chan struct{})
}

func (w *OutputWatch) snapshot() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.text
}

func (w *OutputWatch) position() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.base + int64(len(w.text))
}

func (w *OutputWatch) consumeThrough(position int64) {
	w.mu.Lock()
	w.consumed = max(w.consumed, position)
	w.mu.Unlock()
}

type waitPattern struct {
	text string
	re   *regexp2.Regexp
}

func literalWaitPattern(text string) waitPattern { return waitPattern{text: text} }

func regexWaitPattern(body, flags string) (waitPattern, error) {
	re, err := compileRegex(body, flags)
	if err != nil {
		return waitPattern{}, err
	}
	re.MatchTimeout = 100 * time.Millisecond
	return waitPattern{re: re}, nil
}

func (w *OutputWatch) waitFor(ctx context.Context, patterns []waitPattern, timeout time.Duration, prompt bool) (string, int, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return "", -1, err
		}
		w.mu.Lock()
		if w.changed == nil {
			w.changed = make(chan struct{})
		}
		changed := w.changed
		start := int(max(w.base, w.consumed) - w.base)
		start = min(start, len(w.text))
		if prompt && start == len(w.text) {
			start = max(0, len(w.text)-512)
		}
		text := w.text[start:]
		base := w.base + int64(start)
		w.mu.Unlock()
		for index, pattern := range patterns {
			value, end, err := pattern.match(text)
			if err != nil {
				return "", -1, err
			}
			if end >= 0 {
				w.consumeThrough(base + int64(end))
				return value, index, nil
			}
		}
		select {
		case <-ctx.Done():
			return "", -1, ctx.Err()
		case <-timer.C:
			return "", -1, fmt.Errorf("timed out waiting for terminal output after %s", timeout)
		case <-changed:
		}
	}
}

func (p waitPattern) match(text string) (string, int, error) {
	if p.re == nil {
		if index := strings.Index(text, p.text); index >= 0 {
			return p.text, index + len(p.text), nil
		}
		return "", -1, nil
	}
	match, err := p.re.FindStringMatch(text)
	if err != nil {
		return "", -1, err
	}
	if match == nil {
		return "", -1, nil
	}
	// regexp2 indexes runes, while the rolling buffer cursor is a byte offset.
	runes := []rune(text)
	return match.String(), len(string(runes[:match.Index+match.Length])), nil
}

func looksLikePrompt(text string) bool {
	match, _ := promptEnd.FindStringMatch(strings.TrimRight(text, "\r\n"))
	return match != nil
}

func validUTF8Tail(text string) string {
	if utf8.ValidString(text) {
		return text
	}
	return strings.ToValidUTF8(text, "\uFFFD")
}

func stringWaitPattern(value string, regex bool) (waitPattern, error) {
	if strings.HasPrefix(value, "/") {
		if slash := strings.LastIndex(value[1:], "/"); slash >= 0 {
			slash++
			return regexWaitPattern(value[1:slash], value[slash+1:])
		}
	}
	if regex {
		return regexWaitPattern(value, "")
	}
	return regexWaitPattern(regexp.QuoteMeta(value), "")
}
