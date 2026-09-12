//go:build windows

package deeplink

func SetNativeProtocols(executable string, enabled bool) error {
	return SetOSProtocols(NewRegistryStore(), executable, enabled)
}
func NativeProtocolsRegistered(executable string) (bool, error) {
	return OSProtocolsRegistered(NewRegistryStore(), executable), nil
}
