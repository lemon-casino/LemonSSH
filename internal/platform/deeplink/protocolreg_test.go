package deeplink

import (
	"errors"
	"runtime"
	"strings"
	"testing"
)

type fakeStore struct {
	values  map[string]string // "keyPath::valueName" -> value
	deleted []string
	failOn  string
}

func newFakeStore() *fakeStore {
	return &fakeStore{values: make(map[string]string)}
}

func keyID(keyPath, valueName string) string { return keyPath + "::" + valueName }

func (f *fakeStore) SetStringValue(keyPath, valueName, value string) error {
	if f.failOn != "" && strings.Contains(keyPath, f.failOn) {
		return errors.New("injected failure")
	}
	f.values[keyID(keyPath, valueName)] = value
	return nil
}

func (f *fakeStore) DeleteTree(keyPath string) error {
	f.deleted = append(f.deleted, keyPath)
	prefix := keyPath + "\\"
	for id := range f.values {
		if strings.HasPrefix(id, prefix+"") || strings.HasPrefix(id, keyPath+"::") {
			delete(f.values, id)
		}
	}
	return nil
}

func (f *fakeStore) GetString(keyPath, valueName string) (string, error) {
	value, ok := f.values[keyID(keyPath, valueName)]
	if !ok {
		return "", errors.New("missing")
	}
	return value, nil
}

// platformExecutable returns an absolute path in the running platform's
// native form. Both registration surfaces bake the command into the OS with
// no working-directory context, so the executable path must be absolute in
// native form (production passes os.Executable()): a Windows path is not
// absolute on Linux and vice versa, and the implementations fail closed on
// such input rather than resolving it against the process cwd.
func platformExecutable() string {
	if runtime.GOOS == "windows" {
		return `C:\Apps\LemonSSH\LemonSSH.exe`
	}
	return "/opt/LemonSSH/lemonssh"
}

func TestSetOSProtocolsWritesAllSchemes(t *testing.T) {
	store := newFakeStore()
	exe := platformExecutable()
	if err := SetOSProtocols(store, exe, true); err != nil {
		t.Fatal(err)
	}
	// Enable registers both the current and the legacy schemes. The command
	// template `"%s" "%1"` is the platform-independent spec format built by
	// protocolSpecsForSchemes: on Windows this asserts the exact registry
	// value `"C:\Apps\LemonSSH\LemonSSH.exe" "%1"`, on Linux the same format
	// carrying the native absolute path.
	wantCommand := `"` + exe + `" "%1"`
	for _, scheme := range append(append([]string{}, ProtocolSchemes...), LegacyProtocolSchemes...) {
		command, err := store.GetString(classesRoot+"\\"+scheme+"\\shell\\open\\command", "")
		if err != nil {
			t.Fatalf("scheme %s missing command: %v", scheme, err)
		}
		if command != wantCommand {
			t.Fatalf("scheme %s command %q", scheme, command)
		}
		if urlProtocol, err := store.GetString(classesRoot+"\\"+scheme, "URL Protocol"); err != nil || urlProtocol != "" {
			t.Fatalf("scheme %s URL Protocol marker missing: %q %v", scheme, urlProtocol, err)
		}
	}
	if !OSProtocolsRegistered(store, exe) {
		t.Fatal("registration must read back as complete")
	}
}

func TestSetOSProtocolsDisableRemovesTrees(t *testing.T) {
	store := newFakeStore()
	exe := platformExecutable()
	if err := SetOSProtocols(store, exe, true); err != nil {
		t.Fatal(err)
	}
	if err := SetOSProtocols(store, exe, false); err != nil {
		t.Fatal(err)
	}
	// Disable removes both the current and the legacy scheme trees.
	for _, scheme := range append(append([]string{}, ProtocolSchemes...), LegacyProtocolSchemes...) {
		found := false
		for _, deleted := range store.deleted {
			if deleted == classesRoot+"\\"+scheme {
				found = true
			}
		}
		if !found {
			t.Fatalf("scheme %s tree was not deleted", scheme)
		}
	}
	if OSProtocolsRegistered(store, exe) {
		t.Fatal("registration must read false after disable")
	}
}

func TestSetOSProtocolsRejectsEmptyExecutable(t *testing.T) {
	store := newFakeStore()
	if err := SetOSProtocols(store, "   ", true); err == nil {
		t.Fatal("empty executable path must fail closed")
	}
}

