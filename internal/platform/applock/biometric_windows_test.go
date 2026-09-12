//go:build windows

package applock

import (
	"os"
	"testing"
)

func TestWindowsHelloNativeAuthentication(t *testing.T) {
	if os.Getenv("NETCATTY_TEST_WINDOWS_HELLO") != "1" {
		t.Skip("opt-in hardware authentication test")
	}
	err := AuthenticateBiometric()
	if os.Getenv("NETCATTY_TEST_WINDOWS_HELLO_UNAVAILABLE") == "1" {
		if err == nil {
			t.Fatal("unavailable Hello unexpectedly authenticated")
		}
		t.Log(err)
		return
	}
	if err != nil {
		t.Fatal(err)
	}
}
