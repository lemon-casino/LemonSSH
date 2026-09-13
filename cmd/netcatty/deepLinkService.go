package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/binaricat/netcatty/internal/platform/deeplink"
)

type DeepLinkService struct {
	queue *deeplink.Queue
}

func newDeepLinkService() *DeepLinkService {
	return &DeepLinkService{queue: deeplink.NewQueue()}
}

func (s *DeepLinkService) Parse(rawURL string) (*deeplink.Action, error) {
	return deeplink.Parse(rawURL)
}

func (s *DeepLinkService) Enqueue(rawURL string) error {
	return s.queue.Enqueue(rawURL)
}

func (s *DeepLinkService) Pending() int {
	return s.queue.Pending()
}

func (s *DeepLinkService) Ready() {
	s.queue.Ready()
}

func (s *DeepLinkService) Drain() []*deeplink.Action {
	return s.queue.Drain()
}

// ProtocolRegistrationResult reports the OS handoff state for the URL schemes
// LemonSSH owns (ssh, telnet, netcatty).
type ProtocolRegistrationResult struct {
	Success    bool   `json:"success"`
	Registered bool   `json:"registered"`
	Error      string `json:"error,omitempty"`
}

// SetOSProtocol registers or removes the ssh/telnet/netcatty URL schemes.
// On Windows this writes HKCU\Software\Classes, which needs no elevation;
// other platforms fail closed until their installer formats own registration.
func (s *DeepLinkService) SetOSProtocol(enabled bool) ProtocolRegistrationResult {
	exePath, err := os.Executable()
	if err != nil {
		return ProtocolRegistrationResult{Error: fmt.Sprintf("resolve executable: %v", err)}
	}
	if err := deeplink.SetNativeProtocols(exePath, enabled); err != nil {
		return ProtocolRegistrationResult{Error: err.Error()}
	}
	registered, err := deeplink.NativeProtocolsRegistered(exePath)
	if err != nil {
		return ProtocolRegistrationResult{Error: err.Error()}
	}
	return ProtocolRegistrationResult{Success: true, Registered: registered}
}

// GetOSProtocolStatus reports whether the schemes currently hand off to this
// executable. A drift (another tool took over ssh://) reads as unregistered.
func (s *DeepLinkService) GetOSProtocolStatus() ProtocolRegistrationResult {
	exePath, err := os.Executable()
	if err != nil {
		return ProtocolRegistrationResult{Error: fmt.Sprintf("resolve executable: %v", err)}
	}
	registered, err := deeplink.NativeProtocolsRegistered(exePath)
	if err != nil {
		return ProtocolRegistrationResult{Error: err.Error()}
	}
	return ProtocolRegistrationResult{Success: true, Registered: registered}
}

func deepLinkURLsFromArgs(args []string) []string {
	var urls []string
	for _, arg := range args {
		lower := strings.ToLower(arg)
		if strings.HasPrefix(lower, "ssh://") || strings.HasPrefix(lower, "telnet://") || strings.HasPrefix(lower, "netcatty://") {
			urls = append(urls, arg)
		}
	}
	return urls
}
