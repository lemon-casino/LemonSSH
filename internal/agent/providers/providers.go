// Package providers implements the Go provider protocol families (W09,
// P7-03, AI-03): OpenAI Chat, Anthropic Messages and Google
// GenerateContent streaming, on top of one shared SSE parser and one
// unified event model. All network traffic must flow through an injected
// netpolicy-constrained http.Client; this package never dials by itself.
package providers

import (
	"net/http"
	"time"
)

// HTTPClient is the netpolicy-constrained client providers must use.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Clock enables fake-clock retry tests (T23).
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// RealClock is the production clock.
type RealClock struct{}

func (RealClock) Now() time.Time                         { return time.Now() }
func (RealClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
