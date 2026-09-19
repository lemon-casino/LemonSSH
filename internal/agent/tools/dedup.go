package tools

import (
	"fmt"
	"strings"
)

// DedupEntry is one remembered tool result identity.
type DedupEntry struct {
	Fingerprint string
	ToolName    string
	TurnNumber  int
	Preview     string
}

// ToolResultDedup is the per-turn result deduper (port of
// toolResultDedup.ts). beginTurn advances the turn and clears per-turn
// state so results from earlier turns are never deduplicated against
// (design §W14). Write replay is a separate, explicitly enabled ledger:
// remembered results are reused for identical confirmed-execution
// identities only, never as re-execution authorization.
type ToolResultDedup struct {
	turnNumber int
	cache      map[string]DedupEntry
	consumed   map[string]int
	completed  map[string][]string // fingerprint -> serialized results
	replayable map[string][]string
	jobSession map[string]string
	replayOn   bool
}

func NewToolResultDedup() *ToolResultDedup {
	return &ToolResultDedup{
		cache:      map[string]DedupEntry{},
		consumed:   map[string]int{},
		completed:  map[string][]string{},
		replayable: map[string][]string{},
		jobSession: map[string]string{},
	}
}

// BeginTurn advances the turn counter and resets per-turn ledgers.
func (d *ToolResultDedup) BeginTurn() {
	d.turnNumber++
	d.consumed = map[string]int{}
	d.completed = map[string][]string{}
	d.replayable = map[string][]string{}
	d.replayOn = false
}

// Reset clears everything (new chat session).
func (d *ToolResultDedup) Reset() {
	d.turnNumber = 0
	d.cache = map[string]DedupEntry{}
	d.consumed = map[string]int{}
	d.completed = map[string][]string{}
	d.replayable = map[string][]string{}
	d.jobSession = map[string]string{}
	d.replayOn = false
}

// TurnNumber reports the current turn.
func (d *ToolResultDedup) TurnNumber() int { return d.turnNumber }

// RememberCompletedWrite records a confirmed write result by fingerprint.
func (d *ToolResultDedup) RememberCompletedWrite(fingerprint, serializedResult string) {
	d.completed[fingerprint] = append(d.completed[fingerprint], serializedResult)
}

// RememberTerminalJobSession binds a terminal job to its session.
func (d *ToolResultDedup) RememberTerminalJobSession(jobID, sessionID string) {
	d.jobSession[jobID] = sessionID
}

// TerminalSessionForJob resolves the session a job ran on.
func (d *ToolResultDedup) TerminalSessionForJob(jobID string) (string, bool) {
	sessionID, ok := d.jobSession[jobID]
	return sessionID, ok
}

// EnableWriteReplay snapshots confirmed writes for replay (413 recovery:
// rebuild-from-history may reuse results of already-executed writes whose
// fingerprints were preserved).
func (d *ToolResultDedup) EnableWriteReplay(preservedFingerprints []string) {
	d.replayable = map[string][]string{}
	for fingerprint, results := range d.completed {
		copied := make([]string, len(results))
		copy(copied, results)
		d.replayable[fingerprint] = copied
	}
	d.replayOn = true
	for _, fingerprint := range preservedFingerprints {
		d.consumeReplay(fingerprint)
	}
}

// ReplayEnabled reports whether replay is active.
func (d *ToolResultDedup) ReplayEnabled() bool { return d.replayOn }

// ReplayCompletedWrite consumes one replayable result by fingerprint;
// nil/"" when replay is off or the fingerprint is exhausted.
func (d *ToolResultDedup) ReplayCompletedWrite(fingerprint string) (string, bool) {
	if !d.replayOn {
		return "", false
	}
	results := d.replayable[fingerprint]
	if len(results) == 0 {
		return "", false
	}
	first := results[0]
	rest := results[1:]
	if len(rest) == 0 {
		delete(d.replayable, fingerprint)
	} else {
		d.replayable[fingerprint] = rest
	}
	return first, true
}

func (d *ToolResultDedup) consumeReplay(fingerprint string) (string, bool) {
	results := d.replayable[fingerprint]
	if len(results) == 0 {
		return "", false
	}
	first := results[0]
	rest := results[1:]
	if len(rest) == 0 {
		delete(d.replayable, fingerprint)
	} else {
		d.replayable[fingerprint] = rest
	}
	return first, true
}

// TakeBudget grants up to requested units for a key within the limit,
// tracking cumulative consumption across calls in the turn.
func (d *ToolResultDedup) TakeBudget(key string, requested, limit int) int {
	consumed := d.consumed[key]
	granted := requested
	if granted > limit-consumed {
		granted = limit - consumed
	}
	if granted < 0 {
		granted = 0
	}
	d.consumed[key] = consumed + granted
	return granted
}

// FingerprintFor builds the canonical fingerprint for a tool call key.
func FingerprintFor(toolName, key string) string {
	return toolName + ":" + key
}

// Check returns the remembered entry for a fingerprint, if any.
func (d *ToolResultDedup) Check(fingerprint string) (DedupEntry, bool) {
	entry, ok := d.cache[fingerprint]
	return entry, ok
}

// Remember records a result identity for the current turn.
func (d *ToolResultDedup) Remember(toolName, fingerprint, preview string) {
	d.cache[fingerprint] = DedupEntry{
		Fingerprint: fingerprint,
		ToolName:    toolName,
		TurnNumber:  d.turnNumber,
		Preview:     truncate(preview, 160),
	}
}

// BuildCachedNotice renders the cached-result notice.
func BuildCachedNotice(entry DedupEntry) string {
	return fmt.Sprintf("[cached] same as turn %d for %s", entry.TurnNumber, entry.ToolName)
}

// BuildTerminalWriteFingerprint builds the write fingerprint for terminal
// execution/start calls; ok=false when sessionId or command is missing.
func BuildTerminalWriteFingerprint(toolName, chatSessionID string, sessionID, command any) (string, bool) {
	sid, ok1 := sessionID.(string)
	cmd, ok2 := command.(string)
	if !ok1 || !ok2 || sid == "" || cmd == "" {
		return "", false
	}
	name := toolName
	if toolName == "terminal_start" {
		name = "terminal.start:write"
	}
	return name + ":" + hashScopeKey([]string{chatSessionID, sid, cmd}), true
}

func hashScopeKey(parts []string) string {
	var filtered []string
	for _, part := range parts {
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, "|")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
