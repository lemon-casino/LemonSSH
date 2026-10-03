package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/binaricat/lemonssh/internal/agent/tools"
	"github.com/binaricat/lemonssh/internal/platform/filesystem"
)

// Renderer tool-output persistence (audit: lemonsshTurnDriver's toolOutputTemp
// bridge had no Wails implementation, so the harness ToolOutputStore never
// spilled and restore after a restart was impossible). The W14 handle store
// (toolOutputFacade.go) cannot back this channel: it is the Go turn runtime's
// own in-memory, chat-scoped surface whose handles do not survive a restart.
// These methods therefore extend the service that owns the dedicated LemonSSH
// temp service (never os.TempDir, workspace rule): one JSON envelope per
// handle under <managed temp>/tool-output/<handleId>.json holding the
// persisted record plus the retained content. Reads replicate the renderer's
// buildReadResult semantics (UTF-16 unit offsets, search excerpts, surrogate
// safe bounds) so a restored handle behaves exactly like an in-memory one.

const (
	toolOutputTempDirRel = "tool-output"
	toolOutputTempExt    = ".json"
	// Frozen baseline constants from toolOutputStore.ts (UTF-16 units).
	toolOutputSearchContextChars = 320
	toolOutputMaxHandleIDLen     = 128
)

// ToolOutputRecord mirrors the renderer's PersistedToolOutputRecord
// (infrastructure/ai/harness/toolOutputStore.ts). Char counts are UTF-16
// code units; timestamps are epoch milliseconds.
type ToolOutputRecord struct {
	SchemaVersion     int    `json:"schemaVersion"`
	HandleID          string `json:"handleId"`
	ChatSessionID     string `json:"chatSessionId"`
	CapabilityID      string `json:"capabilityId"`
	TerminalSessionID string `json:"terminalSessionId,omitempty"`
	TotalChars        int64  `json:"totalChars"`
	StoredChars       int64  `json:"storedChars"`
	SourceTruncated   bool   `json:"sourceTruncated"`
	Preview           string `json:"preview"`
	StoredAt          int64  `json:"storedAt"`
	AccessedAt        int64  `json:"accessedAt"`
}

// toolOutputEnvelope is the on-disk shape: metadata plus the retained
// content, written atomically per handle.
type toolOutputEnvelope struct {
	Record  ToolOutputRecord `json:"record"`
	Content string           `json:"content"`
}

// ToolOutputPersistenceStatusResult reports whether durable tool-output
// spill storage is available.
type ToolOutputPersistenceStatusResult struct {
	Durable bool   `json:"durable"`
	Reason  string `json:"reason,omitempty"`
}

// ToolOutputWriteResult reports one spill outcome and the managed path the
// renderer must keep on its handle.
type ToolOutputWriteResult struct {
	OK    bool   `json:"ok"`
	Path  string `json:"path,omitempty"`
	Error string `json:"error,omitempty"`
}

// ToolOutputRestoredRecord is one successful restore: the managed path plus
// the persisted record for renderer-side validation.
type ToolOutputRestoredRecord struct {
	Path   string           `json:"path"`
	Record ToolOutputRecord `json:"record"`
}

// ToolOutputReadRequest mirrors the renderer's ReadToolOutputInput
// (mode/maxChars/offset/query). MaxChars is a pointer so an omitted field
// falls back to the frozen 12k default while an explicit 0 clamps to 1.
type ToolOutputReadRequest struct {
	HandleID string `json:"handleId,omitempty"`
	Mode     string `json:"mode,omitempty"`
	MaxChars *int   `json:"maxChars,omitempty"`
	Offset   int    `json:"offset,omitempty"`
	Query    string `json:"query,omitempty"`
}

// ToolOutputReadPayload mirrors the renderer's read result minus the fields
// the harness overrides from its own handle (handleId/storedChars/
// sourceTruncated). Offsets are UTF-16 code units.
type ToolOutputReadPayload struct {
	Mode         string `json:"mode"`
	Content      string `json:"content"`
	TotalChars   int    `json:"totalChars"`
	StartOffset  int    `json:"startOffset"`
	EndOffset    int    `json:"endOffset"`
	NextOffset   int    `json:"nextOffset"`
	HasMore      bool   `json:"hasMore"`
	MatchOffsets []int  `json:"matchOffsets,omitempty"`
}

// ToolOutputDeleteResult reports one path deletion.
type ToolOutputDeleteResult struct {
	OK bool `json:"ok"`
}

