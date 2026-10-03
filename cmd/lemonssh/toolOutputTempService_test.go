package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/binaricat/lemonssh/internal/platform/filesystem"
)

func newToolOutputTestService(t *testing.T) (*FilesystemService, string) {
	t.Helper()
	temp, err := filesystem.NewTempService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := &FilesystemService{temp: temp}
	return service, temp.Root()
}

func toolOutputTestRecord(handleID, chatSessionID, terminalSessionID string) ToolOutputRecord {
	return ToolOutputRecord{
		SchemaVersion:     1,
		HandleID:          handleID,
		ChatSessionID:     chatSessionID,
		CapabilityID:      "terminal_exec",
		TerminalSessionID: terminalSessionID,
		TotalChars:        int64(len([]rune(handleID))),
		StoredChars:       int64(len([]rune(handleID))),
		Preview:           "preview",
		StoredAt:          1727890123456,
		AccessedAt:        1727890999999,
	}
}

func TestToolOutputPersistenceStatusReportsTempAvailability(t *testing.T) {
	service, _ := newToolOutputTestService(t)
	status := service.ToolOutputPersistenceStatus()
	if !status.Durable || status.Reason != "" {
		t.Fatalf("wired temp service must be durable, got %+v", status)
	}
	withoutTemp := newFilesystemService()
	status = withoutTemp.ToolOutputPersistenceStatus()
	if status.Durable || status.Reason == "" {
		t.Fatalf("missing temp service must be non-durable with a reason, got %+v", status)
	}
}

