package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const helperRootEnv = "NETCATTY_HELPER_ROOT"

func helperBinaryName(kind string) string {
	name := "mosh-client"
	if kind == "et" {
		name = "et"
	}
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func helperPlatformDir() string {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "arm64" {
			return "win32-arm64"
		}
		return "win32-x64"
	case "darwin":
		return "darwin-universal"
	default:
		if runtime.GOARCH == "arm64" {
			return "linux-arm64"
		}
		return "linux-x64"
	}
}

func helperKindDir(kind string) string {
	if kind == "et" {
		return "et"
	}
	return "mosh"
}

func fileIsExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

// resolveHelperBinary locates a bundled mosh-client or et binary. Order:
// explicit path, NETCATTY_HELPER_ROOT, next to the executable (packaged
// Resources/mosh), then the repo resources/<kind>/<platform> layout used by
// fetch-mosh-binaries. Missing helpers fail closed instead of guessing PATH.
func resolveHelperBinary(kind, explicit, envRoot, exeDir, repoRoot string) (string, error) {
	name := helperBinaryName(kind)
	candidates := make([]string, 0, 6)
	if explicit != "" {
		candidates = append(candidates, explicit)
	}
	if envRoot != "" {
		candidates = append(candidates, filepath.Join(envRoot, name))
		candidates = append(candidates, filepath.Join(envRoot, helperKindDir(kind), name))
	}
	if exeDir != "" {
		candidates = append(candidates, filepath.Join(exeDir, helperKindDir(kind), name))
		candidates = append(candidates, filepath.Join(exeDir, "Resources", helperKindDir(kind), name))
	}
	if repoRoot != "" {
		candidates = append(candidates, filepath.Join(repoRoot, "resources", helperKindDir(kind), helperPlatformDir(), name))
	}
	for _, candidate := range candidates {
		if fileIsExecutable(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s helper %s not found (set %s or run npm run fetch:%s)", kind, name, helperRootEnv, kind)
}
