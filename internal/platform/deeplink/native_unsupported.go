//go:build (!windows && !linux && !darwin) || (darwin && !cgo)

package deeplink

import "fmt"

func SetNativeProtocols(string, bool) error {
	return fmt.Errorf("native protocol registration unavailable in this build")
}
func NativeProtocolsRegistered(string) (bool, error) {
	return false, fmt.Errorf("native protocol registration unavailable in this build")
}
