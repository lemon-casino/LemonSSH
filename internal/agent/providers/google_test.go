package providers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestGoogleStreamFullTurn(t *testing.T) {
	raw := loadFixture(t, "google-generatecontent.sse")
	stream := NewGoogleStream()
	parser := &SSEParser{}

	var events []Event
	for len(raw) > 0 {
		size := 7
		if size > len(raw) {
			size = len(raw)
		}
		for _, sse := range parser.Feed(raw[:size]) {
			unified, err := stream.Consume(sse)
			if err != nil {
				t.Fatalf("consume: %v", err)
			}
			events = append(events, unified...)
		}
		raw = raw[size:]
	}

	var text strings.Builder
	var reasoning strings.Builder
	var usageCount int
	var usage *Usage
	var finish FinishReason
	toolStarts, toolDeltas := 0, 0
	var args strings.Builder
	for _, ev := range events {
		switch ev.Kind {
		case EventTextDelta:
			text.WriteString(ev.Text)
		case EventReasoningDelta:
			reasoning.WriteString(ev.Text)
		case EventUsage:
			usageCount++
			usage = &ev.Usage
		case EventFinish:
			finish = ev.Finish
		case EventToolCallStart:
			toolStarts++
			if ev.ToolName != "sftp_list" {
				t.Errorf("tool name = %q", ev.ToolName)
			}
		case EventToolCallDelta:
			toolDeltas++
			args.WriteString(ev.ArgumentsDelta)
		}
	}

	if text.String() != "来自 Gemini ok" {
		t.Errorf("text = %q", text.String())
	}
	if reasoning.String() != "thinking hard" {
		t.Errorf("reasoning = %q", reasoning.String())
	}
	if toolStarts != 1 || toolDeltas != 1 {
		t.Errorf("tool events: starts=%d deltas=%d", toolStarts, toolDeltas)
	}
	if args.String() != `{"sessionId":"s1"}` {
		t.Errorf("arguments = %q", args.String())
	}
	if finish != FinishStop {
		t.Errorf("finish = %q", finish)
	}
	// Cumulative usageMetadata must yield exactly one observation carrying
	// the final counts (T22: never double-counted, never zero-filled).
	if usageCount != 1 {
		t.Fatalf("usage emitted %d times, want 1", usageCount)
	}
	if usage == nil || usage.InputTokens != 30 || usage.OutputTokens != 12 ||
		usage.CacheReadTokens != 4 || usage.ReasoningTokens != 6 || !usage.Known {
		t.Errorf("usage = %+v", usage)
	}
}

func TestGoogleStreamError(t *testing.T) {
	stream := NewGoogleStream()
	_, err := stream.Consume(SSEEvent{Data: `{"error":{"code":429,"message":"Quota exceeded","status":"RESOURCE_EXHAUSTED"}}`})
	var streamErr *StreamError
	if !errors.As(err, &streamErr) || streamErr.Code != "RESOURCE_EXHAUSTED" {
		t.Fatalf("expected StreamError RESOURCE_EXHAUSTED, got %v", err)
	}
}

func TestRetrySingleOwnerHonorsRetryAfter(t *testing.T) {
	clock := NewRecordingClock()
	policy := RetryPolicy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: 10 * time.Second, Clock: clock}

	attempts := 0
	value, err := policy.Do(context.Background(), func(ctx context.Context, attempt int) (any, AttemptOutcome) {
		attempts++
		if attempt < 3 {
			// Upstream asked for 2s; SDK-style extra backoff must NOT stack
			// on top of it (T23).
			return nil, AttemptOutcome{Retryable: true, RetryAfter: 2 * time.Second, Err: errors.New("429")}
		}
		return "ok", AttemptOutcome{}
	})
	if err != nil || value != "ok" {
		t.Fatalf("retry chain failed: %v, %v", value, err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d", attempts)
	}
	if len(clock.Delays) != 2 {
		t.Fatalf("delays = %v, want 2 waits", clock.Delays)
	}
	for _, delay := range clock.Delays {
		if delay != 2*time.Second {
			t.Errorf("Retry-After must be honored exactly, got %v", delay)
		}
	}
}

func TestRetryExponentialBackoffCapped(t *testing.T) {
	clock := NewRecordingClock()
	policy := RetryPolicy{MaxAttempts: 4, BaseDelay: time.Second, MaxDelay: 3 * time.Second, Clock: clock}

	attempts := 0
	_, err := policy.Do(context.Background(), func(ctx context.Context, attempt int) (any, AttemptOutcome) {
		attempts++
		return nil, AttemptOutcome{Retryable: true, Err: errors.New("503")}
	})
	if err == nil {
		t.Fatal("exhausted retries must surface the error")
	}
	if attempts != 4 {
		t.Errorf("attempts = %d, want 4", attempts)
	}
	want := []time.Duration{time.Second, 2 * time.Second, 3 * time.Second} // capped at MaxDelay
	if len(clock.Delays) != len(want) {
		t.Fatalf("delays = %v", clock.Delays)
	}
	for i, delay := range clock.Delays {
		if delay != want[i] {
			t.Errorf("delay[%d] = %v, want %v", i, delay, want[i])
		}
	}
}

func TestRetryNonRetryableFailsImmediately(t *testing.T) {
	clock := NewRecordingClock()
	policy := RetryPolicy{MaxAttempts: 5, Clock: clock}
	attempts := 0
	_, err := policy.Do(context.Background(), func(ctx context.Context, attempt int) (any, AttemptOutcome) {
		attempts++
		return nil, AttemptOutcome{Err: errors.New("invalid request")}
	})
	if err == nil || attempts != 1 {
		t.Errorf("non-retryable must fail on first attempt, attempts=%d err=%v", attempts, err)
	}
	if len(clock.Delays) != 0 {
		t.Errorf("no wait expected, got %v", clock.Delays)
	}
}
