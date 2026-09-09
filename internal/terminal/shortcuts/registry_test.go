package shortcuts

import (
	"strings"
	"testing"
)

func TestParseAcceleratorValid(t *testing.T) {
	cases := []struct {
		raw, key string
		mods     int
	}{
		{"CmdOrCtrl+Shift+F12", "F12", 2},
		{"Ctrl+A", "A", 1},
		{"F12", "F12", 0},
		{"Alt+Space", "SPACE", 1},
		{"Super+Shift+Escape", "ESCAPE", 2},
	}
	for _, tc := range cases {
		accel, err := ParseAccelerator(tc.raw)
		if err != nil {
			t.Fatalf("%s: %v", tc.raw, err)
		}
		if accel.Key != tc.key {
			t.Fatalf("%s: key %q != %q", tc.raw, accel.Key, tc.key)
		}
		if len(accel.Modifiers) != tc.mods {
			t.Fatalf("%s: mods %d != %d", tc.raw, len(accel.Modifiers), tc.mods)
		}
	}
}

func TestParseAcceleratorInvalid(t *testing.T) {
	for _, raw := range []string{"", "  ", "Ctrl+", "+F12", "Ctrl+NotAKey+", "NotModifier+F12"} {
		if _, err := ParseAccelerator(raw); err == nil {
			t.Fatalf("invalid accelerator %q accepted", raw)
		}
	}
}

func TestRegistryConflictAndLookup(t *testing.T) {
	r := NewRegistry()
	if err := r.Register("CmdOrCtrl+Shift+F12", "quake-toggle"); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("CmdOrCtrl+Shift+F12", "other"); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("duplicate must conflict: %v", err)
	}
	id, err := r.Lookup("cmdorctrl+shift+f12")
	if err != nil || id != "quake-toggle" {
		t.Fatalf("lookup: %s (%v)", id, err)
	}
	if err := r.Unregister("cmdorctrl+shift+f12"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Lookup("cmdorctrl+shift+f12"); err == nil {
		t.Fatal("unregistered must not lookup")
	}
}

func TestRegistryCaseInsensitiveLookup(t *testing.T) {
	r := NewRegistry()
	_ = r.Register("Cmd+Shift+Space", "cb")
	if _, err := r.Lookup("cmd+shift+space"); err != nil {
		t.Fatal(err)
	}
}

func TestBuildTrayState(t *testing.T) {
	state := BuildTrayState([]string{"web-1", "db-1"})
	if len(state.Items) < 5 {
		t.Fatalf("too few items: %d", len(state.Items))
	}
	found := false
	for _, item := range state.Items {
		if item.ID == "connect-db-1" {
			found = true
		}
	}
	if !found {
		t.Fatal("recent host item missing")
	}
}
