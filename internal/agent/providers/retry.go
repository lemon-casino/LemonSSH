package providers

import (
	"context"
	"math"
	"time"
)

// RetryPolicy is the single retry owner for provider calls (T23): exactly
// one layer may retry, honoring upstream Retry-After and backing off
// exponentially with a cap. The injected Clock makes delays testable and
// lets Stop interrupt a wait.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Clock       Clock
}

func (r RetryPolicy) withDefaults() RetryPolicy {
	if r.MaxAttempts <= 0 {
		r.MaxAttempts = 3
	}
	if r.BaseDelay <= 0 {
		r.BaseDelay = 500 * time.Millisecond
	}
	if r.MaxDelay <= 0 {
		r.MaxDelay = 20 * time.Second
	}
	if r.Clock == nil {
		r.Clock = RealClock{}
	}
	return r
}

// AttemptOutcome tells the retrier what happened to one attempt.
type AttemptOutcome struct {
	// Retryable marks transient failures (429, 5xx, transport resets).
	Retryable bool
	// RetryAfter is the upstream-honored delay; it overrides backoff when
	// positive (T23: no multiplicative retry).
	RetryAfter time.Duration
	// Err is surfaced to the caller when the attempt chain gives up.
	Err error
}

// Do runs fn until it succeeds, reports a non-retryable failure, or the
// attempt budget is exhausted. fn returns (value, outcome).
func (r RetryPolicy) Do(ctx context.Context, fn func(ctx context.Context, attempt int) (any, AttemptOutcome)) (any, error) {
	policy := r.withDefaults()
	var lastErr error

	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		value, outcome := fn(ctx, attempt)
		if outcome.Err == nil {
			return value, nil
		}
		lastErr = outcome.Err
		if !outcome.Retryable || attempt == policy.MaxAttempts {
			return nil, lastErr
		}

		delay := policy.backoff(attempt, outcome.RetryAfter)
		timer := policy.Clock.After(delay)
		select {
		case <-ctx.Done():
			return nil, lastErr
		case <-timer:
		}
	}
	return nil, lastErr
}

// backoff: Retry-After wins when positive; otherwise exponential with cap.
func (r RetryPolicy) backoff(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		if retryAfter > r.MaxDelay {
			return r.MaxDelay
		}
		return retryAfter
	}
	shift := attempt - 1
	if shift > 8 {
		shift = 8
	}
	delay := time.Duration(float64(r.BaseDelay) * math.Pow(2, float64(shift)))
	if delay > r.MaxDelay {
		delay = r.MaxDelay
	}
	return delay
}

// RecordingClock is a deterministic test clock: every After call is
// answered immediately and the requested delay is recorded for assertions.
type RecordingClock struct {
	NowT   time.Time
	Delays []time.Duration
}

func NewRecordingClock() *RecordingClock {
	return &RecordingClock{NowT: time.Unix(0, 0)}
}

func (c *RecordingClock) Now() time.Time { return c.NowT }

func (c *RecordingClock) After(d time.Duration) <-chan time.Time {
	c.Delays = append(c.Delays, d)
	c.NowT = c.NowT.Add(d)
	out := make(chan time.Time, 1)
	out <- c.NowT
	return out
}
