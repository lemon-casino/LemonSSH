// Package permissions owns the plugin security boundary (P5-02A/P5-06):
// canonical resources, grant records and a fail-closed broker. Nothing here
// executes capability work — brokers only authorize and delegate to the
// shared Go application services.
package permissions

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotGranted       = errors.New("plugin permission not granted")
	ErrGrantExpired     = errors.New("plugin grant expired")
	ErrResourceInvalid  = errors.New("plugin resource invalid")
	ErrPrincipalUnknown = errors.New("plugin principal unknown")
)

// Lifetime of a grant.
type Lifetime string

const (
	LifetimeOnce        Lifetime = "once"
	LifetimeSession     Lifetime = "session"
	LifetimeApplication Lifetime = "application"
	LifetimeAlways      Lifetime = "always"
)

// Grant is one approved (principal, resource, lifetime) tuple.
type Grant struct {
	PrincipalID string
	Resource    string
	Lifetime    Lifetime
	GrantedAt   time.Time
	ExpiresAt   time.Time // zero for always/session
}

// Request describes one authorization ask.
type Request struct {
	PrincipalID string
	Resource    string
	Mode        string // "read" | "write"
}

// Broker is the fail-closed authorization surface shared by the WASM and
// native runtimes. Grants live only in memory for the session lifetime of
// the broker; durable grants are the profile store's concern (P5-06).
type Broker struct {
	mu     sync.Mutex
	grants map[string]map[string]Grant // principal -> resource -> grant
	clock  func() time.Time
}

func NewBroker(clock func() time.Time) *Broker {
	if clock == nil {
		clock = time.Now
	}
	return &Broker{grants: make(map[string]map[string]Grant), clock: clock}
}

// Grant records an approval.
func (b *Broker) Grant(principalID, resource string, lifetime Lifetime, ttl time.Duration) (Grant, error) {
	if err := validateResource(resource); err != nil {
		return Grant{}, err
	}
	if principalID == "" {
		return Grant{}, ErrPrincipalUnknown
	}
	switch lifetime {
	case LifetimeOnce, LifetimeSession, LifetimeApplication, LifetimeAlways:
	default:
		return Grant{}, fmt.Errorf("unknown lifetime %q", lifetime)
	}
	now := b.clock()
	grant := Grant{PrincipalID: principalID, Resource: resource, Lifetime: lifetime, GrantedAt: now}
	if ttl > 0 {
		grant.ExpiresAt = now.Add(ttl)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	byResource := b.grants[principalID]
	if byResource == nil {
		byResource = make(map[string]Grant)
		b.grants[principalID] = byResource
	}
	byResource[resource] = grant
	return grant, nil
}

// Check authorizes (principal, resource, mode). Default is deny: no grant,
// expired grant, or a write attempt over a read grant all fail.
func (b *Broker) Check(principalID, resource, mode string) error {
	if mode != "read" && mode != "write" {
		return ErrNotGranted
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	byResource, ok := b.grants[principalID]
	if !ok {
		return ErrNotGranted
	}
	grant, ok := byResource[resource]
	if !ok {
		return ErrNotGranted
	}
	if grant.ExpiresAt.IsZero() == false && b.clock().After(grant.ExpiresAt) {
		return ErrGrantExpired
	}
	if mode == "write" && !strings.HasSuffix(grant.Resource, ":write") && !grant.writeAllowed() {
		return ErrNotGranted
	}
	if grant.Lifetime == LifetimeOnce {
		delete(byResource, resource)
	}
	return nil
}

// Revoke drops one grant.
func (b *Broker) Revoke(principalID, resource string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if byResource, ok := b.grants[principalID]; ok {
		delete(byResource, resource)
	}
}

// RevokeAll drops every grant of a principal (uninstall/quarantine path).
func (b *Broker) RevokeAll(principalID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.grants, principalID)
}

func validateResource(resource string) error {
	if strings.TrimSpace(resource) == "" || len(resource) > 256 {
		return ErrResourceInvalid
	}
	return nil
}

// writeAllowed is encoded into the resource suffix by the caller boundary
// (resource strings carry ":read" / ":write" trailing scopes).
func (g Grant) writeAllowed() bool { return strings.HasSuffix(g.Resource, ":write") }
