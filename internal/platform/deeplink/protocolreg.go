// OS protocol registration logic (P4-04, SYS-03). The schemes ssh://,
// telnet:// and lemonssh:// (plus the legacy netcatty://) map onto the same
// strict intent parser the app already uses for deep links. The registry
// surface is injected so the write logic is testable without touching the
// real user hive.
package deeplink

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ProtocolSchemes are the URL schemes LemonSSH owns on the OS.
var ProtocolSchemes = []string{"ssh", "telnet", "lemonssh"}

// LegacyProtocolSchemes are schemes registered by previous releases. They
// stay registered (enable writes both sets, disable removes both) so
// upgrading users keep working netcatty:// handoffs and downgrades remain
// functional. Registered-status logic accepts either complete set.
var LegacyProtocolSchemes = []string{"netcatty"}

// RegistryStore is the minimal hive surface protocol registration needs.
// Key paths use backslash separators relative to the Classes root the store
// implements (on Windows: HKCU\Software\Classes).
type RegistryStore interface {
	// SetStringValue writes (keyPath, valueName) -> value. An empty
	// valueName is the key's default value.
	SetStringValue(keyPath, valueName, value string) error
	// DeleteTree removes keyPath and everything under it.
	DeleteTree(keyPath string) error
	// GetString reads (keyPath, valueName); missing yields "" and nil error.
	GetString(keyPath, valueName string) (string, error)
}

// classesRoot is the hive prefix every scheme key lives under.
const classesRoot = "Software\\Classes"

// ProtocolSpecs returns the registry writes that make the OS hand a URL to
// LemonSSH: the scheme key carries "URL Protocol", and the open command
// launches this executable with the URL quoted. Both the current and the
// legacy schemes are covered so enable/registration stays compatible.
func ProtocolSpecs(exePath string) ([]struct{ KeyPath, ValueName, Value string }, error) {
	return protocolSpecsForSchemes(exePath, append(append([]string{}, ProtocolSchemes...), LegacyProtocolSchemes...))
}

// protocolSpecsForSchemes builds the registry writes for one scheme set.
func protocolSpecsForSchemes(exePath string, schemes []string) ([]struct{ KeyPath, ValueName, Value string }, error) {
	trimmed := strings.TrimSpace(exePath)
	if trimmed == "" {
		return nil, fmt.Errorf("protocol registration requires the executable path")
	}
	absolute, err := filepath.Abs(trimmed)
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}
	command := fmt.Sprintf(`"%s" "%%1"`, absolute)
	specs := make([]struct{ KeyPath, ValueName, Value string }, 0, len(schemes)*2)
	for _, scheme := range schemes {
		root := classesRoot + "\\" + scheme
		specs = append(specs,
			struct{ KeyPath, ValueName, Value string }{root, "URL Protocol", ""},
			struct{ KeyPath, ValueName, Value string }{root + "\\shell\\open\\command", "", command},
		)
	}
	return specs, nil
}

// SetOSProtocols enables or removes OS handoff for every scheme. Enabling is
// idempotent and writes both the current and the legacy schemes; disabling
// removes both scheme sets entirely.
func SetOSProtocols(store RegistryStore, exePath string, enabled bool) error {
	specs, err := ProtocolSpecs(exePath)
	if err != nil {
		return err
	}
	if !enabled {
		for _, scheme := range append(append([]string{}, ProtocolSchemes...), LegacyProtocolSchemes...) {
			if err := store.DeleteTree(classesRoot + "\\" + scheme); err != nil {
				return fmt.Errorf("remove %s registration: %w", scheme, err)
			}
		}
		return nil
	}
	for _, spec := range specs {
		if err := store.SetStringValue(spec.KeyPath, spec.ValueName, spec.Value); err != nil {
			return fmt.Errorf("write %s: %w", spec.KeyPath, err)
		}
	}
	return nil
}

// OSProtocolsRegistered reports whether the OS hands URLs off to this
// executable: the complete current scheme set, or — only when no current
// scheme key was ever written (a true pre-rename install) — the complete
// legacy set pointing at this executable. The legacy fallback keeps users who
// only ever registered netcatty:// from seeing the toggle as off after an
// upgrade (which would push them into re-enabling for nothing). It must not
// mask drift: when a current scheme command exists but points elsewhere, the
// registration reads as stale so the user re-enables.
func OSProtocolsRegistered(store RegistryStore, exePath string) bool {
	if schemesRegistered(store, exePath, ProtocolSchemes) {
		return true
	}
	if anySchemeCommandPresent(store, ProtocolSchemes) {
		return false
	}
	return schemesRegistered(store, exePath, LegacyProtocolSchemes)
}

// anySchemeCommandPresent reports whether any current scheme carries an open
// command (i.e. its registry key exists at all).
func anySchemeCommandPresent(store RegistryStore, schemes []string) bool {
	for _, scheme := range schemes {
		if command, _ := store.GetString(classesRoot+"\\"+scheme+"\\shell\\open\\command", ""); command != "" {
			return true
		}
	}
	return false
}

func schemesRegistered(store RegistryStore, exePath string, schemes []string) bool {
	specs, err := protocolSpecsForSchemes(exePath, schemes)
	if err != nil {
		return false
	}
	for _, spec := range specs {
		current, err := store.GetString(spec.KeyPath, spec.ValueName)
		if err != nil || current != spec.Value {
			return false
		}
	}
	return true
}
