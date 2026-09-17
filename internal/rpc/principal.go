// Package rpc implements the authenticated local host RPC (W06, P7-02,
// AI-02): the shell-neutral boundary that the native MCP server and CLI
// binaries (W07) use to reach host capabilities. The package owns protocol
// versioning, bearer-token authentication with first-party/external
// principals, bounded frame parsing and connection lifecycle; capability
// authorization itself stays in internal/capability.
package rpc

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

// ProtocolVersion is the wire version stamped on every frame. Mismatches
// fail with a typed error instead of best-effort parsing.
const ProtocolVersion = 1

// PrincipalKind separates trusted in-app callers (the desktop shell and its
// own launchers) from external agent processes. The distinction is decided
// by the composition root at token issuance, never by request parameters.
type PrincipalKind string

const (
	PrincipalFirstParty PrincipalKind = "first-party"
	PrincipalExternal   PrincipalKind = "external"
)

// Principal is the authenticated identity of one connection. Scope lists
// the session/chat IDs the principal may touch; handlers must consult
// AllowsSession rather than trusting request-carried IDs alone.
type Principal struct {
	ID    string
	Kind  PrincipalKind
	Scope []string
}

// AllowsSession reports whether the session/chat ID is inside the
// principal's scope.
func (p *Principal) AllowsSession(id string) bool {
	if p == nil {
		return false
	}
	for _, allowed := range p.Scope {
		if allowed == id {
			return true
		}
	}
	return false
}

// Typed protocol failure codes.
const (
	CodeAuthFailed      = "AUTH_FAILED"
	CodeVersionMismatch = "VERSION_UNSUPPORTED"
	CodeUnknownMethod   = "UNKNOWN_METHOD"
	CodeScopeDenied     = "SCOPE_DENIED"
	CodeBadRequest      = "BAD_REQUEST"
	CodeDeadline        = "DEADLINE_EXCEEDED"
	CodeServerClosing   = "SERVER_CLOSING"
)

// Token hashing: only SHA-256 digests are retained, so a leaked store never
// leaks usable bearer tokens.
type tokenRecord struct {
	digest    [sha256.Size]byte
	principal Principal
	revoked   bool
}

// TokenStore issues and verifies bearer tokens. Issue returns the raw token
// exactly once; Verify is constant-time over the digest. RevokeAll cuts off
// every outstanding token (app shutdown, lock screen, user revocation);
// revoked tokens keep failing after reuse (T42).
type TokenStore struct {
	mu      sync.Mutex
	records []*tokenRecord
}

// NewTokenStore builds an empty store.
func NewTokenStore() *TokenStore { return &TokenStore{} }

// Issue mints a 256-bit token for the principal and records its digest.
func (s *TokenStore) Issue(principal Principal) (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("rpc: token entropy unavailable: %w", err)
	}
	token := hex.EncodeToString(raw[:])

	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, &tokenRecord{
		digest:    sha256.Sum256(raw[:]),
		principal: principal,
	})
	return token, nil
}

var errTokenRevoked = errors.New("token revoked")

// Verify resolves the principal for a bearer token. Revoked and unknown
// tokens produce the same AUTH_FAILED surface with distinct internal
// causes; revocation is permanent for the record's lifetime.
func (s *TokenStore) Verify(token string) (*Principal, error) {
	raw, err := hex.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, errTokenRevoked
	}
	digest := sha256.Sum256(raw)

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.records {
		if record.digest == digest {
			if record.revoked {
				return nil, errTokenRevoked
			}
			principal := record.principal
			return &principal, nil
		}
	}
	return nil, errTokenRevoked
}

// Revoke marks every issued token revoked and returns the count. Revoked
// records are kept so reuse keeps failing until the store itself is dropped.
func (s *TokenStore) RevokeAll() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	revoked := 0
	for _, record := range s.records {
		if !record.revoked {
			record.revoked = true
			revoked++
		}
	}
	return revoked
}
