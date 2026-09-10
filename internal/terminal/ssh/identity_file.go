package ssh

import (
	"fmt"
	"os"
	"strings"
)

// LoadIdentityFilePEMs reads the first existing identity file as PEM bytes.
// Missing files fail closed so the caller cannot silently fall back to password.
func LoadIdentityFilePEMs(paths []string) ([]byte, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no identity files")
	}
	var lastErr error
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		data, err := os.ReadFile(trimmed)
		if err != nil {
			lastErr = err
			continue
		}
		if len(data) == 0 {
			lastErr = fmt.Errorf("identity file %q is empty", trimmed)
			continue
		}
		return data, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no identity files")
	}
	return nil, lastErr
}
