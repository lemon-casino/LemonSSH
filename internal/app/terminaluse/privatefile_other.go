//go:build !windows

package terminaluse

import "os"

func restrictPrivateFile(path string) error {
	return os.Chmod(path, 0o600)
}
