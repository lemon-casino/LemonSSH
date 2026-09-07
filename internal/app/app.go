// Package app exposes the shell-neutral Netcatty application use cases.
//
// Per the Wails v3 migration (P1-02), this package is the future canonical
// owner of application coordination. It must never import Wails: the
// cmd/netcatty main is the only place allowed to adapt these use cases to a
// shell (Wails today, Electron during the controlled dual-shell transition).
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
)

// Window roles mirror the roles the Electron shell already implements. The
// set is closed: unknown roles must be rejected instead of guessed.
const (
	WindowRoleMain     = "main"
	WindowRoleSettings = "settings"
	WindowRoleSession  = "session"
	WindowRolePopup    = "popup"
)

// ErrUnknownWindowRole is returned for window roles outside the closed set.
var ErrUnknownWindowRole = errors.New("unknown window role")

// VersionInfo describes the running application build.
type VersionInfo struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	GoVersion string `json:"goVersion"`
}

// HealthStatus is the liveness payload used by shells and CI smokes.
type HealthStatus struct {
	Status string `json:"status"`
	PID    int    `json:"pid"`
}

// WindowRoleInfo describes a resolved window role.
type WindowRoleInfo struct {
	Role           string `json:"role"`
	SingleInstance bool   `json:"singleInstance"`
}

// App owns shell-neutral application state. It is safe for concurrent use.
type App struct {
	name    string
	version string
}

// New constructs the application use cases.
func New(name, version string) *App {
	return &App{name: name, version: version}
}

// Health reports liveness.
func (a *App) Health(ctx context.Context) (HealthStatus, error) {
	if err := ctx.Err(); err != nil {
		return HealthStatus{}, err
	}
	return HealthStatus{Status: "ok", PID: os.Getpid()}, nil
}

// Version reports build and runtime identity.
func (a *App) Version(ctx context.Context) (VersionInfo, error) {
	if err := ctx.Err(); err != nil {
		return VersionInfo{}, err
	}
	return VersionInfo{
		Name:      a.name,
		Version:   a.version,
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
		GoVersion: runtime.Version(),
	}, nil
}

// ResolveWindowRole validates a window role requested from a renderer.
func (a *App) ResolveWindowRole(ctx context.Context, role string) (WindowRoleInfo, error) {
	if err := ctx.Err(); err != nil {
		return WindowRoleInfo{}, err
	}
	switch role {
	case WindowRoleMain:
		return WindowRoleInfo{Role: WindowRoleMain, SingleInstance: true}, nil
	case WindowRoleSettings, WindowRoleSession, WindowRolePopup:
		return WindowRoleInfo{Role: role, SingleInstance: false}, nil
	default:
		return WindowRoleInfo{}, fmt.Errorf("%w: %q", ErrUnknownWindowRole, role)
	}
}