func TestSetOSProtocolsPropagatesStoreFailure(t *testing.T) {
	store := newFakeStore()
	store.failOn = "telnet"
	if err := SetOSProtocols(store, platformExecutable(), true); err == nil {
		t.Fatal("store failure must propagate")
	}
}

func TestOSProtocolsRegisteredFalseWhenCommandDrifted(t *testing.T) {
	store := newFakeStore()
	exe := platformExecutable()
	if err := SetOSProtocols(store, exe, true); err != nil {
		t.Fatal(err)
	}
	// A different install path took over the ssh scheme.
	store.values[keyID(classesRoot+"\\ssh\\shell\\open\\command", "")] = `"C:\Other\ssh.exe" "%1"`
	if OSProtocolsRegistered(store, exe) {
		t.Fatal("drifted command must read as not registered")
	}
}

// TestOSProtocolsRegisteredWithLegacyOnlyRegistration proves a user who only
// ever registered the pre-rename netcatty:// scheme still reads as
// registered after the upgrade — otherwise the settings toggle would show
// off and push them into re-enabling for nothing.
func TestOSProtocolsRegisteredWithLegacyOnlyRegistration(t *testing.T) {
	store := newFakeStore()
	exe := platformExecutable()
	// Only the legacy scheme set is registered (as an old release did).
	if err := SetOSProtocols(store, exe, true); err != nil {
		t.Fatal(err)
	}
	// Revert the current scheme keys so only netcatty:// remains.
	for _, scheme := range ProtocolSchemes {
		_ = store.DeleteTree(classesRoot + "\\" + scheme)
	}
	if !OSProtocolsRegistered(store, exe) {
		t.Fatal("a complete legacy-only registration must read as registered")
	}
}

// TestOSProtocolsRegisteredFalseWithoutAnyScheme: nothing registered at all
// reads as unregistered.
func TestOSProtocolsRegisteredFalseWithoutAnyScheme(t *testing.T) {
	store := newFakeStore()
	if OSProtocolsRegistered(store, platformExecutable()) {
		t.Fatal("an empty registry must read as not registered")
	}
}

// TestProtocolSpecsRequireNativeAbsolutePath pins the input contract the
// registry spec builder shares with the .desktop writer: a relative (or
// foreign-platform) executable path must fail closed on every platform
// instead of being silently resolved against the process cwd — the exact bug
// class the Linux CI failure exposed (a Windows-style path joined onto the
// runner's cwd). The native absolute path keeps yielding the platform-
// independent `"%s" "%1"` command template.
func TestProtocolSpecsRequireNativeAbsolutePath(t *testing.T) {
	if _, err := ProtocolSpecs("relative/LemonSSH.exe"); err == nil {
		t.Fatal("a relative path must fail closed on every platform")
	}
	exe := platformExecutable()
	specs, err := ProtocolSpecs(exe)
	if err != nil {
		t.Fatal(err)
	}
	wantCommand := `"` + exe + `" "%1"`
	for _, spec := range specs {
		if spec.ValueName == "" && strings.HasSuffix(spec.KeyPath, "\\shell\\open\\command") && spec.Value != wantCommand {
			t.Fatalf("command %q, want %q", spec.Value, wantCommand)
		}
	}
}

// TestDesktopWriterUsesPlatformNativePaths asserts the real .desktop output
// format (the Linux production surface) alongside the platform boundary: the
// writer requires an absolute Unix path, so on Linux a Windows-style exe path
// fails closed and can never leak into a registration, and the entry
// advertises every scheme including the legacy netcatty://.
func TestDesktopWriterUsesPlatformNativePaths(t *testing.T) {
	entry, err := DesktopEntry("/opt/LemonSSH/lemonssh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(entry, `Exec="/opt/LemonSSH/lemonssh" %u`) {
		t.Fatalf("unexpected Exec: %s", entry)
	}
	if !strings.Contains(entry, "MimeType=x-scheme-handler/ssh;x-scheme-handler/telnet;x-scheme-handler/lemonssh;x-scheme-handler/netcatty;") {
		t.Fatalf("legacy netcatty scheme must stay advertised: %s", entry)
	}
	if runtime.GOOS != "windows" {
		// A Windows path is not absolute on this platform; the writer must
		// reject it rather than treat it as a usable Exec target.
		if _, err := DesktopEntry(`C:\Apps\LemonSSH\LemonSSH.exe`); err == nil {
			t.Fatal("a Windows path must fail closed on non-Windows platforms")
		}
	}
}
