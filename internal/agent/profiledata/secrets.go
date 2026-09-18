package profiledata

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// AIPurpose is the dedicated credential purpose for AI provider secrets
// (design §7.3). Opening an envelope under any other purpose fails with
// ErrPurposeMismatch in the credential provider — the T38 backstop.
const AIPurpose = "ai-provider-secrets"

// SecretOrigin identifies which broker produced an enc:v1 envelope. The
// prefix alone cannot (design §7.3): the export manifest origin (or an
// FND-03 receipt) must say.
type SecretOrigin string

const (
	// OriginElectronBroker marks envelopes produced by the Electron
	// safeStorage broker (FND-03 receipts).
	OriginElectronBroker SecretOrigin = "electron-safeStorage"
	// OriginGoCredential marks envelopes produced by the Go credential
	// provider under a known purpose.
	OriginGoCredential SecretOrigin = "go-credential-provider"
)

// BlockedOriginError aborts promotion when the origin is unknown. The
// repairable reason is surfaced to the user; the value is never opened
// speculatively and never silently emptied (design §7.3).
type BlockedOriginError struct {
	StorageKey string
	Origin     SecretOrigin
}

func (e *BlockedOriginError) Error() string {
	return fmt.Sprintf("profiledata: %s has unverifiable secret origin %q; resolve the export manifest origin before promoting", e.StorageKey, e.Origin)
}

// SecretCodec is the origin-aware open/seal seam. Production impls wrap the
// Electron broker bridge and the Go credential provider (AI purpose).
type SecretCodec interface {
	// Open decrypts an enc:v1 envelope produced by the given origin.
	Open(origin SecretOrigin, envelope []byte) ([]byte, error)
	// Seal encrypts plaintext under the dedicated AI purpose.
	Seal(plaintext []byte) ([]byte, error)
}

// SecretSink durably stores one plaintext under a fresh reference. The
// reference (never the plaintext) travels into the migrated config.
type SecretSink interface {
	Store(reference string, plaintext []byte) error
}

// SecretReceipt records one completed extraction or reseal. It never
// carries plaintext (design §7.2 step 3).
type SecretReceipt struct {
	StorageKey        string       `json:"storageKey"`
	Reference         string       `json:"reference,omitempty"`
	Origin            SecretOrigin `json:"origin,omitempty"`
	SourceFingerprint string       `json:"sourceFingerprint,omitempty"`
	SealedFingerprint string       `json:"sealedFingerprint,omitempty"`
	ResealedAtMS      int64        `json:"resealedAtMs"`
}

func fingerprint(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func newSecretRef() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic("profiledata: reference entropy unavailable: " + err.Error())
	}
	return "secret_" + hex.EncodeToString(raw[:])
}

// ExtractProviderSecrets walks a netcatty_ai_providers_v1 value, moves every
// non-empty apiKey into the sink, and rewrites the config with a secretRef
// plus a version marker (design §7.3: new AI config stores only secretRef;
// the versioned reader replaces the raw apiKey channel). Errors abort with
// the value untouched; the sink may hold earlier entries, whose references
// are in the returned receipts for re-entrancy.
func ExtractProviderSecrets(storageKey string, providersValue []byte, sink SecretSink, now time.Time) ([]byte, []SecretReceipt, error) {
	var configs []map[string]json.RawMessage
	if err := json.Unmarshal(providersValue, &configs); err != nil {
		return nil, nil, fmt.Errorf("profiledata: %s is not a provider config array: %w", storageKey, err)
	}

	receipts := []SecretReceipt{}
	for i, config := range configs {
		rawKey, present := config["apiKey"]
		if !present {
			continue // already migrated or never had a key
		}
		var apiKey string
		_ = json.Unmarshal(rawKey, &apiKey)

		// The raw channel closes unconditionally: empty keys are dropped
		// without touching the sink, non-empty ones move into it.
		reference := ""
		if apiKey != "" {
			var configID string
			_ = json.Unmarshal(config["id"], &configID)
			reference = newSecretRef()
			if err := sink.Store(reference, []byte(apiKey)); err != nil {
				return nil, nil, fmt.Errorf("profiledata: sink store for %s[%d] failed: %w", storageKey, i, err)
			}
			receipts = append(receipts, SecretReceipt{
				StorageKey:        fmt.Sprintf("%s#%d(%s)", storageKey, i, configID),
				Reference:         reference,
				SourceFingerprint: fingerprint([]byte(apiKey)),
				ResealedAtMS:      now.UnixMilli(),
			})
		}

		rewritten, err := rewriteConfig(config, reference)
		if err != nil {
			return nil, nil, err
		}
		configs[i] = rewritten
	}

	out, err := json.Marshal(configs)
	if err != nil {
		return nil, nil, err
	}
	if len(receipts) == 0 {
		return out, nil, nil
	}
	return out, receipts, nil
}

func rewriteConfig(config map[string]json.RawMessage, reference string) (map[string]json.RawMessage, error) {
	rewritten := make(map[string]json.RawMessage, len(config)+1)
	for field, value := range config {
		if field == "apiKey" {
			continue // the raw key channel closes (T38)
		}
		rewritten[field] = value
	}
	if reference != "" {
		rewritten["secretRef"] = json.RawMessage(fmt.Sprintf("%q", reference))
	}
	rewritten["configVersion"] = json.RawMessage("2")
	return rewritten, nil
}

// VerifyNoRawSecrets is the post-mutation audit (T38): a promoted
// providers value must not carry any raw apiKey field.
func VerifyNoRawSecrets(providersValue []byte) error {
	var configs []map[string]json.RawMessage
	if err := json.Unmarshal(providersValue, &configs); err != nil {
		return fmt.Errorf("profiledata: verify input is not a provider config array: %w", err)
	}
	for i, config := range configs {
		if _, present := config["apiKey"]; present {
			return fmt.Errorf("profiledata: provider config %d still carries a raw apiKey", i)
		}
	}
	return nil
}

// ResealOpaqueSecret re-encrypts one opaque enc:v1 envelope under the AI
// purpose via the origin-aware codec, returning the resealed value and a
// plaintext-free receipt. Unknown origins block (never speculative open).
func ResealOpaqueSecret(storageKey string, envelope []byte, origin SecretOrigin, codec SecretCodec, now time.Time) ([]byte, SecretReceipt, error) {
	switch origin {
	case OriginElectronBroker, OriginGoCredential:
	default:
		return nil, SecretReceipt{}, &BlockedOriginError{StorageKey: storageKey, Origin: origin}
	}

	plaintext, err := codec.Open(origin, envelope)
	if err != nil {
		return nil, SecretReceipt{}, fmt.Errorf("profiledata: %s open via %s failed: %w", storageKey, origin, err)
	}
	defer zero(plaintext)

	sealed, err := codec.Seal(plaintext)
	if err != nil {
		return nil, SecretReceipt{}, fmt.Errorf("profiledata: %s reseal failed: %w", storageKey, err)
	}
	receipt := SecretReceipt{
		StorageKey:        storageKey,
		Origin:            origin,
		SourceFingerprint: fingerprint(envelope),
		SealedFingerprint: fingerprint(sealed),
		ResealedAtMS:      now.UnixMilli(),
	}
	return sealed, receipt, nil
}

func zero(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
