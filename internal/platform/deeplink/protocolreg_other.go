//go:build !windows

package deeplink

import "errors"

// ErrUnsupportedPlatform reports that OS protocol handoff is only implemented
// for the Windows registry; macOS and Linux registration belongs to their
// installer formats (Info.plist, .desktop).
var ErrUnsupportedPlatform = errors.New("deeplink: OS protocol registration is Windows-only; use the platform installer format")

// UnsupportedRegistryStore always fails closed.
type UnsupportedRegistryStore struct{}

// NewRegistryStore returns the fail-closed store for non-Windows platforms.
func NewRegistryStore() RegistryStore { return UnsupportedRegistryStore{} }

func (UnsupportedRegistryStore) SetStringValue(string, string, string) error {
	return ErrUnsupportedPlatform
}

func (UnsupportedRegistryStore) DeleteTree(string) error { return ErrUnsupportedPlatform }

func (UnsupportedRegistryStore) GetString(string, string) (string, error) {
	return "", ErrUnsupportedPlatform
}