func TestToolOutputTempWriteReadDeleteRoundTrip(t *testing.T) {
	service, root := newToolOutputTestService(t)
	record := toolOutputTestRecord("tool-output-roundtrip-1", "chat-1", "term-1")
	content := "hello tool output 日本語 🎉 tail"

	write, err := service.WriteToolOutputTemp(record, content)
	if err != nil || !write.OK {
		t.Fatalf("write: %+v, %v", write, err)
	}
	wantPath := filepath.Join(root, "tool-output", "tool-output-roundtrip-1.json")
	if write.Path != wantPath {
		t.Fatalf("path = %q, want %q", write.Path, wantPath)
	}
	// Atomic write must not leave staging residue and must persist the record
	// itself so restore works after a restart.
	if _, err := os.Stat(write.Path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp residue left behind: %v", err)
	}
	raw, err := os.ReadFile(write.Path)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Record  ToolOutputRecord `json:"record"`
		Content string           `json:"content"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("envelope parse: %v", err)
	}
	if envelope.Record.HandleID != record.HandleID || envelope.Record.ChatSessionID != "chat-1" ||
		envelope.Record.StoredAt != record.StoredAt || envelope.Content != content {
		t.Fatalf("envelope mismatch: %+v", envelope)
	}

	head, err := service.ReadToolOutputTemp(write.Path, &ToolOutputReadRequest{HandleID: record.HandleID, MaxChars: intp(5)})
	if err != nil || head == nil {
		t.Fatalf("head read: %v, %v", head, err)
	}
	if head.Content != "hello" || head.StartOffset != 0 || head.EndOffset != 5 || head.NextOffset != 5 || !head.HasMore {
		t.Fatalf("head payload: %+v", head)
	}

	tail, err := service.ReadToolOutputTemp(write.Path, &ToolOutputReadRequest{Mode: "tail"})
	if err != nil || tail == nil || tail.Content != content {
		t.Fatalf("tail read: %+v, %v", tail, err)
	}

	mid, err := service.ReadToolOutputTemp(write.Path, &ToolOutputReadRequest{Mode: "range", Offset: 6})
	if err != nil || mid == nil {
		t.Fatalf("range read: %v, %v", mid, err)
	}
	if !strings.HasPrefix(mid.Content, "tool output") {
		t.Fatalf("range content = %q", mid.Content)
	}

	folded, err := service.ReadToolOutputTemp(write.Path, &ToolOutputReadRequest{Mode: "search", Query: "TAIL"})
	if err != nil || folded == nil || len(folded.MatchOffsets) != 1 {
		t.Fatalf("search read: %+v, %v", folded, err)
	}
	if !strings.Contains(folded.Content, "[match offset=") || folded.HasMore {
		t.Fatalf("search payload: %+v", folded)
	}

	missingQuery, err := service.ReadToolOutputTemp(write.Path, &ToolOutputReadRequest{Mode: "search"})
	if err != nil || missingQuery == nil || missingQuery.Content != "Search query is required." {
		t.Fatalf("empty search: %+v, %v", missingQuery, err)
	}

	if _, err := service.DeleteToolOutputTemp(write.Path); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if gone, err := service.ReadToolOutputTemp(write.Path, nil); err != nil || gone != nil {
		t.Fatalf("read after delete must be null, got %+v, %v", gone, err)
	}
	if restored, err := service.RestoreToolOutputTemp(record.HandleID, "chat-1"); err != nil || restored != nil {
		t.Fatalf("restore after delete must be null, got %+v, %v", restored, err)
	}
	// Deleting an already-deleted file stays successful (idempotent).
	if _, err := service.DeleteToolOutputTemp(write.Path); err != nil {
		t.Fatalf("idempotent delete: %v", err)
	}
}

func TestToolOutputTempRewriteReplacesContent(t *testing.T) {
	service, _ := newToolOutputTestService(t)
	record := toolOutputTestRecord("tool-output-rewrite-1", "chat-1", "")
	if _, err := service.WriteToolOutputTemp(record, "first"); err != nil {
		t.Fatal(err)
	}
	write, err := service.WriteToolOutputTemp(record, "second")
	if err != nil {
		t.Fatal(err)
	}
	read, err := service.ReadToolOutputTemp(write.Path, nil)
	if err != nil || read == nil || read.Content != "second" {
		t.Fatalf("rewritten read: %+v, %v", read, err)
	}
}

func TestToolOutputTempRestoreValidatesScopeAndRecord(t *testing.T) {
	service, _ := newToolOutputTestService(t)
	recordA := toolOutputTestRecord("tool-output-restore-a", "chat-a", "term-a")
	recordB := toolOutputTestRecord("tool-output-restore-b", "chat-b", "")
	writeA, err := service.WriteToolOutputTemp(recordA, "content-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.WriteToolOutputTemp(recordB, "content-b"); err != nil {
		t.Fatal(err)
	}

	restored, err := service.RestoreToolOutputTemp(recordA.HandleID, "chat-a")
	if err != nil || restored == nil {
		t.Fatalf("restore: %+v, %v", restored, err)
	}
	if restored.Path != writeA.Path || restored.Record.HandleID != recordA.HandleID ||
		restored.Record.ChatSessionID != "chat-a" || restored.Record.CapabilityID != "terminal_exec" {
		t.Fatalf("restored record: %+v", restored)
	}

	if foreign, err := service.RestoreToolOutputTemp(recordA.HandleID, "chat-b"); err != nil || foreign != nil {
		t.Fatalf("cross-chat restore must be null, got %+v, %v", foreign, err)
	}
	if unknown, err := service.RestoreToolOutputTemp("tool-output-missing", "chat-a"); err != nil || unknown != nil {
		t.Fatalf("unknown restore must be null, got %+v, %v", unknown, err)
	}

	// A corrupt envelope is not restorable but must not error.
	corruptRecord := toolOutputTestRecord("tool-output-corrupt", "chat-a", "")
	write, err := service.WriteToolOutputTemp(corruptRecord, "doomed")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(write.Path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if corrupt, err := service.RestoreToolOutputTemp(corruptRecord.HandleID, "chat-a"); err != nil || corrupt != nil {
		t.Fatalf("corrupt restore must be null, got %+v, %v", corrupt, err)
	}

	// A record whose embedded id disagrees with the file name is refused.
	mismatched := toolOutputTestRecord("tool-output-mismatch", "chat-a", "")
	write, err = service.WriteToolOutputTemp(mismatched, "swap")
	if err != nil {
		t.Fatal(err)
	}
	spoofed, err := os.ReadFile(write.Path)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(spoofed, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope["record"].(map[string]any)["handleId"] = "tool-output-other"
	rewritten, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(write.Path, rewritten, 0o600); err != nil {
		t.Fatal(err)
	}
	if swapped, err := service.RestoreToolOutputTemp(mismatched.HandleID, "chat-a"); err != nil || swapped != nil {
		t.Fatalf("mismatched restore must be null, got %+v, %v", swapped, err)
	}
}

func TestToolOutputTempDeleteScoped(t *testing.T) {
	service, root := newToolOutputTestService(t)
	handles := []struct {
		id      string
		chat    string
		term    string
		content string
	}{
		{"tool-output-scope-1", "chat-a", "term-x", "one"},
		{"tool-output-scope-2", "chat-a", "term-y", "two"},
		{"tool-output-scope-3", "chat-b", "term-x", "three"},
		{"tool-output-scope-4", "chat-c", "", "four"},
	}
	paths := map[string]string{}
	for _, handle := range handles {
		write, err := service.WriteToolOutputTemp(toolOutputTestRecord(handle.id, handle.chat, handle.term), handle.content)
		if err != nil {
			t.Fatal(err)
		}
		paths[handle.id] = write.Path
	}
	// Unattributable garbage must survive scoped deletes for ClearTemp to own.
	garbage := filepath.Join(root, "tool-output", "tool-output-garbage.json")
	if err := os.WriteFile(garbage, []byte("<<<"), 0o600); err != nil {
		t.Fatal(err)
	}

	deleted, err := service.DeleteChatToolOutputsTemp("chat-a")
	if err != nil || deleted.DeletedCount != 2 {
		t.Fatalf("chat delete: %+v, %v", deleted, err)
	}
	if _, err := os.Stat(garbage); err != nil {
		t.Fatalf("garbage must survive scoped delete: %v", err)
	}
	if missing, err := service.ReadToolOutputTemp(paths["tool-output-scope-1"], nil); err != nil || missing != nil {
		t.Fatalf("deleted chat file must be gone: %+v, %v", missing, err)
	}
	if kept, err := service.RestoreToolOutputTemp("tool-output-scope-3", "chat-b"); err != nil || kept == nil {
		t.Fatalf("chat-b must survive: %+v, %v", kept, err)
	}

	deleted, err = service.DeleteTerminalToolOutputsEverywhereTemp("term-x")
	if err != nil || deleted.DeletedCount != 1 {
		t.Fatalf("terminal everywhere delete: %+v, %v", deleted, err)
	}
	if gone, err := service.RestoreToolOutputTemp("tool-output-scope-3", "chat-b"); err != nil || gone != nil {
		t.Fatalf("term-x chat-b must be gone: %+v, %v", gone, err)
	}

	deleted, err = service.DeleteTerminalToolOutputsTemp("chat-c", "term-y")
	if err != nil || deleted.DeletedCount != 0 {
		t.Fatalf("non-matching terminal delete: %+v, %v", deleted, err)
	}
	if kept, err := service.RestoreToolOutputTemp("tool-output-scope-4", "chat-c"); err != nil || kept == nil {
		t.Fatalf("chat-c must survive: %+v, %v", kept, err)
	}

	deleted, err = service.DeleteChatToolOutputsTemp("chat-unknown")
	if err != nil || deleted.DeletedCount != 0 {
		t.Fatalf("unknown chat delete: %+v, %v", deleted, err)
	}
}

func TestToolOutputTempRejectsUnsafeInputs(t *testing.T) {
	service, root := newToolOutputTestService(t)
	if _, err := service.WriteToolOutputTemp(toolOutputTestRecord("../evil", "chat", ""), "x"); err == nil {
		t.Fatal("path traversal handle id must be rejected")
	}
	if _, err := service.WriteToolOutputTemp(toolOutputTestRecord("bad/id", "chat", ""), "x"); err == nil {
		t.Fatal("slash handle id must be rejected")
	}
	badSchema := toolOutputTestRecord("tool-output-schema", "chat", "")
	badSchema.SchemaVersion = 2
	if _, err := service.WriteToolOutputTemp(badSchema, "x"); err == nil {
		t.Fatal("unsupported schema must be rejected")
	}
	noChat := toolOutputTestRecord("tool-output-nochat", "", "")
	if _, err := service.WriteToolOutputTemp(noChat, "x"); err == nil {
		t.Fatal("empty chat session must be rejected")
	}
	if _, err := service.WriteToolOutputTemp(toolOutputTestRecord("tool-output-big", "chat", ""), strings.Repeat("a", 4_000_001)); err == nil {
		t.Fatal("oversized content must be rejected")
	}

	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReadToolOutputTemp(outside, nil); err == nil {
		t.Fatal("read outside the managed root must fail")
	}
	if _, err := service.DeleteToolOutputTemp(outside); err == nil {
		t.Fatal("delete outside the managed root must fail")
	}
	insideRoot := filepath.Join(root, "stray.json")
	if err := os.WriteFile(insideRoot, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReadToolOutputTemp(insideRoot, nil); err == nil {
		t.Fatal("read outside the tool-output directory must fail")
	}
	if _, err := service.DeleteToolOutputTemp(insideRoot); err == nil {
		t.Fatal("delete outside the tool-output directory must fail")
	}

	withoutTemp := newFilesystemService()
	if _, err := withoutTemp.WriteToolOutputTemp(toolOutputTestRecord("tool-output-x", "chat", ""), "x"); err == nil {
		t.Fatal("write without managed temp must fail")
	}
	if _, err := withoutTemp.DeleteChatToolOutputsTemp("chat"); err == nil {
		t.Fatal("scoped delete without managed temp must fail")
	}
}

func TestToolOutputTempReadMatchesRendererReadSemantics(t *testing.T) {
	service, _ := newToolOutputTestService(t)
	record := toolOutputTestRecord("tool-output-units", "chat-1", "")
	// UTF-16 units: a(1) 🎉(2) b(1) -> length 4.
	content := "a🎉b"
	write, err := service.WriteToolOutputTemp(record, content)
	if err != nil {
		t.Fatal(err)
	}

	// A range that starts on a low surrogate steps back one unit, matching
	// safeSliceBounds in toolOutputStore.ts; the end only backs off a HIGH
	// surrogate, so the window re-expands to cover the whole pair.
	split, err := service.ReadToolOutputTemp(write.Path, &ToolOutputReadRequest{Mode: "range", Offset: 2, MaxChars: intp(1)})
	if err != nil || split == nil {
		t.Fatalf("split read: %+v, %v", split, err)
	}
	if split.StartOffset != 1 || split.EndOffset != 3 || split.Content != "🎉" || !split.HasMore {
		t.Fatalf("surrogate bounds: %+v", split)
	}

	// Search: case-insensitive with context excerpts and capped rendering.
	searchRecord := toolOutputTestRecord("tool-output-search", "chat-1", "")
	searchWrite, err := service.WriteToolOutputTemp(searchRecord, "alpha BETA gamma beta delta")
	if err != nil {
		t.Fatal(err)
	}
	found, err := service.ReadToolOutputTemp(searchWrite.Path, &ToolOutputReadRequest{Mode: "search", Query: "beta"})
	if err != nil || found == nil {
		t.Fatalf("search: %+v, %v", found, err)
	}
	if len(found.MatchOffsets) != 2 || found.MatchOffsets[0] != 6 || found.MatchOffsets[1] != 17 {
		t.Fatalf("match offsets: %+v", found)
	}
	if found.NextOffset != 21 || found.EndOffset != 21 || found.HasMore {
		t.Fatalf("search cursor: %+v", found)
	}
	if !strings.Contains(found.Content, "[match offset=6]\nalpha BETA gamma beta delta") {
		t.Fatalf("excerpt rendering: %q", found.Content)
	}

	capped, err := service.ReadToolOutputTemp(searchWrite.Path, &ToolOutputReadRequest{Mode: "search", Query: "beta", MaxChars: intp(30)})
	if err != nil || capped == nil {
		t.Fatalf("capped search: %+v, %v", capped, err)
	}
	if len(capped.MatchOffsets) != 1 || capped.MatchOffsets[0] != 6 {
		t.Fatalf("capped offsets: %+v", capped)
	}
	if len([]rune(capped.Content)) > 30 {
		t.Fatalf("capped content exceeds budget: %q", capped.Content)
	}

	none, err := service.ReadToolOutputTemp(searchWrite.Path, &ToolOutputReadRequest{Mode: "search", Query: "zeta"})
	if err != nil || none == nil || none.Content != "No matches found for \"zeta\"." || len(none.MatchOffsets) != 0 {
		t.Fatalf("no-match search: %+v, %v", none, err)
	}

	// A read whose handleId disagrees with the envelope is refused.
	if wrong, err := service.ReadToolOutputTemp(searchWrite.Path, &ToolOutputReadRequest{HandleID: "tool-output-other"}); err != nil || wrong != nil {
		t.Fatalf("foreign handle read must be null: %+v, %v", wrong, err)
	}
}

func intp(value int) *int {
	return &value
}
