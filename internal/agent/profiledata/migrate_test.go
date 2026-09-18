package profiledata

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/profile/store"
)

// spyStore records Write calls without touching disk.
type spyStore struct {
	mu       sync.Mutex
	writes   []store.WriteRequest
	failNext error
}

func (s *spyStore) Write(request store.WriteRequest) (store.WriteResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failNext != nil {
		return store.WriteResult{}, s.failNext
	}
	s.writes = append(s.writes, request)
	return store.WriteResult{Revision: uint64(len(s.writes))}, nil
}

func (s *spyStore) writeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.writes)
}

// failingSink aborts after N successful stores, simulating a mid-migration
// credential failure (T36 interruption point).
type failingSink struct {
	memSink
	failAfter int
}

func (s *failingSink) Store(reference string, plaintext []byte) (string, error) {
	if len(s.values) >= s.failAfter {
		return "", errors.New("keyring locked mid-flight")
	}
	return s.memSink.Store(reference, plaintext)
}

func TestPromoteAISnapshotHappyPath(t *testing.T) {
	storeSpy := &spyStore{}
	sink := newMemSink()
	deps := PromotionDeps{
		Store:  storeSpy,
		Sink:   sink,
		Origin: OriginElectronBroker,
		Codec:  fakeCodec{openOrigin: OriginElectronBroker},
		Now:    func() time.Time { return testNow },
	}
	snapshot := map[string]json.RawMessage{
		"netcatty_ai_providers_v1":          []byte(`[{"id":"p1","apiKey":"sk-live-9"}]`),
		"netcatty_ai_permission_mode_v1":    []byte(`"confirm"`),
		"netcatty_ai_active_session_map_v1": []byte(`{"w":"s"}`),
	}

	receipt, err := PromoteAISnapshot(snapshot, deps)
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if receipt.SchemaMarker != AISchemaMarker {
		t.Errorf("schema marker = %q", receipt.SchemaMarker)
	}
	if len(receipt.Secrets) != 1 || receipt.Secrets[0].Reference == "" {
		t.Errorf("secret receipts = %+v", receipt.Secrets)
	}
	if storeSpy.writeCount() != 1 {
		t.Fatalf("promotion must be one transaction, got %d writes", storeSpy.writeCount())
	}

	written := storeSpy.writes[0]
	if len(written.Mutations) < 4 {
		t.Fatalf("mutations = %d, want providers + mode + map + receipt", len(written.Mutations))
	}
	var receiptOnWire PromotionReceipt
	var sawReceipt bool
	for _, mutation := range written.Mutations {
		if mutation.Key == migrationReceiptKey {
			sawReceipt = true
			if mutation.Domain != "device" {
				t.Errorf("receipt must be device-local (T40), got %q", mutation.Domain)
			}
			if err := json.Unmarshal(mutation.Value, &receiptOnWire); err != nil {
				t.Fatalf("receipt unmarshal: %v", err)
			}
		}
		if strings.Contains(string(mutation.Value), "sk-live-9") {
			t.Errorf("mutation %s/%s carries plaintext", mutation.Domain, mutation.Key)
		}
	}
	if !sawReceipt {
		t.Fatal("receipt mutation missing")
	}
	if receiptOnWire.SchemaMarker != AISchemaMarker {
		t.Errorf("persisted receipt = %+v", receiptOnWire)
	}

	// The stored providers value must pass the raw-secret audit.
	for _, mutation := range written.Mutations {
		if mutation.Key == "ai/providers_v1" {
			if err := VerifyNoRawSecrets(mutation.Value); err != nil {
				t.Errorf("stored providers value: %v", err)
			}
			if !strings.Contains(string(mutation.Value), "secret_0001") {
				t.Errorf("stored value must carry the reference: %s", mutation.Value)
			}
		}
	}
}

// TestPromoteAISnapshotSinkFailureAtomic pins T36: a mid-flight sink
// failure aborts before any store write, leaving the old state intact.
func TestPromoteAISnapshotSinkFailureAtomic(t *testing.T) {
	storeSpy := &spyStore{}
	sink := &failingSink{failAfter: 0}
	deps := PromotionDeps{
		Store:  storeSpy,
		Sink:   sink,
		Origin: OriginElectronBroker,
		Codec:  fakeCodec{openOrigin: OriginElectronBroker},
	}
	snapshot := map[string]json.RawMessage{
		"netcatty_ai_providers_v1": []byte(`[{"id":"p1","apiKey":"sk-1"}]`),
	}
	if _, err := PromoteAISnapshot(snapshot, deps); err == nil {
		t.Fatal("sink failure must abort promotion")
	}
	if storeSpy.writeCount() != 0 {
		t.Errorf("failed promotion must not write, got %d writes", storeSpy.writeCount())
	}
}

// TestPromoteAISnapshotBlockedOrigin pins the T37 fail-closed path: an
// unknown origin on a secret-bearing opaque value aborts before writing.
func TestPromoteAISnapshotBlockedOrigin(t *testing.T) {
	storeSpy := &spyStore{}
	sink := newMemSink()
	deps := PromotionDeps{
		Store:  storeSpy,
		Sink:   sink,
		Origin: SecretOrigin("who-knows"),
		Codec:  fakeCodec{openOrigin: OriginGoCredential},
	}
	// The providers value is JSON so extraction succeeds without the codec;
	// the web search value holds an opaque envelope requiring the codec.
	snapshot := map[string]json.RawMessage{
		"netcatty_ai_web_search_v1": []byte("enc:v1:opaque-payload"),
	}
	_, err := PromoteAISnapshot(snapshot, deps)
	if err == nil {
		t.Fatal("unknown origin must block promotion")
	}
	var blocked *BlockedOriginError
	if _, ok := err.(*BlockedOriginError); !ok {
		t.Fatalf("error must be BlockedOriginError, got %v", err)
	}
	_ = blocked
	if storeSpy.writeCount() != 0 {
		t.Errorf("blocked promotion must not write, got %d", storeSpy.writeCount())
	}
}

func TestPromoteAISnapshotReceiptIsDeviceLocal(t *testing.T) {
	storeSpy := &spyStore{}
	sink := newMemSink()
	deps := PromotionDeps{Store: storeSpy, Sink: sink, Origin: OriginElectronBroker}
	snapshot := map[string]json.RawMessage{
		"netcatty_ai_permission_mode_v1": []byte(`"auto"`),
	}
	receipt, err := PromoteAISnapshot(snapshot, deps)
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	found := false
	for _, mutation := range storeSpy.writes[0].Mutations {
		if mutation.Key == migrationReceiptKey {
			found = true
			if mutation.Domain != "device" {
				t.Errorf("receipt domain = %q, want device (T40)", mutation.Domain)
			}
		}
	}
	if !found {
		t.Fatal("receipt not persisted")
	}
	if receipt.Keys[0] != "ai/permission_mode_v1" {
		t.Errorf("receipt keys = %v", receipt.Keys)
	}
}
