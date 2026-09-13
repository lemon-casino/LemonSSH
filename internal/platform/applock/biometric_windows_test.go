//go:build windows

package applock

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestBiometricPowerShellCommandFlags(t *testing.T) {
	cmd := biometricPowerShellCommand(context.Background(), "Write-Output 'test'")
	if cmd.SysProcAttr == nil {
		t.Fatal("PowerShell command has no process attributes")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Error("PowerShell command must hide its startup window")
	}
	if cmd.SysProcAttr.CreationFlags != windows.CREATE_NO_WINDOW {
		t.Errorf("creation flags = %#x, want CREATE_NO_WINDOW", cmd.SysProcAttr.CreationFlags)
	}
}

func TestBiometricPowerShellCommandNoConsole(t *testing.T) {
	if os.Getenv("NETCATTY_TEST_CONSOLE_PARENT") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestBiometricPowerShellCommandNoConsole$", "-test.v")
		cmd.Env = append(os.Environ(), "NETCATTY_TEST_CONSOLE_PARENT=1")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_CONSOLE}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("console parent test failed: %v\n%s", err, out)
		}
		return
	}
	getConsoleWindow := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
	if handle, _, err := getConsoleWindow.Call(); handle == 0 {
		t.Fatalf("test helper must have a console: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// CREATE_NO_WINDOW must prevent both console allocation and inheritance.
	// This probes the actual child, without calling any Windows Hello API.
	out, err := biometricPowerShellCommand(ctx, `
$ErrorActionPreference = 'Stop'
Add-Type -TypeDefinition 'using System; using System.Runtime.InteropServices; public static class ConsoleProbe { [DllImport("kernel32.dll")] public static extern IntPtr GetConsoleWindow(); }'
[Console]::WriteLine([ConsoleProbe]::GetConsoleWindow().ToInt64())
`).CombinedOutput()
	if err != nil {
		t.Fatalf("console probe failed: %v: %s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "0" {
		t.Fatalf("PowerShell console handle = %q, want 0", got)
	}
}

func TestBiometricPowerShellCommandOutputAndError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := biometricPowerShellCommand(ctx, `[Console]::Out.WriteLine('stdout-marker'); [Console]::Error.WriteLine('stderr-marker'); exit 7`).CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("error = %v, want exit code 7; output: %s", err, out)
	}
	for _, marker := range []string{"stdout-marker", "stderr-marker"} {
		if !strings.Contains(string(out), marker) {
			t.Errorf("combined output %q is missing %q", out, marker)
		}
	}
}

func TestBiometricPowerShellCommandContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := biometricPowerShellCommand(ctx, "Write-Output 'must not run'").CombinedOutput()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestWindowsHelloNativeAuthentication(t *testing.T) {
	if os.Getenv("NETCATTY_TEST_WINDOWS_HELLO") != "1" {
		t.Skip("opt-in hardware authentication test")
	}
	err := AuthenticateBiometric()
	if os.Getenv("NETCATTY_TEST_WINDOWS_HELLO_UNAVAILABLE") == "1" {
		if err == nil {
			t.Fatal("unavailable Hello unexpectedly authenticated")
		}
		t.Log(err)
		return
	}
	if err != nil {
		t.Fatal(err)
	}
}
