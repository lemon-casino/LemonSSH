//go:build linux

package deeplink

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func desktopPath() (string, error) {
	root := os.Getenv("XDG_DATA_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".local", "share")
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("XDG_DATA_HOME must be absolute")
	}
	return filepath.Join(root, "applications", desktopID), nil
}

func SetNativeProtocols(executable string, enabled bool) error {
	path, err := desktopPath()
	if err != nil {
		return err
	}
	if !enabled {
		// Remove only our private desktop entry; never delete another application's defaults.
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	entry, err := DesktopEntry(executable)
	if err != nil {
		return err
	}
	if _, err = exec.LookPath("xdg-mime"); err != nil {
		return fmt.Errorf("xdg-mime unavailable: %w", err)
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if err = os.WriteFile(path, []byte(entry), 0600); err != nil {
		return err
	}
	for _, scheme := range ProtocolSchemes {
		out, err := exec.Command("xdg-mime", "default", desktopID, "x-scheme-handler/"+scheme).CombinedOutput()
		if err != nil {
			return fmt.Errorf("register %s: %w: %s", scheme, err, strings.TrimSpace(string(out)))
		}
	}
	registered, err := NativeProtocolsRegistered(executable)
	if err != nil {
		return err
	}
	if !registered {
		return fmt.Errorf("desktop protocol registration was not accepted")
	}
	return nil
}

func NativeProtocolsRegistered(executable string) (bool, error) {
	path, err := desktopPath()
	if err != nil {
		return false, err
	}
	expected, err := DesktopEntry(executable)
	if err != nil {
		return false, err
	}
	actual, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if string(actual) != expected {
		return false, nil
	}
	for _, scheme := range ProtocolSchemes {
		out, err := exec.Command("xdg-mime", "query", "default", "x-scheme-handler/"+scheme).Output()
		if err != nil {
			return false, fmt.Errorf("query %s registration: %w", scheme, err)
		}
		if strings.TrimSpace(string(out)) != desktopID {
			return false, nil
		}
	}
	return true, nil
}
