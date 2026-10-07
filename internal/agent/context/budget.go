// Package context implements the W14 context layer (P7-04, AI-03): the
// frozen contextBudget.ts formulas (§5.3), the token estimator heuristic
// and the 413 retry decision — all pure, no I/O.
package context

import (
	"math"
	"strings"
)

// Frozen constants from contextBudget.ts.
const (
	CompactionPromptReserve    = 150
	AutoCompactBufferCap       = 15_000
	AutoCompactBufferRatio     = 0.8
	CompactionSummaryMaxOutput = 1600
	DefaultMaxOutputTokens     = 4096
	minOutputReserve           = 256
	maxOutputShareOfWindow     = 0.25
	charsPerTokenFallback      = 4
)

// EstimatorKind selects the chars-per-token heuristic.
type EstimatorKind string

const (
	EstimatorCharsDiv4       EstimatorKind = "chars-div-4"
	EstimatorOpenAIHeuristic EstimatorKind = "openai-heuristic"
	EstimatorAnthropic       EstimatorKind = "anthropic-heuristic"
	EstimatorGoogle          EstimatorKind = "google-heuristic"
)

// ResolveEstimatorKind maps a provider id to its heuristic kind.
func ResolveEstimatorKind(providerID string) EstimatorKind {
	id := strings.ToLower(providerID)
	switch {
	case strings.Contains(id, "openai"), strings.Contains(id, "gpt"):
		return EstimatorOpenAIHeuristic
	case strings.Contains(id, "anthropic"), strings.Contains(id, "claude"):
		return EstimatorAnthropic
	case strings.Contains(id, "google"), strings.Contains(id, "gemini"):
		return EstimatorGoogle
	default:
		return EstimatorCharsDiv4
	}
}

func applyHeuristicMultiplier(chars int, kind EstimatorKind) int {
	if chars < 0 {
		chars = 0
	}
	var divisor float64
	switch kind {
	case EstimatorOpenAIHeuristic:
		divisor = 3.5
	case EstimatorAnthropic:
		divisor = 3.2
	case EstimatorGoogle:
		divisor = 3.8
	default:
		divisor = charsPerTokenFallback
	}
	return int(math.Ceil(float64(chars) / divisor))
}

// EstimateTextTokens estimates tokens for one text body. The length is
// counted in UTF-16 code units to match the JS String.length baseline.
func EstimateTextTokens(text string, kind EstimatorKind) int {
	return applyHeuristicMultiplier(utf16Len(text), kind)
}

// EstimateUnknownTokens estimates tokens for a value by its serialized
// UTF-16 unit length (callers pass the JSON string length).
func EstimateUnknownTokens(serializedUnits int, kind EstimatorKind) int {
	return applyHeuristicMultiplier(serializedUnits, kind)
}

// MessagePart is one message's role and content as plain strings; callers
// serialize structured content before estimating.
type MessagePart struct {
	Role    string
	Content string
}

// EstimateMessagesTokens estimates tokens across messages: role chars +
// content chars per message.
func EstimateMessagesTokens(messages []MessagePart, kind EstimatorKind) int {
	chars := 0
	for _, message := range messages {
		chars += utf16Len(message.Role) + utf16Len(message.Content)
	}
	return applyHeuristicMultiplier(chars, kind)
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16RuneLen(r)
	}
	return n
}

func utf16RuneLen(r rune) int {
	if r > 0xFFFF {
		return 2
	}
	return 1
}

// ResolveEffectiveMaxOutputTokens ports resolveEffectiveMaxOutputTokens:
// window <= 0 returns the configured output directly (separate fixture —
// the positive-window branch must not apply); positive windows cap the
// configured output at max(256, floor(window*0.25)).
func ResolveEffectiveMaxOutputTokens(contextWindow, maxOutputTokens int) int {
	if maxOutputTokens <= 0 {
		maxOutputTokens = DefaultMaxOutputTokens
	}
	if contextWindow <= 0 {
		return maxOutputTokens
	}
	cappedByWindow := max(minOutputReserve, contextWindow/4)
	return min(maxOutputTokens, cappedByWindow)
}

// ComputeCompactionBuffer ports computeCompactionBuffer: ceil((1-0.8) *
// remaining), floored at maxOutputTokens, capped at AutoCompactBufferCap.
func ComputeCompactionBuffer(contextWindow, maxOutputTokens int) int {
	remaining := max(0, contextWindow-maxOutputTokens)
	ratioBuffer := int(math.Ceil((1 - AutoCompactBufferRatio) * float64(remaining)))
	safe := max(maxOutputTokens, ratioBuffer)
	return min(safe, AutoCompactBufferCap)
}

// CompactionThresholdInput describes one threshold computation.
type CompactionThresholdInput struct {
	ContextWindow         int
	MaxOutputTokens       int // <=0 uses the default
	CompactionPromptTokes int // <=0 uses 150
}

// ComputeCompactionThreshold ports computeCompactionThreshold: window -
// effectiveOutput - buffer - promptReserve, floored at 1.
func ComputeCompactionThreshold(input CompactionThresholdInput) int {
	maxOutput := input.MaxOutputTokens
	if maxOutput <= 0 {
		maxOutput = DefaultMaxOutputTokens
	}
	promptReserve := input.CompactionPromptTokes
	if promptReserve <= 0 {
		promptReserve = CompactionPromptReserve
	}
	effective := ResolveEffectiveMaxOutputTokens(input.ContextWindow, maxOutput)
	buffer := ComputeCompactionBuffer(input.ContextWindow, effective)
	return max(1, input.ContextWindow-effective-buffer-promptReserve)
}

// TotalInputTokensInput aggregates one pre-turn estimate.
type TotalInputTokensInput struct {
	MessageTokens  int
	SystemTokens   int
	ToolTokens     int
	ReservedTokens int
}

// TotalInputTokens sums the parts (reserved floored at 0).
func TotalInputTokens(input TotalInputTokensInput) int {
	reserved := max(0, input.ReservedTokens)
	return input.MessageTokens + input.SystemTokens + input.ToolTokens + reserved
}

// ShouldCompactByBudget compares the total estimate against the threshold.
func ShouldCompactByBudget(input TotalInputTokensInput, contextWindow, maxOutputTokens, forceThreshold int) bool {
	total := TotalInputTokens(input)
	threshold := forceThreshold
	if threshold <= 0 {
		threshold = ComputeCompactionThreshold(CompactionThresholdInput{
			ContextWindow:   contextWindow,
			MaxOutputTokens: maxOutputTokens,
		})
	}
	return total >= threshold
}
