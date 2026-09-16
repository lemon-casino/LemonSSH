package main

import "testing"

func TestDeepLinkSecondInstanceArgs(t *testing.T) {
	urls := deepLinkURLsFromArgs([]string{"LemonSSH.exe", "ssh://root@lab:22", "--flag", "telnet://switch"})
	if len(urls) != 2 || urls[0] != "ssh://root@lab:22" || urls[1] != "telnet://switch" {
		t.Fatalf("urls: %#v", urls)
	}
}

func TestAppLockEnableUnlockRoundTrip(t *testing.T) {
	service := newAppLockServiceForTest(t)
	if state := service.GetRuntimeState(); state.Locked {
		t.Fatal("fresh service must start unlocked")
	}
	if _, err := service.Enable("hunter22"); err != nil {
		t.Fatal(err)
	}
	if state := service.GetRuntimeState(); !state.Locked {
		t.Fatal("enable must lock immediately")
	}
	if err := service.Unlock("wrongpass"); err == nil {
		t.Fatal("wrong password must fail closed")
	}
	if err := service.Unlock("hunter22"); err != nil {
		t.Fatal(err)
	}
	if state := service.GetRuntimeState(); state.Locked {
		t.Fatal("unlock must clear locked")
	}
	service.SetRuntimeLocked("manual")
	if err := service.Disable("hunter22"); err != nil {
		t.Fatal(err)
	}
}
