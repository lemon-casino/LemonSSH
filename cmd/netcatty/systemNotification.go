package main

import "github.com/binaricat/netcatty/internal/platform/notifications"

type SystemNotificationRequest struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	SessionID string `json:"sessionId,omitempty"`
}
type SystemNotificationResult struct {
	Shown  bool   `json:"shown"`
	Reason string `json:"reason,omitempty"`
}

// ShowSystemNotification is reached only after the renderer's OSC permission,
// focus and rate-limit gates. The platform boundary also bounds untrusted text.
func (s *SettingsWindowService) ShowSystemNotification(request SystemNotificationRequest) SystemNotificationResult {
	if err := notifications.Show(request.Title, request.Body); err != nil {
		return SystemNotificationResult{Reason: err.Error()}
	}
	return SystemNotificationResult{Shown: true}
}
