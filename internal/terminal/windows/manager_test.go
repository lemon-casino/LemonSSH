package windows

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCreateRolesAndSingleInstance(t *testing.T) {
	manager := NewManager()
	mainWindow, err := manager.Create(RoleMain)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create(RoleMain); !errors.Is(err, ErrRoleSingleWindow) {
		t.Fatalf("second main must fail: %v", err)
	}
	if _, err := manager.Create(RolePopup); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create("rogue"); !errors.Is(err, ErrUnknownRole) {
		t.Fatalf("unknown role must fail: %v", err)
	}
	if manager.Count(RolePopup) != 1 {
		t.Fatal("popup count mismatch")
	}
	_ = mainWindow
}

func TestTokenGate(t *testing.T) {
	manager := NewManager()
	window, err := manager.Create(RoleSession)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Get(window.ID, "wrong"); !errors.Is(err, ErrWindowNotFound) {
		t.Fatal("wrong token must not find window")
	}
	if _, err := manager.Get(window.ID, window.Token); err != nil {
		t.Fatal(err)
	}
}

func TestCloseGateVetoAndDirty(t *testing.T) {
	manager := NewManager()
	window, _ := manager.Create(RoleSession)

	if err := manager.SetCloseVeto(window.ID, window.Token, true); err != nil {
		t.Fatal(err)
	}
	if err := manager.CloseAttempt(window.ID, window.Token); !errors.Is(err, ErrWindowVetoed) {
		t.Fatalf("veto must block: %v", err)
	}
	if err := manager.SetCloseVeto(window.ID, window.Token, false); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetDirty(window.ID, window.Token, true); err != nil {
		t.Fatal(err)
	}
	if err := manager.CloseAttempt(window.ID, window.Token); !errors.Is(err, ErrDirtyEditor) {
		t.Fatalf("dirty editor must block: %v", err)
	}
	if err := manager.SetDirty(window.ID, window.Token, false); err != nil {
		t.Fatal(err)
	}
	if err := manager.CloseAttempt(window.ID, window.Token); err != nil {
		t.Fatalf("clean window must close: %v", err)
	}
}

func TestCrashCleanupClearsGuardsWithoutToken(t *testing.T) {
	manager := NewManager()
	window, _ := manager.Create(RolePopup)
	_ = manager.SetCloseVeto(window.ID, window.Token, true)
	_ = manager.SetDirty(window.ID, window.Token, true)

	_ = manager.BindRenderer(window.ID, window.Token, "renderer-1")
	other, _ := manager.Create(RolePopup)
	_ = manager.BindRenderer(other.ID, other.Token, "renderer-2")
	_ = manager.SetDirty(other.ID, other.Token, true)
	removed := manager.CrashCleanup("renderer-1")
	if removed != 1 {
		t.Fatalf("removed = %d", removed)
	}
	if _, err := manager.Get(window.ID, window.Token); err == nil {
		t.Fatal("crashed window token remained valid")
	}
	if err := manager.CloseAttempt(other.ID, other.Token); !errors.Is(err, ErrDirtyEditor) {
		t.Fatal("other renderer lost dirty guard")
	}
	if manager.Count(RolePopup) != 1 {
		t.Fatal("crash removed wrong windows")
	}
}

func TestValidateRoleForSession(t *testing.T) {
	if err := ValidateRoleForSession(RolePopup); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRoleForSession(RoleSession); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRoleForSession(RoleMain); err == nil {
		t.Fatal("main must not host terminals")
	}
}

func TestDestroyRemovesFromRoleList(t *testing.T) {
	manager := NewManager()
	a, _ := manager.Create(RolePopup)
	b, _ := manager.Create(RolePopup)
	_ = manager.Destroy(a.ID, a.Token)
	if manager.Count(RolePopup) != 1 {
		t.Fatalf("count after destroy: %d", manager.Count(RolePopup))
	}
	if _, err := manager.Get(b.ID, b.Token); err != nil {
		t.Fatal("surviving window must stay")
	}
	_ = strings.TrimSpace("")
	_ = context.Background()
}
