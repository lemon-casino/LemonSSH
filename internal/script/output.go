package script

import (
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	outputCap           = 1024 * 1024
	freshMatchTailSlack = 512
)

var (
	defaultPromptSuffixes = []string{"# ", "$ ", "~# ", "~$ ", "% "}
	promptEnd             = regexp.MustCompile(`(?:~[#$]\s*|[@][^\n]{0,120}[:][^\n]{0,120}[#$%]\s*)$`)
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
	return promptEnd.MatchString(last)
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
// window of the rolling buffer.
func anyRegexFresh(text string, patterns []*regexp.Regexp) bool {
	start := 0
	if len(text) > regexScanTail {
		start = len(text) - regexScanTail - utf8.UTFMax
		if start < 0 {
			start = 0
		}
	}
	tail := text[start:]
	for _, re := range patterns {
		for _, m := range re.FindAllStringIndex(tail, -1) {
			if start+m[1] >= len(text)-freshMatchTailSlack {
				return true
			}
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
