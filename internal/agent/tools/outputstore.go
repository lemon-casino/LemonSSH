// Package tools implements the W14 tool-output layer (P7-04, AI-03): the
// bounded handle store that backs harness.tool_output.read. Units are
// UTF-16 code units, matching the frozen Electron baseline
// (toolOutputStore.ts) — the design forbids silently switching units
// (§6.3). Expired and not-found handles are distinct results; truncated
// content never masquerades as complete output.
package tools

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

// Frozen baseline limits (toolOutputStore.ts), in UTF-16 code units
// unless stated otherwise.
const (
	ReadMaxChars         = 12_000
	MaxHandleChars       = 4_000_000
	MaxHandlesPerSession = 64
	MaxCharsPerSession   = 8_000_000
	MaxHandlesGlobal     = 256
	MaxCharsGlobal       = 32_000_000
	MaxClosedTerminals   = 1_024
	TTL                  = 30 * time.Minute
	DefaultPreviewChars  = 240
	SearchMaxMatches     = 20
)

// ReadMode selects which slice of the stored content a read returns.
type ReadMode string

const (
	ModeHead   ReadMode = "head"
	ModeTail   ReadMode = "tail"
	ModeRange  ReadMode = "range"
	ModeSearch ReadMode = "search"
	ModeFull   ReadMode = "full" // bounded: capped at MaxHandleChars units
)

// Typed outcomes. Expired and not-found are distinct (design §6.3: no
// truncated-content masquerade).
var (
	ErrHandleNotFound = fmt.Errorf("tool output handle not found")
	ErrHandleExpired  = fmt.Errorf("tool output handle expired")
	ErrScopeDenied    = fmt.Errorf("tool output handle belongs to another chat session")
	ErrOffsetRange    = fmt.Errorf("tool output offset out of range")
	ErrEvicted        = fmt.Errorf("tool output handle was evicted")
)

// UTF16Len counts UTF-16 code units of a Go string.
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// TruncateUTF16 cuts s to at most maxUnits UTF-16 code units without
// splitting a surrogate pair.
func TruncateUTF16(s string, maxUnits int) string {
	if maxUnits <= 0 {
		return ""
	}
	n := 0
	for index, r := range s {
		units := utf16.RuneLen(r)
		if n+units > maxUnits {
			return s[:index]
		}
		n += units
	}
	return s
}

// UTF16Slice returns the substring between UTF-16 unit offsets, adjusting
// unit boundaries to rune boundaries.
func UTF16Slice(s string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	unit := 0
	startByte, endByte := len(s), len(s)
	for index, r := range s {
		if unit >= start && startByte == len(s) {
			startByte = index
		}
		units := utf16.RuneLen(r)
		if unit >= end {
			endByte = index
			break
		}
		unit += units
	}
	return s[startByte:endByte]
}

// Handle is one stored tool output.
type Handle struct {
	ID              string
	ChatSessionID   string
	CapabilityID    string
	SessionID       string // terminal session, may be empty
	TotalUnits      int
	StoredUnits     int
	SourceTruncated bool
	Preview         string
	StoredAt        time.Time
	AccessedAt      time.Time
	Evicted         bool

	content string
	spilled bool
}

// PreviewUnits is the preview length constant for external readers.
const PreviewUnits = DefaultPreviewChars

// SpillSink receives oversized handles for durable storage. Production
// wires the Netcatty temp manager — never os.TempDir (workspace rule).
// nil or a failing sink keeps the bounded in-memory copy.
type SpillSink interface {
	Spill(handleID string, content string) (string, error)
}

// StoreOptions bounds one store.
type StoreOptions struct {
	MaxHandleChars       int
	MaxHandlesPerSession int
	MaxCharsPerSession   int
	MaxHandlesGlobal     int
	MaxCharsGlobal       int
	TTL                  time.Duration
	Now                  func() time.Time
	Spill                SpillSink
}

