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

// Valid reports whether the ID has the expected prefix and body shape.
func (id InstanceID) Valid() bool { return validateID(idPrefixInstance, string(id)) }

// Valid reports whether the ID has the expected prefix and body shape.
func (id WindowID) Valid() bool { return validateID(idPrefixWindow, string(id)) }

// Valid reports whether the ID has the expected prefix and body shape.
func (id SessionID) Valid() bool { return validateID(idPrefixSession, string(id)) }

// Valid reports whether the ID has the expected prefix and body shape.
func (id RequestID) Valid() bool { return validateID(idPrefixRequest, string(id)) }

func validateID(prefix, value string) bool {
	if len(value) != len(prefix)+idBodyHex || value[:len(prefix)] != prefix {
		return false
	}
	return idBodyPattern.MatchString(value[len(prefix):])
}
