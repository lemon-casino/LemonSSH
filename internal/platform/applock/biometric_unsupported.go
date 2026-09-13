//go:build (!windows && !darwin) || (darwin && !cgo)

package applock

import "fmt"

func BiometricAvailable() error { return AuthenticateBiometric() }

func AuthenticateBiometric() error {
	return fmt.Errorf("biometric authentication unavailable on this platform build")
}
