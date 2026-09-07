package app

import (
	"context"
	"errors"
	"testing"
)

func TestHealthAndVersion(t *testing.T) {
	application := New("Netcatty", "0.0.0-skeleton")
	ctx := context.Background()

	health, err := application.Health(ctx)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if health.Status != "ok" || health.PID <= 0 {
		t.Fatalf("unexpected health payload: %+v", health)
	}

	version, err := application.Version(ctx)
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if version.Name != "Netcatty" || version.Version != "0.0.0-skeleton" {
		t.Fatalf("unexpected version payload: %+v", version)
	}
	if version.GOOS == "" || version.GOARCH == "" || version.GoVersion == "" {
		t.Fatalf("runtime identity missing: %+v", version)
	}
}

func TestResolveWindowRole(t *testing.T) {
	application := New("Netcatty", "0.0.0-skeleton")
	ctx := context.Background()

	main, err := application.ResolveWindowRole(ctx, WindowRoleMain)
	if err != nil {
		t.Fatalf("main role: %v", err)
	}
	if !main.SingleInstance {
		t.Fatal("main window must be single instance")
	}

	for _, role := range []string{WindowRoleSettings, WindowRoleSession, WindowRolePopup} {
		info, err := application.ResolveWindowRole(ctx, role)
		if err != nil {
			t.Fatalf("role %s: %v", role, err)
		}
		if info.SingleInstance {
			t.Fatalf("role %s must not be single instance", role)
		}
	}

	if _, err := application.ResolveWindowRole(ctx, "../escape"); !errors.Is(err, ErrUnknownWindowRole) {
		t.Fatalf("unknown role must fail closed, got %v", err)
	}
}

func TestContextCancellation(t *testing.T) {
	application := New("Netcatty", "0.0.0-skeleton")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := application.Health(ctx); err == nil {
		t.Fatal("health must respect cancellation")
	}
	if _, err := application.Version(ctx); err == nil {
		t.Fatal("version must respect cancellation")
	}
	if _, err := application.ResolveWindowRole(ctx, WindowRoleMain); err == nil {
		t.Fatal("resolve window role must respect cancellation")
	}
}
