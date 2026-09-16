package contracts

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
)

// Opaque identity types. The string forms are prefixed and length-bounded so
// shells and tests can validate them cheaply; no component may derive meaning
// from anything beyond the prefix.
const (
	idPrefixInstance = "inst_"
	idPrefixWindow   = "win_"
	idPrefixSession  = "ses_"
	idPrefixRequest  = "req_"
	idPrefixChat     = "chat_"
	idPrefixTurn     = "turn_"
	idPrefixAgent    = "agnt_"
)

// idBodyHex is the number of random hex characters after the prefix.
const idBodyHex = 24

var idBodyPattern = regexp.MustCompile(`^[0-9a-f]{` + fmt.Sprint(idBodyHex) + `}$`)

// InstanceID identifies one running application instance.
type InstanceID string

// WindowID identifies one window within an instance.
type WindowID string

// SessionID identifies one terminal session.
type SessionID string

// RequestID correlates one request/response pair.
type RequestID string

// ChatSessionID identifies one AI chat session.
type ChatSessionID string

// TurnID identifies one prepared or running AI turn.
type TurnID string

// AgentID identifies one agent kind or adapter.
type AgentID string

func newPrefixedID(prefix string) string {
	body := make([]byte, idBodyHex/2)
	if _, err := rand.Read(body); err != nil {
		// crypto/rand failure is unrecoverable for identity purposes.
		panic(fmt.Sprintf("contracts: identity generation failed: %v", err))
	}
	return prefix + hex.EncodeToString(body)
}

// NewInstanceID mints a fresh instance identity.
func NewInstanceID() InstanceID { return InstanceID(newPrefixedID(idPrefixInstance)) }

// NewWindowID mints a fresh window identity.
func NewWindowID() WindowID { return WindowID(newPrefixedID(idPrefixWindow)) }

// NewSessionID mints a fresh session identity.
func NewSessionID() SessionID { return SessionID(newPrefixedID(idPrefixSession)) }

// NewRequestID mints a fresh request correlation identity.
func NewRequestID() RequestID { return RequestID(newPrefixedID(idPrefixRequest)) }

// NewChatSessionID mints a fresh chat session identity.
func NewChatSessionID() ChatSessionID { return ChatSessionID(newPrefixedID(idPrefixChat)) }

// NewTurnID mints a fresh turn identity.
func NewTurnID() TurnID { return TurnID(newPrefixedID(idPrefixTurn)) }

// NewAgentID mints a fresh agent identity.
func NewAgentID() AgentID { return AgentID(newPrefixedID(idPrefixAgent)) }

// Valid reports whether the ID has the expected prefix and body shape.
func (id InstanceID) Valid() bool { return validateID(idPrefixInstance, string(id)) }

// Valid reports whether the ID has the expected prefix and body shape.
func (id WindowID) Valid() bool { return validateID(idPrefixWindow, string(id)) }

// Valid reports whether the ID has the expected prefix and body shape.
func (id SessionID) Valid() bool { return validateID(idPrefixSession, string(id)) }

// Valid reports whether the ID has the expected prefix and body shape.
func (id RequestID) Valid() bool { return validateID(idPrefixRequest, string(id)) }

// Valid reports whether the ID has the expected prefix and body shape.
func (id ChatSessionID) Valid() bool { return validateID(idPrefixChat, string(id)) }

// Valid reports whether the ID has the expected prefix and body shape.
func (id TurnID) Valid() bool { return validateID(idPrefixTurn, string(id)) }

// Valid reports whether the ID has the expected prefix and body shape.
func (id AgentID) Valid() bool { return validateID(idPrefixAgent, string(id)) }

func validateID(prefix, value string) bool {
	if len(value) != len(prefix)+idBodyHex || value[:len(prefix)] != prefix {
		return false
	}
	return idBodyPattern.MatchString(value[len(prefix):])
}
