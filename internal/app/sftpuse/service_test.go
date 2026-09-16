package sftpuse

import (
	"os"
	"testing"
)

// Characterization moved verbatim from cmd/netcatty/sftp_external_operations_test.go
// during the W04 SFTP use-case extraction; the assertion table is unchanged.
func TestParseSFTPPermissions(t *testing.T) {
	for text, want := range map[string]os.FileMode{"000": 0, "644": 0644, "0755": 0755, "4755": 0755 | os.ModeSetuid, "2750": 0750 | os.ModeSetgid, "1777": 0777 | os.ModeSticky} {
		got, err := parsePermissions(text)
		if err != nil || got != want {
			t.Errorf("%s: %v %v, want %v", text, got, err, want)
		}
	}
	for _, text := range []string{"", "77777", "888", "-1", "0x777", " 755", "755x"} {
		if _, err := parsePermissions(text); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
}
