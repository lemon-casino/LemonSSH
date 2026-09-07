// Package platform holds platform-specific adapters for Netcatty.
//
// During P1-02 this is intentionally minimal: it reports the platform facts
// the shell-neutral application layer is allowed to see. Wails-specific
// platform adapters belong behind interfaces defined here so the application
// layer stays testable without a display server.
package platform

import "runtime"

// Info describes the host platform.
type Info struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
}

// Current reports the build target platform.
func Current() Info {
	return Info{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
}
