// OS protocol registration logic (P4-04, SYS-03). The schemes ssh://,
// telnet:// and netcatty:// map onto the same strict intent parser the app
// already uses for deep links. The registry surface is injected so the write
// logic is testable without touching the real user hive.
package deeplink

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ProtocolSchemes are the URL schemes LemonSSH can own on the OS.
var ProtocolSchemes = []string{"ssh", "telnet", "netcatty"}

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
// launches this executable with the URL quoted.
func ProtocolSpecs(exePath string) ([]struct{ KeyPath, ValueName, Value string }, error) {
	trimmed := strings.TrimSpace(exePath)
	if trimmed == "" {
		return nil, fmt.Errorf("protocol registration requires the executable path")
	}
	absolute, err := filepath.Abs(trimmed)
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}
	command := fmt.Sprintf(`"%s" "%%1"`, absolute)
	specs := make([]struct{ KeyPath, ValueName, Value string }, 0, len(ProtocolSchemes)*2)
	for _, scheme := range ProtocolSchemes {
		root := classesRoot + "\\" + scheme
		specs = append(specs,
			struct{ KeyPath, ValueName, Value string }{root, "URL Protocol", ""},
			struct{ KeyPath, ValueName, Value string }{root + "\\shell\\open\\command", "", command},
		)
	}
	return specs, nil
}

// SetOSProtocols enables or removes OS handoff for every scheme. Enabling is
// idempotent; disabling removes the scheme keys entirely.
func SetOSProtocols(store RegistryStore, exePath string, enabled bool) error {
	specs, err := ProtocolSpecs(exePath)
	if err != nil {
		return err
	}
	if !enabled {
		for _, scheme := range ProtocolSchemes {
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

// OSProtocolsRegistered reports whether every scheme's open command already
// points at this executable.
func OSProtocolsRegistered(store RegistryStore, exePath string) bool {
	specs, err := ProtocolSpecs(exePath)
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
