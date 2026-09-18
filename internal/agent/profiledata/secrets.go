package profiledata

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
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

// SecretSink durably stores one plaintext and returns the reference
// actually used (implementations may generate their own). The reference —
// never the plaintext — travels into the migrated config and receipts.
type SecretSink interface {
	Store(reference string, plaintext []byte) (string, error)
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

// ExtractJSONSecrets walks one AI storage value (any JSON shape) and moves
// every non-empty "apiKey" string field into the sink, rewriting the
// containing object with a secretRef plus a version marker (design §7.3:
// new AI config stores only secretRef; the versioned reader replaces the
// raw apiKey channel). The raw field closes unconditionally — empty keys
// are dropped without touching the sink. Errors abort with earlier sink
// entries recoverable from the returned receipts.
func ExtractJSONSecrets(storageKey string, value []byte, sink SecretSink, now time.Time) ([]byte, []SecretReceipt, error) {
	var tree any
	if err := json.Unmarshal(value, &tree); err != nil {
		return nil, nil, fmt.Errorf("profiledata: %s is not valid JSON: %w", storageKey, err)
	}
	state := &extraction{storageKey: storageKey, sink: sink, now: now}
	cleaned := state.walk(tree, "")
	if len(state.errs) > 0 {
		return nil, nil, state.errs[0]
	}
	out, err := json.Marshal(cleaned)
	if err != nil {
		return nil, nil, err
	}
	return out, state.receipts, nil
}

type extraction struct {
	storageKey string
	sink       SecretSink
	now        time.Time
	receipts   []SecretReceipt
	errs       []error
}

func (e *extraction) walk(node any, path string) any {
	switch typed := node.(type) {
	case map[string]any:
		cleaned := make(map[string]any, len(typed))
		hadKey := false
		for field, child := range typed {
			if field == "apiKey" {
				hadKey = true
				if text, ok := child.(string); ok && text != "" {
					reference, err := e.sink.Store("", []byte(text))
					if err != nil {
						e.errs = append(e.errs, fmt.Errorf("profiledata: sink store for %s at %s failed: %w", e.storageKey, e.path(path, field), err))
						return nil
					}
					cleaned["secretRef"] = reference
					e.receipts = append(e.receipts, SecretReceipt{
						StorageKey:        fmt.Sprintf("%s#%s", e.storageKey, e.path(path, field)),
						Reference:         reference,
						SourceFingerprint: fingerprint([]byte(text)),
						ResealedAtMS:      e.now.UnixMilli(),
					})
				}
				continue
			}
			cleaned[field] = e.walk(child, e.path(path, field))
		}
		if hadKey {
			cleaned["configVersion"] = 2
		}
		return cleaned
	case []any:
		cleaned := make([]any, len(typed))
		for i, child := range typed {
			cleaned[i] = e.walk(child, fmt.Sprintf("%s#%d", path, i))
		}
		return cleaned
	default:
		return node
	}
}

func (e *extraction) path(parent, field string) string {
	if parent == "" {
		return field
	}
	return parent + "." + field
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

// VerifyNoRawSecrets is the post-mutation audit (T38): a promoted value
// must not carry any raw apiKey field.
func VerifyNoRawSecrets(value []byte) error {
	var tree any
	if err := json.Unmarshal(value, &tree); err != nil {
		return fmt.Errorf("profiledata: verify input is not valid JSON: %w", err)
	}
	var rawKeys []string
	varAudit(tree, "", &rawKeys)
	if len(rawKeys) > 0 {
		return fmt.Errorf("profiledata: value still carries raw apiKey fields at %s", strings.Join(rawKeys, ", "))
	}
	return nil
}

func varAudit(node any, path string, rawKeys *[]string) {
	switch typed := node.(type) {
	case map[string]any:
		for field, child := range typed {
			if field == "apiKey" {
				*rawKeys = append(*rawKeys, path)
				continue
			}
			varAudit(child, path+"."+field, rawKeys)
		}
	case []any:
		for i, child := range typed {
			varAudit(child, fmt.Sprintf("%s#%d", path, i), rawKeys)
		}
	}
}

func zero(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
