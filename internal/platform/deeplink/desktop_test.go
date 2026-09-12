package deeplink

import (
	"strings"
	"testing"
)

// Reject desktop Exec field-code injection and preserve paths containing spaces.
func TestDesktopEntryEscapesExecutable(t *testing.T) {
	entry, err := DesktopEntry(`/opt/Lemon SSH/100%/lemon"ssh`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(entry, `Exec="/opt/Lemon SSH/100%%/lemon\\\"ssh" %u`) {
		t.Fatalf("unsafe Exec: %s", entry)
	}
	if !strings.Contains(entry, "MimeType=x-scheme-handler/ssh;x-scheme-handler/telnet;x-scheme-handler/netcatty;") {
		t.Fatal(entry)
	}
	for _, path := range []string{"", "relative/path", "/tmp/app\nHidden=true"} {
		if _, err := DesktopEntry(path); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
}
