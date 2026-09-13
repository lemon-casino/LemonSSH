//go:build windows

package notifications

import (
	"context"
	"encoding/base64"
	"os/exec"
	"syscall"
	"time"
)

// Show uses a fixed script; terminal text enters only as base64 encoded XML data.
func Show(title, body string) error {
	payload := base64.StdEncoding.EncodeToString([]byte(toastXML(title, body)))
	script := `$ErrorActionPreference='Stop'; [Windows.UI.Notifications.ToastNotificationManager,Windows.UI.Notifications,ContentType=WindowsRuntime] > $null; [Windows.Data.Xml.Dom.XmlDocument,Windows.Data.Xml.Dom.XmlDocument,ContentType=WindowsRuntime] > $null; $xml=New-Object Windows.Data.Xml.Dom.XmlDocument; $xml.LoadXml([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + payload + `'))); $toast=[Windows.UI.Notifications.ToastNotification]::new($xml); [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('Netcatty').Show($toast)`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd.Run()
}
