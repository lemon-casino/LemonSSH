package script

import (
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/dlclark/regexp2"
)

const (
	outputCap           = 1024 * 1024
	freshMatchTailSlack = 512
)

var (
	defaultPromptSuffixes = []string{"# ", "$ ", "~# ", "~$ ", "% "}
	promptEnd             = regexp2.MustCompile(`(?:~[#$]\s*|[@][^\n]{0,120}[:][^\n]{0,120}[#$%]\s*)$`, regexp2.None)
)

// OutputWatch holds a rolling UTF-8 view of one session's terminal bytes.
type OutputWatch struct {
	mu   sync.Mutex
	buf  strings.Builder
	wait []chan struct{}
}

func (w *OutputWatch) Append(data []byte) {
	if len(data) == 0 {
		return
	}
	w.mu.Lock()
	w.buf.Write(data)
	if w.buf.Len() > outputCap {
		keep := w.buf.String()[w.buf.Len()-outputCap/2:]
		w.buf.Reset()
		w.buf.WriteString(keep)
	}
	waiters := w.wait
	w.wait = nil
	w.mu.Unlock()
	for _, waiter := range waiters {
		close(waiter)
	}
}

func (w *OutputWatch) Reset() {
	w.mu.Lock()
	w.buf.Reset()
	w.mu.Unlock()
}

func (w *OutputWatch) snapshot() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func (w *OutputWatch) notify() <-chan struct{} {
	ch := make(chan struct{})
	w.mu.Lock()
	w.wait = append(w.wait, ch)
	w.mu.Unlock()
	return ch
}

func looksLikePrompt(text string) bool {
	trimmed := strings.TrimRight(text, "\r\n")
	if trimmed == "" {
		return false
	}
	last := trimmed
	if idx := strings.LastIndexAny(trimmed, "\r\n"); idx >= 0 {
		last = trimmed[idx+1:]
	}
	for _, suffix := range defaultPromptSuffixes {
		if strings.HasSuffix(last, suffix) {
			return true
		}
	}
	if match, _ := promptEnd.FindStringMatch(last); match != nil {
		return true
	}
	return false
}

func containsFresh(text, needle string) bool {
	if needle == "" {
		return true
	}
	index := strings.LastIndex(text, needle)
	if index < 0 {
		return false
	}
	end := index + len(needle)
	return end >= len(text)-freshMatchTailSlack
}

const regexScanTail = 64 * 1024

// anyRegexFresh reports whether any pattern matches within the fresh tail
// window of the rolling buffer. Patterns run on the regexp2 engine so
// JavaScript-only features (backreferences, lookaround) keep working.
func anyRegexFresh(text string, patterns []*regexp2.Regexp) bool {
	start := 0
	if len(text) > regexScanTail {
		start = len(text) - regexScanTail - utf8.UTFMax
		if start < 0 {
			start = 0
		}
	}
	tail := text[start:]
	for _, re := range patterns {
		match, err := re.FindStringMatchStartingAt(tail, start)
		for match != nil {
			if match.Index+match.Length >= len(text)-freshMatchTailSlack {
				return true
			}
			match, err = re.FindNextMatch(match)
			if err != nil {
				break
			}
		}
		if err != nil {
			continue
		}
	}
	return false
}

func validUTF8Tail(text string) string {
	if utf8.ValidString(text) {
		return text
	}
	for i := 0; i < len(text); i++ {
		if utf8.ValidString(text[i:]) {
			return text[i:]
		}
	}
	return ""
}
