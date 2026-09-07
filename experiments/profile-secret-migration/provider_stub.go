//go:build !windows

package migrationprobe

type unavailableProvider struct{}

func NewPlatformProvider() Provider { return unavailableProvider{} }

func (unavailableProvider) Name() string    { return "unavailable" }
func (unavailableProvider) Available() bool { return false }
func (unavailableProvider) Seal([]byte, string) ([]byte, error) {
	return nil, ErrProviderUnavailable
}
func (unavailableProvider) Open([]byte, string) ([]byte, error) {
	return nil, ErrProviderUnavailable
}
