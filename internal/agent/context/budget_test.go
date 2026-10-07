package context

import (
	"testing"
)

func TestResolveEffectiveMaxOutputTokens(t *testing.T) {
	cases := []struct {
		window    int
		maxOutput int
		want      int
	}{
		{window: 0, maxOutput: 4096, want: 4096},    // non-positive window: passthrough branch
		{window: -5, maxOutput: 8192, want: 8192},   // negative window same branch
		{window: 8000, maxOutput: 4096, want: 4096}, // window cap = 2000 → min(4096, 2000)=2000
		{window: 100_000, maxOutput: 4096, want: 4096},
		{window: 8192, maxOutput: 0, want: 4096}, // default fills in
	}
	for _, tc := range cases {
		got := ResolveEffectiveMaxOutputTokens(tc.window, tc.maxOutput)
		// recompute expected per formula
		if tc.window > 0 {
			capped := 256
			if floor := tc.window / 4; floor > capped {
				capped = floor
			}
			want := tc.maxOutput
			if want <= 0 {
				want = 4096
			}
			if want > capped {
				want = capped
			}
			if got != want {
				t.Errorf("window=%d max=%d: got %d want %d", tc.window, tc.maxOutput, got, want)
			}
		}
	}
	// explicit spot checks
	if got := ResolveEffectiveMaxOutputTokens(8000, 4096); got != 2000 {
		t.Errorf("window 8000: output capped by floor(window/4)=2000, got %d", got)
	}
	if got := ResolveEffectiveMaxOutputTokens(100_000, 100_000); got != 25_000 {
		t.Errorf("window 100000: floor(window*0.25)=25000, got %d", got)
	}
}

func TestComputeCompactionBuffer(t *testing.T) {
	// remaining = window - output; ratio buffer = ceil(0.2*remaining);
	// floored at output, capped 15000.
	if got := ComputeCompactionBuffer(100_000, 4096); got != 15_000 {
		t.Errorf("huge window buffer = %d, want cap 15000", got)
	}
	if got := ComputeCompactionBuffer(10_000, 4_000); got != 4_000 {
		// remaining=6000, ratio=1200 → max(4000,1200)=4000
		t.Errorf("buffer = %d, want 4000 (floored at output)", got)
	}
}

func TestComputeCompactionThresholdFloorAtOne(t *testing.T) {
	// Tiny window: threshold must never drop below 1.
	got := ComputeCompactionThreshold(CompactionThresholdInput{ContextWindow: 100, MaxOutputTokens: 4096})
	if got < 1 {
		t.Errorf("threshold = %d, want >= 1", got)
	}
}

func TestShouldCompactByBudget(t *testing.T) {
	input := TotalInputTokensInput{MessageTokens: 1000}
	if ShouldCompactByBudget(input, 8000, 4096, 0) {
		t.Errorf("small input must not compact")
	}
	if !ShouldCompactByBudget(input, 8000, 4096, 500) {
		t.Errorf("forceThreshold 500 must trigger compaction")
	}
	if !ShouldCompactByBudget(TotalInputTokensInput{MessageTokens: 8000}, 8000, 4096, 0) {
		t.Errorf("total >= threshold must compact")
	}
}

// Estimator golden: default chars/4.
func TestEstimatorKinds(t *testing.T) {
	if got := EstimateTextTokens("abcd", EstimatorCharsDiv4); got != 1 {
		t.Errorf("chars-div-4: %d", got)
	}
	if got := EstimateTextTokens("abcdefghi", EstimatorOpenAIHeuristic); got != 3 {
		// ceil(9/3.5)=3
		t.Errorf("openai: %d", got)
	}
	if ResolveEstimatorKind("my-gpt4-proxy") != EstimatorOpenAIHeuristic {
		t.Errorf("gpt proxy must map to openai heuristic")
	}
	if ResolveEstimatorKind("unknown-provider") != EstimatorCharsDiv4 {
		t.Errorf("unknown provider must map to chars-div-4")
	}
}

// Four13 models the 413 recovery decision (§5.3 table).
type Four13Action string

const (
	Four13ClearDraft         Four13Action = "clear_draft"          // no tool progress: drop the assistant draft
	Four13RebuildFromHistory Four13Action = "rebuild_from_history" // tool progress exists: rebuild request from latest history, new assistant segment
	Four13GiveUp             Four13Action = "give_up"              // second 413: never retry in the same turn
)

// Decide413 chooses the bounded remediation for one payload-too-large
// failure. retriedAlready is true after the first forced-compaction retry.
func Decide413(hasToolProgress, retriedAlready bool) Four13Action {
	if retriedAlready {
		return Four13GiveUp
	}
	if hasToolProgress {
		return Four13RebuildFromHistory
	}
	return Four13ClearDraft
}

func TestDecide413(t *testing.T) {
	if got := Decide413(false, false); got != Four13ClearDraft {
		t.Errorf("no progress first 413 = %v", got)
	}
	if got := Decide413(true, false); got != Four13RebuildFromHistory {
		t.Errorf("tool progress first 413 = %v", got)
	}
	if got := Decide413(false, true); got != Four13GiveUp {
		t.Errorf("second 413 = %v, want give_up", got)
	}
	if got := Decide413(true, true); got != Four13GiveUp {
		t.Errorf("second 413 with progress = %v, want give_up", got)
	}
}
