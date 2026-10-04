//go:build windows

package notifications

import (
	"encoding/base64"
	"strings"
	"testing"
)

// TestWindowsToastScriptUsesLemonSSHAUMID proves the PowerShell notifier id is
// wired to the ToastAUMID constant: a stale inline literal would silently
// break toast delivery after the brand rename (Windows drops toasts whose
// AUMID has no registered Start Menu shortcut).
func TestWindowsToastScriptUsesLemonSSHAUMID(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte(toastXML("title", "body")))
	script := toastScript(payload)
	if !strings.Contains(script, "CreateToastNotifier('LemonSSH')") {
		t.Fatalf("toast script must create the notifier under the LemonSSH AUMID: %s", script)
	}
	if !strings.Contains(script, "FromBase64String('"+payload+"')") {
		t.Fatal("toast script must carry the base64 toast payload")
	}
	if strings.Contains(strings.ToLower(script), "netcatty") {
		t.Fatalf("toast script must not reference the legacy brand: %s", script)
	}
}
