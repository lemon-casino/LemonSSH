//go:build windows

package main

import (
	"context"
	"encoding/base64"
	"os/exec"
	"strings"
	"syscall"
)

func readNativeClipboardPNG(ctx context.Context) ([]byte, error) {
	// STA is required by Windows Forms. Neither the shell nor helper opens a window.
	const script = `$ErrorActionPreference='Stop'; Add-Type -AssemblyName System.Windows.Forms; if (![System.Windows.Forms.Clipboard]::ContainsImage()) { exit 0 }; $image=[System.Windows.Forms.Clipboard]::GetImage(); if ($null -eq $image) { exit 0 }; try { if ([long]$image.Width * [long]$image.Height -gt 16777216) { throw 'Clipboard image exceeds pixel limit' }; $stream=New-Object System.IO.MemoryStream; try { $image.Save($stream,[System.Drawing.Imaging.ImageFormat]::Png); if ($stream.Length -gt 33554432) { throw 'Clipboard image exceeds size limit' }; [Console]::Out.Write([Convert]::ToBase64String($stream.ToArray())) } finally { $stream.Dispose() } } finally { $image.Dispose() }`
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-STA", "-WindowStyle", "Hidden", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	output, err := boundedClipboardOutput(cmd, ((maxClipboardPNGBytes+2)/3)*4)
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(strings.TrimSpace(string(output)))
}
