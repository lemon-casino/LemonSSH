package profiledata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/binaricat/netcatty/internal/profile/store"
)

// AISchemaMarker stamps a promoted snapshot as AI-canonical v1.
const AISchemaMarker = "ai-canonical-v1"

// migrationReceiptKey lands in the device domain: migration receipts are
// device-local metadata and never sync (T40).
const migrationReceiptKey = "ai/migration-receipt"

// SnapshotWriter is the profile-store transaction surface the promotion
// needs. Concrete stores satisfy it; tests substitute spies.
type SnapshotWriter interface {
	Write(request store.WriteRequest) (store.WriteResult, error)
}

// PromotionDeps wires one promotion run.
type PromotionDeps struct {
	Store  SnapshotWriter
	Sink   SecretSink
	Codec  SecretCodec
	Origin SecretOrigin
	Now    func() time.Time
}

// PromotionReceipt is the plaintext-free record persisted beside the data.
type PromotionReceipt struct {
	PromotedAtMS int64           `json:"promotedAtMs"`
	Keys         []string        `json:"keys"`
	Secrets      []SecretReceipt `json:"secrets,omitempty"`
	SchemaMarker string          `json:"schemaMarker"`
}

// PromoteAISnapshot migrates one renderer localStorage snapshot into the
// profile store in a single all-or-nothing transaction (design §7.2 steps
// 2-5 for the store-level scope). Secrets are extracted or resealed BEFORE
// the write; the receipt lands in the same transaction, so an interruption
// leaves either the old state or the fully promoted one — a re-run simply
// repeats the idempotent write (T36).
func PromoteAISnapshot(snapshot map[string]json.RawMessage, deps PromotionDeps) (*PromotionReceipt, error) {
	if deps.Store == nil {
		return nil, fmt.Errorf("profiledata: promotion requires a store")
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}

	working := make(map[string]json.RawMessage, len(snapshot))
	for key, value := range snapshot {
		working[key] = value
	}
	var secretReceipts []SecretReceipt

	// Secret-bearing values come in two shapes: structured JSON configs
	// (providers, web search — extract apiKey fields to references) and
	// opaque enc:v1 envelopes (reseal via the manifest-declared origin,
	// blocking when unknown — T37).
	for _, storageKey := range secretBearingKeys(working) {
		if bytes.HasPrefix(bytes.TrimSpace(working[storageKey]), encV1Prefix) {
			sealed, receipt, err := ResealOpaqueSecret(storageKey, working[storageKey], deps.Origin, deps.Codec, deps.Now())
			if err != nil {
				return nil, err
			}
			working[storageKey] = sealed
			secretReceipts = append(secretReceipts, receipt)
			continue
		}
		rewritten, receipts, err := ExtractJSONSecrets(storageKey, working[storageKey], deps.Sink, deps.Now())
		if err != nil {
			return nil, err
		}
		working[storageKey] = rewritten
		secretReceipts = append(secretReceipts, receipts...)
	}

	mutations, report, err := PlanMigrations(working)
	if err != nil {
		return nil, err
	}
	if len(report.SecretBearing) > 0 {
		return nil, fmt.Errorf("profiledata: secret-bearing values survived planning: %s", strings.Join(report.SecretBearing, ", "))
	}

	now := deps.Now()
	receipt := PromotionReceipt{
		PromotedAtMS: now.UnixMilli(),
		SchemaMarker: AISchemaMarker,
		Secrets:      secretReceipts,
	}
	for _, mutation := range mutations {
		receipt.Keys = append(receipt.Keys, mutation.Key)
	}
	receiptRaw, err := json.Marshal(receipt)
	if err != nil {
		return nil, err
	}
	mutations = append(mutations, store.Mutation{
		Domain: "device",
		Key:    migrationReceiptKey,
		Value:  receiptRaw,
	})

	if _, err := deps.Store.Write(store.WriteRequest{Mutations: mutations}); err != nil {
		return nil, fmt.Errorf("profiledata: promotion transaction failed: %w", err)
	}
	return &receipt, nil
}

// secretBearingKeys lists snapshot keys the inventory flags secret-bearing.
// Sorted iteration keeps failure order deterministic.
func secretBearingKeys(values map[string]json.RawMessage) []string {
	var keys []string
	for key := range values {
		if spec := Spec(key); spec != nil && spec.SecretBearing {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}
