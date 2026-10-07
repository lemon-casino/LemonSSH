//go:build windows

package ssh

import (
	"os/exec"
	"testing"
)

func TestX11XauthDoesNotCreateConsole(t *testing.T) {
	cmd := exec.Command("xauth", "list", ":0")
	hideXauthWindow(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags&0x08000000 == 0 {
		t.Fatal("xauth helper can allocate or show a console")
	}
}
