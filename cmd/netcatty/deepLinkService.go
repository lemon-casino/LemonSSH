package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/binaricat/netcatty/internal/platform/deeplink"
)

type DeepLinkService struct {
	queue *deeplink.Queue
	emit  func(name string, payload any)
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

func (s *DeepLinkService) EnqueueOpenTerminalPath(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	s.queue.EnqueueAction(&deeplink.Action{Kind: "open-terminal", Path: path}, "open-terminal:"+path)
}

func (s *DeepLinkService) Pending() int {
	return s.queue.Pending()
}

func (s *DeepLinkService) setEventEmitter(emit func(name string, payload any)) { s.emit = emit }

func (s *DeepLinkService) Ready() {
	s.queue.Ready(func(action *deeplink.Action) {
		if s.emit != nil {
			s.emit("deeplink:"+action.Kind, action)
		}
	})
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

type DesktopToggleResult struct {
	Success   bool   `json:"success"`
	Enabled   bool   `json:"enabled"`
	Supported bool   `json:"supported,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (s *DeepLinkService) SetSshDeepLinkEnabled(enabled bool) DesktopToggleResult {
	result := s.SetOSProtocol(enabled)
	return DesktopToggleResult{Success: result.Success, Enabled: result.Registered, Supported: true, Error: result.Error}
}

func (s *DeepLinkService) GetSshDeepLinkEnabled() bool {
	return s.GetOSProtocolStatus().Registered
}

func (s *DeepLinkService) SetJmsDeepLinkEnabled(enabled bool) DesktopToggleResult {
	if err := setJMSProtocolEnabled(enabled); err != nil {
		return DesktopToggleResult{Enabled: jmsProtocolEnabled(), Supported: false, Error: err.Error()}
	}
	return DesktopToggleResult{Success: true, Enabled: jmsProtocolEnabled(), Supported: true}
}

func (s *DeepLinkService) GetJmsDeepLinkEnabled() bool { return jmsProtocolEnabled() }

func (s *DeepLinkService) SetExplorerContextMenuEnabled(enabled bool) DesktopToggleResult {
	if err := setExplorerContextMenu(enabled); err != nil {
		current, supported := explorerContextMenuEnabled()
		return DesktopToggleResult{Enabled: current, Supported: supported, Error: err.Error()}
	}
	current, supported := explorerContextMenuEnabled()
	return DesktopToggleResult{Success: true, Enabled: current, Supported: supported}
}

func (s *DeepLinkService) GetExplorerContextMenuEnabled() DesktopToggleResult {
	enabled, supported := explorerContextMenuEnabled()
	return DesktopToggleResult{Success: true, Enabled: enabled, Supported: supported}
}

func deepLinkURLsFromArgs(args []string) []string {
	var urls []string
	for _, arg := range args {
		lower := strings.ToLower(arg)
		if strings.HasPrefix(lower, "ssh://") || strings.HasPrefix(lower, "telnet://") || strings.HasPrefix(lower, "netcatty://") || strings.HasPrefix(lower, "jms://") {
			urls = append(urls, arg)
		}
	}
	return urls
}

func openTerminalPathsFromArgs(args []string) []string {
	result := []string{}
	for index := 0; index < len(args); index++ {
		if args[index] == "--open-terminal" && index+1 < len(args) {
			if path := strings.TrimSpace(args[index+1]); path != "" {
				result = append(result, path)
			}
			index++
		}
	}
	return result
}