// ToolOutputDeleteManyResult reports how many spill files a scoped delete
// removed.
type ToolOutputDeleteManyResult struct {
	DeletedCount int `json:"deletedCount"`
}

// ToolOutputPersistenceStatus reports whether the dedicated temp service is
// wired; the renderer gates spill and restore on this.
func (s *FilesystemService) ToolOutputPersistenceStatus() ToolOutputPersistenceStatusResult {
	if s.temp == nil {
		return ToolOutputPersistenceStatusResult{Durable: false, Reason: "managed temp unavailable"}
	}
	return ToolOutputPersistenceStatusResult{Durable: true}
}

// WriteToolOutputTemp stores one record plus content in the managed temp
// directory and returns the managed path. The write lands via a same-name
// .tmp file and rename so a crash cannot leave a half-written envelope
// behind under the final name.
func (s *FilesystemService) WriteToolOutputTemp(record ToolOutputRecord, content string) (ToolOutputWriteResult, error) {
	if s.temp == nil {
		return ToolOutputWriteResult{}, errors.New("managed temp unavailable")
	}
	if record.SchemaVersion != 1 {
		return ToolOutputWriteResult{}, fmt.Errorf("unsupported tool output record schema %d", record.SchemaVersion)
	}
	name, err := toolOutputFileName(record.HandleID)
	if err != nil {
		return ToolOutputWriteResult{}, err
	}
	if strings.TrimSpace(record.ChatSessionID) == "" {
		return ToolOutputWriteResult{}, errors.New("tool output record requires chatSessionId")
	}
	if strings.TrimSpace(record.CapabilityID) == "" {
		return ToolOutputWriteResult{}, errors.New("tool output record requires capabilityId")
	}
	if tools.UTF16Len(content) > tools.MaxHandleChars {
		return ToolOutputWriteResult{}, fmt.Errorf("tool output exceeds local handle limit of %d units", tools.MaxHandleChars)
	}
	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(toolOutputEnvelope{Record: record, Content: content}); err != nil {
		return ToolOutputWriteResult{}, fmt.Errorf("tool output encode: %w", err)
	}
	relTarget := toolOutputTempDirRel + "/" + name
	relTmp := relTarget + ".tmp"
	if err := s.temp.WriteFile(relTmp, payload.Bytes()); err != nil {
		return ToolOutputWriteResult{}, fmt.Errorf("tool output write: %w", err)
	}
	target, err := s.temp.FilePath(relTarget)
	if err != nil {
		_ = s.temp.Remove(relTmp)
		return ToolOutputWriteResult{}, err
	}
	tmp, err := s.temp.FilePath(relTmp)
	if err != nil {
		_ = s.temp.Remove(relTmp)
		return ToolOutputWriteResult{}, err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = s.temp.Remove(relTmp)
		return ToolOutputWriteResult{}, fmt.Errorf("tool output write: %w", err)
	}
	return ToolOutputWriteResult{OK: true, Path: target}, nil
}

// RestoreToolOutputTemp looks up one persisted handle for a chat session.
// Missing, corrupt, or foreign records return null (not an error) so the
// renderer treats the handle simply as unrestorable.
func (s *FilesystemService) RestoreToolOutputTemp(handleID, chatSessionID string) (*ToolOutputRestoredRecord, error) {
	envelope, target, found, err := s.readToolOutputEnvelope(handleID)
	if err != nil {
		return nil, err
	}
	if !found || envelope.Record.ChatSessionID != chatSessionID {
		return nil, nil
	}
	return &ToolOutputRestoredRecord{Path: target, Record: envelope.Record}, nil
}

// ReadToolOutputTemp serves one bounded read of a persisted spill file with
// the same head/tail/range/full/search semantics as the in-memory store.
// Corrupt or missing files return null; only path escapes are hard errors.
func (s *FilesystemService) ReadToolOutputTemp(filePath string, request *ToolOutputReadRequest) (*ToolOutputReadPayload, error) {
	envelope, found, err := s.readToolOutputEnvelopeAt(filePath)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	readRequest := ToolOutputReadRequest{}
	if request != nil {
		readRequest = *request
	}
	if readRequest.HandleID != "" && readRequest.HandleID != envelope.Record.HandleID {
		return nil, nil
	}
	payload := buildToolOutputReadPayload(envelope.Content, readRequest)
	return &payload, nil
}

