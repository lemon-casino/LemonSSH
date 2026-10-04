package notifications

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestToastPayloadEscapesUntrustedTerminalText(t *testing.T) {
	payload := toastXML("<title>&'\"", "$(Start-Process calc); <body>")
	var toast struct {
		Text []string `xml:"visual>binding>text"`
	}
	if err := xml.Unmarshal([]byte(payload), &toast); err != nil {
		t.Fatal(err)
	}
	if len(toast.Text) != 2 || toast.Text[0] != "<title>&'\"" || toast.Text[1] != "$(Start-Process calc); <body>" {
		t.Fatalf("unexpected payload: %#v", toast)
	}
}

func TestNotificationBoundsAndControls(t *testing.T) {
	title, body := sanitize(strings.Repeat("x", 200)+"\x00", "hello\x1b\x00\nworld"+strings.Repeat("z", 600))
	if len([]rune(title)) != 120 || len([]rune(body)) != 500 || strings.ContainsAny(body, "\x00\x1b") {
		t.Fatalf("invalid bounds/controls: %d %q", len(title), body)
	}
	title, _ = sanitize("\x00 ", "body")
	if title != "LemonSSH" {
		t.Fatalf("empty title: %q", title)
	}
}

// TestToastAUMIDIsLemonSSH pins the brand rename: the toast AUMID (the Windows
// CreateToastNotifier id and the notify-send app name) must be "LemonSSH" so
// notifications match the Start Menu shortcut registered by the installer.
func TestToastAUMIDIsLemonSSH(t *testing.T) {
	if ToastAUMID != "LemonSSH" {
		t.Fatalf("toast AUMID = %q, want %q", ToastAUMID, "LemonSSH")
	}
	if strings.Contains(strings.ToLower(ToastAUMID), "netcatty") {
		t.Fatalf("toast AUMID must not use the legacy brand: %q", ToastAUMID)
	}
}
