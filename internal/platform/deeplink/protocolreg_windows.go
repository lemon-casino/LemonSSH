//go:build windows

package deeplink

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// WindowsRegistryStore persists protocol registration under
// HKCU\Software\Classes, which needs no elevation and no installer.
type WindowsRegistryStore struct{}

// NewRegistryStore returns the platform registry store.
func NewRegistryStore() RegistryStore { return WindowsRegistryStore{} }

func openWritable(keyPath string) (registry.Key, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err == nil {
		return key, nil
	}
	key, _, err = registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", keyPath, err)
	}
	return key, nil
}

func (WindowsRegistryStore) SetStringValue(keyPath, valueName, value string) error {
	key, err := openWritable(keyPath)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(valueName, value)
}

func (WindowsRegistryStore) DeleteTree(keyPath string) error {
	if err := registry.DeleteKey(registry.CURRENT_USER, keyPath); err != nil {
		// Already absent is success for the disable path.
		if err == registry.ErrNotExist {
			return nil
		}
		// A key with subkeys (shell\open\command) must be deleted deepest first.
		if err := registry.DeleteKey(registry.CURRENT_USER, keyPath+"\\shell\\open\\command"); err != nil {
			return fmt.Errorf("delete %s: %w", keyPath, err)
		}
		if err := registry.DeleteKey(registry.CURRENT_USER, keyPath+"\\shell\\open"); err != nil {
			return fmt.Errorf("delete %s: %w", keyPath, err)
		}
		if err := registry.DeleteKey(registry.CURRENT_USER, keyPath+"\\shell"); err != nil {
			return fmt.Errorf("delete %s: %w", keyPath, err)
		}
		if err := registry.DeleteKey(registry.CURRENT_USER, keyPath); err != nil {
			return fmt.Errorf("delete %s: %w", keyPath, err)
		}
	}
	return nil
}

func (WindowsRegistryStore) GetString(keyPath, valueName string) (string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer key.Close()
	value, _, err := key.GetStringValue(valueName)
	if err != nil {
		return "", err
	}
	return value, nil
}