// DeleteToolOutputTemp removes one spill file. Paths outside the managed
// tool-output directory are rejected; a missing file deletes successfully.
func (s *FilesystemService) DeleteToolOutputTemp(filePath string) (ToolOutputDeleteResult, error) {
	if s.temp == nil {
		return ToolOutputDeleteResult{}, errors.New("managed temp unavailable")
	}
	rel, err := s.managedToolOutputRel(filePath)
	if err != nil {
		return ToolOutputDeleteResult{}, err
	}
	if err := s.temp.Remove(rel); err != nil {
		return ToolOutputDeleteResult{}, err
	}
	return ToolOutputDeleteResult{OK: true}, nil
}

// DeleteChatToolOutputsTemp removes every spill file owned by one chat.
func (s *FilesystemService) DeleteChatToolOutputsTemp(chatSessionID string) (ToolOutputDeleteManyResult, error) {
	return s.deleteToolOutputsTempWhere(func(record ToolOutputRecord) bool {
		return record.ChatSessionID == chatSessionID
	})
}

// DeleteTerminalToolOutputsTemp removes spill files for one chat+terminal
// pair.
func (s *FilesystemService) DeleteTerminalToolOutputsTemp(chatSessionID, terminalSessionID string) (ToolOutputDeleteManyResult, error) {
	return s.deleteToolOutputsTempWhere(func(record ToolOutputRecord) bool {
		return record.ChatSessionID == chatSessionID && record.TerminalSessionID == terminalSessionID
	})
}

// DeleteTerminalToolOutputsEverywhereTemp removes spill files for a terminal
// session across every chat.
func (s *FilesystemService) DeleteTerminalToolOutputsEverywhereTemp(terminalSessionID string) (ToolOutputDeleteManyResult, error) {
	return s.deleteToolOutputsTempWhere(func(record ToolOutputRecord) bool {
		return record.TerminalSessionID == terminalSessionID
	})
}

// readToolOutputEnvelope resolves <dir>/<handleId>.json and parses it.
// found=false means absent or corrupt (restore semantics: not restorable).
func (s *FilesystemService) readToolOutputEnvelope(handleID string) (toolOutputEnvelope, string, bool, error) {
	var envelope toolOutputEnvelope
	if s.temp == nil {
		return envelope, "", false, errors.New("managed temp unavailable")
	}
	name, err := toolOutputFileName(handleID)
	if err != nil {
		return envelope, "", false, err
	}
	target, err := s.temp.FilePath(toolOutputTempDirRel + "/" + name)
	if err != nil {
		return envelope, "", false, err
	}
	envelope, found, err := parseToolOutputEnvelope(target)
	if err != nil || !found {
		return envelope, "", false, err
	}
	if !envelope.Record.validFor(handleID) {
		return toolOutputEnvelope{}, "", false, nil
	}
	return envelope, target, true, nil
}

// readToolOutputEnvelopeAt parses an absolute spill path the renderer kept
// on a handle. The path must resolve inside the managed tool-output
// directory (lexically and through symlinks).
func (s *FilesystemService) readToolOutputEnvelopeAt(filePath string) (toolOutputEnvelope, bool, error) {
	var envelope toolOutputEnvelope
	if s.temp == nil {
		return envelope, false, errors.New("managed temp unavailable")
	}
	rel, err := s.managedToolOutputRel(filePath)
	if err != nil {
		return envelope, false, err
	}
	target, err := s.temp.FilePath(rel)
	if err != nil {
		return envelope, false, err
	}
	return parseToolOutputEnvelope(target)
}

// managedToolOutputRel validates that an absolute path names a regular
// spill file (one segment under the managed tool-output directory) and
// returns its managed-relative form.
func (s *FilesystemService) managedToolOutputRel(filePath string) (string, error) {
	if s.temp == nil {
		return "", errors.New("managed temp unavailable")
	}
	rel, err := filepath.Rel(s.temp.Root(), filePath)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", filesystem.ErrPathEscapesRoot, filePath)
	}
	segments := strings.Split(rel, string(filepath.Separator))
	if len(segments) != 2 || segments[0] != toolOutputTempDirRel || !strings.HasSuffix(segments[1], toolOutputTempExt) {
		return "", fmt.Errorf("tool output path must stay inside %s: %q", toolOutputTempDirRel, filePath)
	}
	return rel, nil
}

