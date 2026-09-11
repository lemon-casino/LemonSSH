package deeplink

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

type fakeStore struct {
	values map[string]string // "keyPath::valueName" -> value
	deleted []string
	failOn string
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

func TestSetOSProtocolsWritesAllSchemes(t *testing.T) {
	store := newFakeStore()
	exe := `C:\Apps\LemonSSH\LemonSSH.exe`
	if err := SetOSProtocols(store, exe, true); err != nil {
		t.Fatal(err)
	}
	for _, scheme := range ProtocolSchemes {
		command, err := store.GetString(classesRoot+"\\"+scheme+"\\shell\\open\\command", "")
		if err != nil {
			t.Fatalf("scheme %s missing command: %v", scheme, err)
		}
		if command != `"C:\Apps\LemonSSH\LemonSSH.exe" "%1"` {
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
	exe := filepath.Join("C:", "Apps", "LemonSSH.exe")
	if err := SetOSProtocols(store, exe, true); err != nil {
		t.Fatal(err)
	}
	if err := SetOSProtocols(store, exe, false); err != nil {
		t.Fatal(err)
	}
	for _, scheme := range ProtocolSchemes {
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
	if err := SetOSProtocols(store, "C:\\x\\LemonSSH.exe", true); err == nil {
		t.Fatal("store failure must propagate")
	}
}

func TestOSProtocolsRegisteredFalseWhenCommandDrifted(t *testing.T) {
	store := newFakeStore()
	exe := "C:\\Apps\\LemonSSH.exe"
	if err := SetOSProtocols(store, exe, true); err != nil {
		t.Fatal(err)
	}
	// A different install path took over the ssh scheme.
	store.values[keyID(classesRoot+"\\ssh\\shell\\open\\command", "")] = `"C:\Other\ssh.exe" "%1"`
	if OSProtocolsRegistered(store, exe) {
		t.Fatal("drifted command must read as not registered")
	}
}
