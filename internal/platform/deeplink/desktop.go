package deeplink

import (
	"fmt"
	"strings"
)

const desktopID = "lemonssh.desktop"

// DesktopEntry quotes both desktop-entry strings and Exec field codes. No shell is used.
func DesktopEntry(executable string) (string, error) {
	if !strings.HasPrefix(executable, "/") || strings.ContainsAny(executable, "\r\n\x00") {
		return "", fmt.Errorf("protocol registration requires an absolute Unix executable path")
	}
	escaped := strings.NewReplacer(`\`, `\\\\`, `"`, `\\\"`, "`", "\\\\`", "$", "\\\\$", "%", "%%").Replace(executable)
	// Desktop string decoding precedes Exec quoting.
	return "[Desktop Entry]\nType=Application\nName=LemonSSH\nNoDisplay=true\nExec=\"" + escaped + "\" %u\nTerminal=false\nMimeType=x-scheme-handler/ssh;x-scheme-handler/telnet;x-scheme-handler/netcatty;\n", nil
}