// deleteToolOutputsTempWhere scans the spill directory once and removes
// every envelope whose record matches. Unparseable files are left alone:
// they cannot be attributed to a scope and ClearTemp still reclaims them.
func (s *FilesystemService) deleteToolOutputsTempWhere(match func(ToolOutputRecord) bool) (ToolOutputDeleteManyResult, error) {
	if s.temp == nil {
		return ToolOutputDeleteManyResult{}, errors.New("managed temp unavailable")
	}
	dir, err := s.temp.FilePath(toolOutputTempDirRel)
	if err != nil {
		return ToolOutputDeleteManyResult{}, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return ToolOutputDeleteManyResult{}, nil
		}
		return ToolOutputDeleteManyResult{}, err
	}
	deleted := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), toolOutputTempExt) {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			continue
		}
		var envelope toolOutputEnvelope
		if json.Unmarshal(data, &envelope) != nil || envelope.Record.SchemaVersion != 1 {
			continue
		}
		if !match(envelope.Record) {
			continue
		}
		if s.temp.Remove(toolOutputTempDirRel+"/"+entry.Name()) == nil {
			deleted++
		}
	}
	return ToolOutputDeleteManyResult{DeletedCount: deleted}, nil
}

func (r ToolOutputRecord) validFor(handleID string) bool {
	return r.SchemaVersion == 1 &&
		r.HandleID == handleID &&
		strings.TrimSpace(r.CapabilityID) != "" &&
		r.TotalChars >= 0 &&
		r.StoredChars >= 0 &&
		r.StoredChars <= tools.MaxHandleChars
}

func parseToolOutputEnvelope(target string) (toolOutputEnvelope, bool, error) {
	var envelope toolOutputEnvelope
	data, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return envelope, false, nil
		}
		return envelope, false, err
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return envelope, false, nil
	}
	return envelope, true, nil
}

// toolOutputFileName turns a renderer handle id into a safe file name. Only
// [A-Za-z0-9-] survives, so a hostile id cannot escape the spill directory.
func toolOutputFileName(handleID string) (string, error) {
	trimmed := strings.TrimSpace(handleID)
	if trimmed == "" || len(trimmed) > toolOutputMaxHandleIDLen {
		return "", fmt.Errorf("invalid tool output handle id %q", handleID)
	}
	for _, r := range trimmed {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return "", fmt.Errorf("invalid tool output handle id %q", handleID)
	}
	return trimmed + toolOutputTempExt, nil
}

// buildToolOutputReadPayload ports the renderer's buildReadResult
// (toolOutputStore.ts) onto UTF-16 code units so offsets stay interchangeable
// between in-memory and restored reads.
func buildToolOutputReadPayload(content string, request ToolOutputReadRequest) ToolOutputReadPayload {
	units := utf16.Encode([]rune(content))
	requestedMax := tools.ReadMaxChars
	if request.MaxChars != nil {
		requestedMax = *request.MaxChars
	}
	maxChars := requestedMax
	if maxChars > tools.ReadMaxChars {
		maxChars = tools.ReadMaxChars
	}
	if maxChars < 1 {
		maxChars = 1
	}
	mode := request.Mode
	if mode == "" {
		mode = string(tools.ModeHead)
	}
	if mode == string(tools.ModeSearch) {
		return buildToolOutputSearchPayload(units, request, maxChars)
	}

	startOffset := 0
	switch mode {
	case string(tools.ModeTail):
		startOffset = max(0, len(units)-maxChars)
	case string(tools.ModeRange):
		startOffset = min(max(0, request.Offset), len(units))
	}
	start, end := toolOutputSafeSliceBounds(units, startOffset, startOffset+maxChars)
	return ToolOutputReadPayload{
		Mode:        mode,
		Content:     toolOutputString(units[start:end]),
		TotalChars:  len(units),
		StartOffset: start,
		EndOffset:   end,
		NextOffset:  end,
		HasMore:     end < len(units),
	}
}

