package credentials

import keyring "github.com/zalando/go-keyring"

// OSKeyring adapts go-keyring's platform implementations to Keyring.
type OSKeyring struct{}

func (OSKeyring) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}
func (OSKeyring) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}
func (OSKeyring) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

// NewOSProvider constructs the real platform provider. It remains unavailable
// when the platform keyring cannot be reached; callers must not fall back to
// plaintext or localStorage.
func NewOSProvider() Provider { return New(OSKeyring{}) }
