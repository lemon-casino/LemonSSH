package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newValidChatSessionID(t *testing.T) ChatSessionID {
	t.Helper()
	id := NewChatSessionID()
	if !id.Valid() {
		t.Fatalf("fresh chat id invalid: %s", id)
	}
	return id
}

func TestAgentPrepareTurnRoundTrip(t *testing.T) {
	request := PrepareTurnRequest{
		RequestID:            NewRequestID(),
		ChatSessionID:        newValidChatSessionID(t),
		ExpectedChatRevision: "18446744073709551615",
		AgentID:              NewAgentID(),
		ModelID:              "glm-5.3-flash",
		Input:                TurnInput{Text: "list the vault keys"},
		RequestedScope:       TurnScope{TerminalRead: true, SFTPRead: true},
	}
	encoded, err := Encode(request)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var decoded PrepareTurnRequest
	if err := Decode(encoded, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	sameWire := decoded.RequestID == request.RequestID &&
		decoded.ChatSessionID == request.ChatSessionID &&
		decoded.ExpectedChatRevision == request.ExpectedChatRevision &&
		decoded.AgentID == request.AgentID &&
		decoded.Input.Text == request.Input.Text &&
		decoded.RequestedScope == request.RequestedScope
	if !sameWire {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", decoded, request)
	}
	if !strings.Contains(string(encoded), `"18446744073709551615"`) {
		t.Fatalf("revision must travel as a decimal string: %s", encoded)
	}
	if !decoded.ChatSessionID.Valid() {
		t.Fatalf("fresh chat id must validate: %s", decoded.ChatSessionID)
	}
	if AgentID("agnt_short").Valid() {
		t.Fatal("short agent id must not validate")
	}
}

func TestAgentEventSequenceSurvivesUint64Max(t *testing.T) {
	envelope := AgentEventEnvelope{
		SchemaVersion: 1,
		InstanceID:    NewInstanceID(),
		ChatSessionID: newValidChatSessionID(t),
		TurnID:        NewTurnID(),
		Sequence:      "18446744073709551615",
		Type:          "turn_end",
		Backend:       "go-catty",
		TimestampMS:   1726473600000,
	}
	encoded, err := Encode(envelope)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var page EventPage
	if err := Decode([]byte(`{"events":[`+string(encoded)+`],"nextCursor":"18446744073709551615","hasMore":false}`), &page); err != nil {
		t.Fatalf("decode page: %v", err)
	}
	if len(page.Events) != 1 || page.Events[0].Sequence != "18446744073709551615" {
		t.Fatalf("sequence precision lost: %+v", page)
	}
	if page.NextCursor != "18446744073709551615" {
		t.Fatalf("cursor precision lost: %+v", page)
	}
}

func TestAgentEventRejectsUnknownField(t *testing.T) {
	raw := []byte(`{"schemaVersion":1,"instanceId":"x","chatSessionId":"y","turnId":"z","sequence":"1","type":"turn_start","backend":"go-catty","timestampMs":0,"vendorSecret":"nope"}`)
	var envelope AgentEventEnvelope
	if err := Decode(raw, &envelope); err == nil {
		t.Fatal("unknown envelope field must be rejected")
	}
}

func TestFakeDriverEmitsDeterministicEvents(t *testing.T) {
	driver := NewFakeTurnDriver()
	turn := driver.Prepare(PrepareTurnRequest{
		RequestID:     NewRequestID(),
		ChatSessionID: newValidChatSessionID(t),
		AgentID:       NewAgentID(),
		Input:         TurnInput{Text: "hi"},
	})
	if turn.TurnID == "" || turn.Cursor == "" {
		t.Fatalf("prepare must reserve a turn: %+v", turn)
	}
	if _, err := driver.Start(turn.TurnID); err != nil {
		t.Fatalf("start: %v", err)
	}
	page, err := driver.ReadEvents(turn.TurnID, "0", 100)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if len(page.Events) == 0 || page.Events[0].Type != "turn_start" {
		t.Fatalf("first event must be turn_start: %+v", page)
	}
	if _, err := driver.Stop(turn.TurnID, "user"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	page, err = driver.ReadEvents(turn.TurnID, page.NextCursor, 100)
	if err != nil {
		t.Fatalf("read after stop: %v", err)
	}
	types := make([]string, 0, len(page.Events))
	for _, event := range page.Events {
		types = append(types, event.Type)
	}
	if len(types) == 0 || types[len(types)-1] != "turn_end" {
		t.Fatalf("stop must end with turn_end: %v", types)
	}
}

func TestFakeDriverRejectsDoubleStart(t *testing.T) {
	driver := NewFakeTurnDriver()
	turn := driver.Prepare(PrepareTurnRequest{
		RequestID:     NewRequestID(),
		ChatSessionID: newValidChatSessionID(t),
		AgentID:       NewAgentID(),
		Input:         TurnInput{Text: "hi"},
	})
	if _, err := driver.Start(turn.TurnID); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if _, err := driver.Start(turn.TurnID); err != ErrBusy {
		t.Fatalf("second start must be busy, got %v", err)
	}
}

// TestAgentGoldenFixturesStable keeps one canonical wire payload per shape so
// future contract edits fail loudly instead of silently re-shaping the wire.
func TestAgentGoldenFixturesStable(t *testing.T) {
	chat := ChatSessionID(idPrefixChat + fixtureIDBody("c1"))
	turn := TurnID(idPrefixTurn + fixtureIDBody("t1"))
	fixtures := map[string]any{
		"agent-prepare.json": PrepareTurnRequest{
			RequestID:     RequestID(idPrefixRequest + fixtureIDBody("p1")),
			ChatSessionID: chat,
			AgentID:       AgentID(idPrefixAgent + fixtureIDBody("a1")),
			Input:         TurnInput{Text: "hello"},
			RequestedScope: TurnScope{
				TerminalRead: true,
			},
		},
		"agent-prepared.json": PreparedTurn{
			TurnID:                  turn,
			LeaseExpiresAtMS:        1726473600000,
			Cursor:                  "0",
			SnapshotRevision:        "1",
			EffectiveConfigRevision: "1",
			PolicyRevision:          "1",
		},
		"agent-event.json": AgentEventEnvelope{
			SchemaVersion: 1,
			InstanceID:    InstanceID(idPrefixInstance + fixtureIDBody("i1")),
			ChatSessionID: chat,
			TurnID:        turn,
			Sequence:      "1",
			Type:          "turn_start",
			Backend:       "go-catty",
			TimestampMS:   1726473600000,
		},
	}
	dir := filepath.Join("..", "..", "..", "testdata", "migration", "contracts")
	for name, value := range fixtures {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("%s: marshal: %v", name, err)
		}
		target := filepath.Join(dir, name)
		if err := os.WriteFile(target, append(encoded, '\n'), 0o644); err != nil {
			t.Fatalf("%s: write: %v", name, err)
		}
	}
}