func buildToolOutputSearchPayload(units []uint16, request ToolOutputReadRequest, maxChars int) ToolOutputReadPayload {
	query := request.Query
	if query == "" {
		return ToolOutputReadPayload{
			Mode:         string(tools.ModeSearch),
			Content:      "Search query is required.",
			TotalChars:   len(units),
			MatchOffsets: []int{},
		}
	}
	haystack := toolOutputLowerUnits(units)
	needle := toolOutputLowerUnits(utf16.Encode([]rune(query)))
	cursor := max(0, request.Offset)
	var offsets []int
	for len(offsets) < tools.SearchMaxMatches {
		match := toolOutputIndexOf(haystack, needle, cursor)
		if match < 0 {
			break
		}
		offsets = append(offsets, match)
		cursor = match + max(1, len(needle))
	}

	var excerpts []string
	renderedOffsets := []int{}
	renderedChars := 0
	for _, match := range offsets {
		start, end := toolOutputSafeSliceBounds(
			units,
			match-toolOutputSearchContextChars,
			match+len(needle)+toolOutputSearchContextChars,
		)
		excerpt := fmt.Sprintf("[match offset=%d]\n%s", match, toolOutputString(units[start:end]))
		excerptUnits := utf16.Encode([]rune(excerpt))
		separator := ""
		if len(excerpts) > 0 {
			separator = "\n\n"
		}
		available := maxChars - renderedChars - len(separator)
		if available <= 0 {
			break
		}
		if len(excerptUnits) > available {
			if len(excerpts) > 0 {
				break
			}
			_, safeEnd := toolOutputSafeSliceBounds(excerptUnits, 0, available)
			excerpts = append(excerpts, toolOutputString(excerptUnits[:safeEnd]))
			renderedOffsets = append(renderedOffsets, match)
			renderedChars += safeEnd
			break
		}
		excerpts = append(excerpts, excerpt)
		renderedOffsets = append(renderedOffsets, match)
		renderedChars += len(separator) + len(excerptUnits)
	}

	nextOffset := len(units)
	if len(renderedOffsets) > 0 {
		nextOffset = renderedOffsets[len(renderedOffsets)-1] + max(1, len(needle))
	}
	rendered := strings.Join(excerpts, "\n\n")
	if rendered == "" {
		rendered = "No matches found for \"" + query + "\"."
	}
	return ToolOutputReadPayload{
		Mode:         string(tools.ModeSearch),
		Content:      rendered,
		TotalChars:   len(units),
		StartOffset:  max(0, request.Offset),
		EndOffset:    nextOffset,
		NextOffset:   nextOffset,
		HasMore:      toolOutputIndexOf(haystack, needle, nextOffset) >= 0,
		MatchOffsets: renderedOffsets,
	}
}

// toolOutputLowerUnits lowercases UTF-16 units with Unicode simple case
// mapping. Simple mappings keep every rune's UTF-16 length, so offsets in
// the lowered buffer line up with the original.
func toolOutputLowerUnits(units []uint16) []uint16 {
	out := make([]uint16, len(units))
	for i := 0; i < len(units); {
		r := rune(units[i])
		width := 1
		if utf16.IsSurrogate(r) && i+1 < len(units) {
			if combined := utf16.DecodeRune(r, rune(units[i+1])); combined != unicode.ReplacementChar {
				r = combined
				width = 2
			}
		}
		encoded := utf16.Encode([]rune{unicode.ToLower(r)})
		if len(encoded) != width {
			// Length-changing mapping would desync offsets; keep the original.
			encoded = []uint16{units[i]}
			if width == 2 {
				encoded = []uint16{units[i], units[i+1]}
			}
		}
		copy(out[i:], encoded)
		i += width
	}
	return out
}

func toolOutputIndexOf(haystack, needle []uint16, from int) int {
	if from < 0 {
		from = 0
	}
	if from > len(haystack) {
		from = len(haystack)
	}
	if len(needle) == 0 {
		return from
	}
	limit := len(haystack) - len(needle)
	for i := from; i <= limit; i++ {
		if haystack[i] != needle[0] {
			continue
		}
		match := true
		for j := 1; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// toolOutputSafeSliceBounds ports safeSliceBounds: clamps a requested window
// and backs off half a surrogate pair so slices never split one.
func toolOutputSafeSliceBounds(units []uint16, requestedStart, requestedEnd int) (int, int) {
	start := min(max(0, requestedStart), len(units))
	end := min(max(start, requestedEnd), len(units))
	if start > 0 && start < len(units) && isToolOutputLowSurrogate(units[start]) {
		start--
	}
	if end > start && end < len(units) && isToolOutputHighSurrogate(units[end-1]) {
		end--
	}
	return start, end
}

func isToolOutputHighSurrogate(unit uint16) bool {
	return unit >= 0xD800 && unit <= 0xDBFF
}

func isToolOutputLowSurrogate(unit uint16) bool {
	return unit >= 0xDC00 && unit <= 0xDFFF
}

func toolOutputString(units []uint16) string {
	return string(utf16.Decode(units))
}
