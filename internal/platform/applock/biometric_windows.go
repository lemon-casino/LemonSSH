//go:build windows

package applock

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// AuthenticateBiometric asks Windows Hello to sign a fresh random challenge with
// an app-specific KeyCredential. Windows owns the private key and consent UI.
func AuthenticateBiometric() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	executable := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	out, err := exec.CommandContext(ctx, executable, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", helloScript).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Windows Hello authentication failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if strings.TrimSpace(string(out)) != "NETCATTY_HELLO_AUTHENTICATED" {
		return fmt.Errorf("Windows Hello returned no authenticated result")
	}
	return nil
}

func BiometricAvailable() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	script := strings.Split(helloScript, "if (!(Await")[0] + "\nif (Await ([Windows.Security.Credentials.KeyCredentialManager]::IsSupportedAsync()) ([bool])) { Write-Output 'AVAILABLE' } else { throw 'Windows Hello is not configured or supported' }"
	executable := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	out, err := exec.CommandContext(ctx, executable, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Windows Hello unavailable: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if strings.TrimSpace(string(out)) != "AVAILABLE" {
		return fmt.Errorf("Windows Hello availability not confirmed")
	}
	return nil
}

const helloScript = `
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Runtime.WindowsRuntime
[Windows.Security.Credentials.KeyCredentialManager,Windows.Security.Credentials,ContentType=WindowsRuntime] | Out-Null
[Windows.Security.Credentials.KeyCredentialRetrievalResult,Windows.Security.Credentials,ContentType=WindowsRuntime] | Out-Null
[Windows.Security.Credentials.KeyCredentialOperationResult,Windows.Security.Credentials,ContentType=WindowsRuntime] | Out-Null
[Windows.Security.Cryptography.CryptographicBuffer,Windows.Security.Cryptography,ContentType=WindowsRuntime] | Out-Null
function Await($operation, $resultType) {
 $method = [System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object { $_.Name -eq 'AsTask' -and $_.IsGenericMethod -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq ('IAsyncOperation'+[char]96+'1') } | Select-Object -First 1
 if (!$method) { throw 'Windows Runtime task adapter unavailable' }
 $task = $method.MakeGenericMethod($resultType).Invoke($null, @($operation))
 $task.GetAwaiter().GetResult()
}
if (!(Await ([Windows.Security.Credentials.KeyCredentialManager]::IsSupportedAsync()) ([bool]))) { throw 'Windows Hello is not configured or supported' }
$name = 'app.lemonssh.desktop.app-lock.v1'
$result = Await ([Windows.Security.Credentials.KeyCredentialManager]::OpenAsync($name)) ([Windows.Security.Credentials.KeyCredentialRetrievalResult])
if ($result.Status.ToString() -eq 'NotFound') {
 $result = Await ([Windows.Security.Credentials.KeyCredentialManager]::RequestCreateAsync($name, [Windows.Security.Credentials.KeyCredentialCreationOption]::FailIfExists)) ([Windows.Security.Credentials.KeyCredentialRetrievalResult])
}
if ($result.Status.ToString() -ne 'Success') { throw ('Windows Hello credential: '+$result.Status) }
$challenge = [Windows.Security.Cryptography.CryptographicBuffer]::GenerateRandom(32)
$signed = Await ($result.Credential.RequestSignAsync($challenge)) ([Windows.Security.Credentials.KeyCredentialOperationResult])
if ($signed.Status.ToString() -ne 'Success' -or $signed.Result.Length -eq 0) { throw ('Windows Hello signature rejected: '+$signed.Status) }
Write-Output 'NETCATTY_HELLO_AUTHENTICATED'
`
