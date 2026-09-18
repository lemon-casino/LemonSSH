package runtime

import (
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/binaricat/netcatty/internal/app/contracts"
)

// Ring operation failures. ConsistencyFailure pins T06: the same sequence
// redelivered with different payload bytes is a protocol violation, never
// silently overwritten.
var (
	ErrSequenceBehind   = errors.New("runtime: sequence already evicted from ring")
	ErrSequenceConflict = errors.New("runtime: sequence redelivered with different payload")
)

// DefaultRingCapacity is the per-turn event ring bound (design candidate;
// tunable). Byte budgets arrive with W14 handles; this slice bounds count.
const DefaultRingCapacity = 1024

// EventRing is one turn's bounded event log. Sequences are strictly
// increasing uint64 values carried on the wire as decimal strings.
// Identical redelivery of the newest event is idempotent; any conflict or
// evicted write fails loudly.
type EventRing struct {
	mu       sync.Mutex
	capacity int
	events   []contracts.AgentEventEnvelope // FIFO, oldest first
	maxSeq   uint64                         // 0 = empty
}

func NewEventRing(capacity int) *EventRing {
	if capacity <= 0 {
		capacity = DefaultRingCapacity
	}
	return &EventRing{capacity: capacity}
}

// Append stores one event. The envelope must carry a sequence strictly
// greater than every stored one, except a byte-identical redelivery of the
// newest event, which is ignored.
func (r *EventRing) Append(env contracts.AgentEventEnvelope) error {
	seq, err := parseSequence(env.Sequence)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.maxSeq > 0 {
		if seq < r.maxSeq {
			return ErrSequenceBehind
		}
		if seq == r.maxSeq {
			last := r.events[len(r.events)-1]
			if sameEnvelope(last, env) {
				return nil // idempotent redelivery (T06)
			}
			return ErrSequenceConflict
		}
	}

	r.events = append(r.events, env)
	r.maxSeq = seq
	if len(r.events) > r.capacity {
		evicted := len(r.events) - r.capacity
		r.events = r.events[evicted:]
	}
	return nil
}

// Read returns events strictly after the cursor, bounded by limit
// (<=0 means all retained). When the cursor points into an evicted region,
// expired is true and the caller must serve a snapshot instead (T07).
func (r *EventRing) Read(afterSequence string, limit int) (events []contracts.AgentEventEnvelope, expired bool, err error) {
	after, err := parseSequence(afterSequence)
	if err != nil {
		if afterSequence == "" {
			after = 0
			err = nil
		} else {
			return nil, false, err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.maxSeq == 0 {
		return nil, false, nil
	}
	oldest := sequenceOf(r.events[0])
	// A cursor exactly at oldest-1 has seen everything evicted; only a
	// cursor below that missed retained-region events (T07).
	if after > 0 && after < oldest-1 {
		return nil, true, nil
	}

	for _, env := range r.events {
		if sequenceOf(env) <= after {
			continue
		}
		if limit > 0 && len(events) >= limit {
			break
		}
		events = append(events, env)
	}
	return events, false, nil
}

// ThroughSequence reports the last appended sequence as a decimal string
// ("0" for an empty ring).
func (r *EventRing) ThroughSequence() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.maxSeq == 0 {
		return "0"
	}
	return strconv.FormatUint(r.maxSeq, 10)
}

// hasMore reports whether the ring holds events strictly after lastSeq.
func (r *EventRing) hasMore(lastSeq string) bool {
	last, err := parseSequence(lastSeq)
	if err != nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events) > 0 && sequenceOf(r.events[len(r.events)-1]) > last
}

func parseSequence(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	return strconv.ParseUint(s, 10, 64)
}

func sequenceOf(env contracts.AgentEventEnvelope) uint64 {
	seq, _ := strconv.ParseUint(env.Sequence, 10, 64)
	return seq
}

// sameEnvelope compares wire-visible fields, not timestamps: redelivery of
// the same logical event with a re-stamped clock is still idempotent.
func sameEnvelope(a, b contracts.AgentEventEnvelope) bool {
	return a.Sequence == b.Sequence &&
		a.Type == b.Type &&
		a.MessageID == b.MessageID &&
		a.ModelCallID == b.ModelCallID &&
		a.ToolCallID == b.ToolCallID &&
		string(a.Payload) == string(b.Payload)
}
