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
	if title != "Netcatty" {
		t.Fatalf("empty title: %q", title)
	}
}