func (o StoreOptions) withDefaults() StoreOptions {
	if o.MaxHandleChars <= 0 {
		o.MaxHandleChars = MaxHandleChars
	}
	if o.MaxHandlesPerSession <= 0 {
		o.MaxHandlesPerSession = MaxHandlesPerSession
	}
	if o.MaxCharsPerSession <= 0 {
		o.MaxCharsPerSession = MaxCharsPerSession
	}
	if o.MaxHandlesGlobal <= 0 {
		o.MaxHandlesGlobal = MaxHandlesGlobal
	}
	if o.MaxCharsGlobal <= 0 {
		o.MaxCharsGlobal = MaxCharsGlobal
	}
	if o.TTL <= 0 {
		o.TTL = TTL
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return o
}

// OutputStore is the bounded, chat-scoped handle store.
type OutputStore struct {
	mu         sync.Mutex
	options    StoreOptions
	bySession  map[string]map[string]*Handle
	closedTerm map[string]bool // bounded closed-terminal deny set
	denyChat   []string        // chat deletion deny list
}

func NewOutputStore(options StoreOptions) *OutputStore {
	return &OutputStore{
		options:    options.withDefaults(),
		bySession:  map[string]map[string]*Handle{},
		closedTerm: map[string]bool{},
	}
}

// StoreInput describes one store request.
type StoreInput struct {
	Content       string
	ChatSessionID string
	CapabilityID  string
	SessionID     string
	PreviewChars  int
}

// StoreInputResult reports what was retained.
type StoreInputResult struct {
	ID              string
	ChatSessionID   string
	TotalUnits      int
	StoredUnits     int
	SourceTruncated bool
	Preview         string
	SpillPath       string
}

// Store retains bounded content under a fresh unpredictable handle ID.
// Content truncation keeps whole runes within the UTF-16 unit budget.
func (s *OutputStore) Store(input StoreInput) StoreInputResult {
	s.options = s.options.withDefaults()
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.options.Now()

	if s.lifecycleDenied(input.ChatSessionID, input.SessionID) {
		id := newHandleID()
		return StoreInputResult{
			ID:              id,
			ChatSessionID:   input.ChatSessionID,
			TotalUnits:      UTF16Len(input.Content),
			SourceTruncated: input.Content != "",
		}
	}

	retained := TruncateUTF16(input.Content, s.options.MaxHandleChars)
	totalUnits := UTF16Len(input.Content)
	previewChars := input.PreviewChars
	if previewChars <= 0 {
		previewChars = DefaultPreviewChars
	}
	handle := &Handle{
		ID:              newHandleID(),
		ChatSessionID:   input.ChatSessionID,
		CapabilityID:    input.CapabilityID,
		SessionID:       input.SessionID,
		TotalUnits:      totalUnits,
		StoredUnits:     UTF16Len(retained),
		SourceTruncated: totalUnits > UTF16Len(retained),
		Preview:         TruncateUTF16(retained, previewChars),
		StoredAt:        now,
		AccessedAt:      now,
		content:         retained,
	}

	sessionMap := s.bySession[input.ChatSessionID]
	if sessionMap == nil {
		sessionMap = map[string]*Handle{}
		s.bySession[input.ChatSessionID] = sessionMap
	}
	sessionMap[handle.ID] = handle
	s.enforceSessionLimits(input.ChatSessionID, now)
	s.enforceGlobalLimits(now)

	result := StoreInputResult{
		ID:              handle.ID,
		ChatSessionID:   handle.ChatSessionID,
		TotalUnits:      handle.TotalUnits,
		StoredUnits:     handle.StoredUnits,
		SourceTruncated: handle.SourceTruncated,
		Preview:         handle.Preview,
	}
	// Spill threshold is 0 (frozen baseline): every stored handle attempts
	// async spill when persistence is configured; failures keep the
	// bounded in-memory copy.
	if s.options.Spill != nil {
		if path, err := s.options.Spill.Spill(handle.ID, handle.content); err == nil {
			handle.spilled = true
			result.SpillPath = path
		}
	}
	return result
}

// ReadOptions selects the slice to return.
type ReadOptions struct {
	HandleID      string
	ChatSessionID string
	Mode          ReadMode
	Offset        int
	MaxChars      int
	Query         string
}

// ReadResult is one read outcome. Content is "" unless a slice applies.
type ReadResult struct {
	Content    string
	TotalUnits int
	Status     string // stored | truncated | evicted
	Matches    []string
}

// Read serves one bounded read after validating owner scope. Cross-chat
// reads are denied; expired handles are distinct from not-found.
func (s *OutputStore) Read(opts ReadOptions) (ReadResult, error) {
	s.options = s.options.withDefaults()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked()

	handle := s.findLocked(opts.HandleID, opts.ChatSessionID)
	if handle == nil {
		// Distinguish expired from never-known by scanning without chat.
		if h := s.findAnyLocked(opts.HandleID); h != nil {
			return ReadResult{}, ErrScopeDenied
		}
		return ReadResult{}, ErrHandleNotFound
	}
	if handle.Evicted {
		return ReadResult{TotalUnits: handle.TotalUnits, Status: "evicted"}, ErrEvicted
	}
	if opts.ChatSessionID != "" && handle.ChatSessionID != opts.ChatSessionID {
		return ReadResult{}, ErrScopeDenied
	}
	handle.AccessedAt = s.options.Now()

	if opts.MaxChars <= 0 {
		opts.MaxChars = ReadMaxChars
	}
	content := handle.content

	switch opts.Mode {
	case ModeSearch:
		if opts.Query == "" {
			return ReadResult{}, ErrOffsetRange
		}
		return ReadResult{Matches: searchContents(content, opts.Query), TotalUnits: handle.TotalUnits, Status: "stored"}, nil
	case ModeTail:
		tail := UTF16Slice(content, UTF16Len(content)-opts.MaxChars, UTF16Len(content))
		return ReadResult{Content: tail, TotalUnits: handle.TotalUnits, Status: "stored"}, nil
	case ModeRange:
		slice := UTF16Slice(content, opts.Offset, opts.Offset+opts.MaxChars)
		return ReadResult{Content: slice, TotalUnits: handle.TotalUnits, Status: "stored"}, nil
	case ModeFull:
		return ReadResult{Content: TruncateUTF16(content, opts.MaxChars), TotalUnits: handle.TotalUnits, Status: "stored"}, nil
	default: // head
		return ReadResult{Content: UTF16Slice(content, 0, opts.MaxChars), TotalUnits: handle.TotalUnits, Status: "stored"}, nil
	}
}

// searchContents returns newline-joined matches capped at SearchMaxMatches.
func searchContents(content, query string) []string {
	var matches []string
	remaining := content
	for len(matches) < SearchMaxMatches {
		index := strings.Index(remaining, query)
		if index < 0 {
			break
		}
		start := index - 40
		if start < 0 {
			start = 0
		}
		end := index + len(query) + 40
		if end > len(remaining) {
			end = len(remaining)
		}
		matches = append(matches, remaining[start:end])
		remaining = remaining[index+len(query):]
	}
	return matches
}

func (s *OutputStore) findLocked(handleID, chatSessionID string) *Handle {
	if chatSessionID != "" {
		return s.bySession[chatSessionID][handleID]
	}
	return s.findAnyLocked(handleID)
}

func (s *OutputStore) findAnyLocked(handleID string) *Handle {
	for _, sessionMap := range s.bySession {
		if handle := sessionMap[handleID]; handle != nil {
			return handle
		}
	}
	return nil
}

// pruneExpiredLocked drops handles past the TTL (accessedAt driven).
func (s *OutputStore) pruneExpiredLocked() {
	cutoff := s.options.Now().Add(-s.options.TTL)
	for chatID, sessionMap := range s.bySession {
		for id, handle := range sessionMap {
			if handle.AccessedAt.Before(cutoff) {
				delete(sessionMap, id)
				if len(sessionMap) == 0 {
					delete(s.bySession, chatID)
				}
			}
		}
	}
}

// enforceSessionLimits evicts least-recently-accessed handles when the
// per-session count or unit budget is exceeded.
func (s *OutputStore) enforceSessionLimits(chatID string, now time.Time) {
	sessionMap := s.bySession[chatID]
	for len(sessionMap) > s.options.MaxHandlesPerSession || s.sessionUnits(sessionMap) > s.options.MaxCharsPerSession {
		oldest := s.oldestHandle(sessionMap)
		if oldest == nil {
			return
		}
		oldest.Evicted = true
		delete(sessionMap, oldest.ID)
		if len(sessionMap) == 0 {
			delete(s.bySession, chatID)
			return
		}
	}
}

func (s *OutputStore) sessionUnits(sessionMap map[string]*Handle) int {
	total := 0
	for _, handle := range sessionMap {
		total += handle.StoredUnits
	}
	return total
}

func (s *OutputStore) oldestHandle(sessionMap map[string]*Handle) *Handle {
	var oldest *Handle
	for _, handle := range sessionMap {
		if oldest == nil || handle.AccessedAt.Before(oldest.AccessedAt) {
			oldest = handle
		}
	}
	return oldest
}

// enforceGlobalLimits evicts across sessions when the global budgets are
// exceeded.
func (s *OutputStore) enforceGlobalLimits(now time.Time) {
	totalHandles, totalUnits := 0, 0
	for _, sessionMap := range s.bySession {
		totalHandles += len(sessionMap)
		totalUnits += s.sessionUnits(sessionMap)
	}
	for totalHandles > s.options.MaxHandlesGlobal || totalUnits > s.options.MaxCharsGlobal {
		var oldest *Handle
		var oldestChat string
		for chatID, sessionMap := range s.bySession {
			for _, handle := range sessionMap {
				if oldest == nil || handle.AccessedAt.Before(oldest.AccessedAt) {
					oldest = handle
					oldestChat = chatID
				}
			}
		}
		if oldest == nil {
			return
		}
		oldest.Evicted = true
		delete(s.bySession[oldestChat], oldest.ID)
		if len(s.bySession[oldestChat]) == 0 {
			delete(s.bySession, oldestChat)
		}
		totalHandles--
		totalUnits -= oldest.StoredUnits
	}
}

// MarkTerminalClosed records a closed terminal session so its handles are
// lifecycle-denied (bounded set, ported from closedTerminalSessions).
func (s *OutputStore) MarkTerminalClosed(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closedTerm[sessionID] = true
	if len(s.closedTerm) > MaxClosedTerminals {
		for id := range s.closedTerm {
			delete(s.closedTerm, id)
			break
		}
	}
}

// DenyChat adds a chat to the lifecycle deny filter (chat deletion).
func (s *OutputStore) DenyChat(chatSessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.denyChat = append(s.denyChat, chatSessionID)
}

func (s *OutputStore) lifecycleDenied(chatID, sessionID string) bool {
	for _, denied := range s.denyChat {
		if denied == chatID {
			return true
		}
	}
	return sessionID != "" && s.closedTerm[sessionID]
}

func newHandleID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic("tools: handle entropy unavailable: " + err.Error())
	}
	return "tool-output-" + hex.EncodeToString(raw[:])
}
