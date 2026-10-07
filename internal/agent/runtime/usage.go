package runtime

import (
	"sync"
	"time"
)

// UsageRecord is one usage observation (design §5.4). Counts stay per
// observation: the ledger never sums step deltas into a turn total and
// never fabricates zeros when a provider omits usage.
type UsageRecord struct {
	Source       string // vendor/provider family (openai|anthropic|google|fixture)
	TurnID       string
	ModelCallID  string
	CountType    string // delta | cumulative | final
	InputTokens  int64
	OutputTokens int64
	CacheRead    int64
	CacheWrite   int64
	Reasoning    int64
	Complete     bool
	ObservedAtMS int64
}

// UsageLedger accumulates usage observations per turn. The turn total is
// authoritative only after a final observation (providers.completion
// usage); step deltas are never re-summed on top of cumulative totals.
type UsageLedger struct {
	mu     sync.Mutex
	byTurn map[string]*turnUsage
	now    func() time.Time
}

type turnUsage struct {
	input, output, cacheRead, cacheWrite, reasoning int64
	complete                                        bool
	sealed                                          bool
	observations                                    int
}

func NewUsageLedger() *UsageLedger {
	return &UsageLedger{byTurn: map[string]*turnUsage{}, now: time.Now}
}

// Observe applies one usage observation. countType: delta adds,
// cumulative overwrites, final overwrites and seals the turn.
func (l *UsageLedger) Observe(turnID string, source string, countType string, input, output, cacheRead, cacheWrite, reasoning int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	state := l.byTurn[turnID]
	if state == nil {
		state = &turnUsage{}
		l.byTurn[turnID] = state
	}
	switch countType {
	case "delta":
		state.input += input
		state.output += output
		state.cacheRead += cacheRead
		state.cacheWrite += cacheWrite
		state.reasoning += reasoning
	case "cumulative":
		if input > state.input {
			state.input = input
		}
		if output > state.output {
			state.output = output
		}
		if cacheRead > state.cacheRead {
			state.cacheRead = cacheRead
		}
		if cacheWrite > state.cacheWrite {
			state.cacheWrite = cacheWrite
		}
		if reasoning > state.reasoning {
			state.reasoning = reasoning
		}
	case "final":
		state.input = input
		state.output = output
		state.cacheRead = cacheRead
		state.cacheWrite = cacheWrite
		state.reasoning = reasoning
		state.complete = true
	}
	state.observations++
}

// TurnTotal returns the authoritative usage for one turn; complete is
// false while observations are still partial (unknown stays unknown).
func (l *UsageLedger) TurnTotal(turnID string) (input, output, cacheRead, cacheWrite, reasoning int64, complete bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	state := l.byTurn[turnID]
	if state == nil {
		return 0, 0, 0, 0, 0, false
	}
	return state.input, state.output, state.cacheRead, state.cacheWrite, state.reasoning, state.complete
}

// ObservationCount reports how many usage observations a turn received.
func (l *UsageLedger) ObservationCount(turnID string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	state := l.byTurn[turnID]
	if state == nil {
		return 0
	}
	return state.observations
}
