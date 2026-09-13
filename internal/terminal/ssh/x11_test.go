package ssh

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// These cases catch routing DISPLAY as a raw TCP port or accepting nonlocal hosts.
func TestX11Display(t *testing.T) {
	for _, tc := range []struct {
		spec, platform, network, address string
		screen                           uint32
	}{
		{":0", "linux", "unix", "/tmp/.X11-unix/X0", 0},
		{"unix:2.3", "darwin", "unix", "/tmp/.X11-unix/X2", 3},
		{":1", "windows", "tcp", "127.0.0.1:6001", 0},
		{"localhost:100.2", "windows", "tcp", "127.0.0.1:6100", 2},
		{"[::1]:0", "linux", "tcp", "[::1]:6000", 0},
	} {
		t.Run(tc.spec+tc.platform, func(t *testing.T) {
			d, err := ParseX11Display(tc.spec, tc.platform)
			if err != nil || d.Network != tc.network || d.Address != tc.address || d.Screen != tc.screen {
				t.Fatalf("got %+v %v", d, err)
			}
		})
	}
	for _, s := range []string{"evil.example:0", "192.168.1.4:0", "127.0.0.1:60000", "-q", ":-1", ":1.bad", "/tmp/arbitrary.sock", "tcp/localhost:0"} {
		if _, err := ParseX11Display(s, "linux"); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
}

func x11Setup(order binary.ByteOrder, cookie []byte) []byte {
	p := make([]byte, 48)
	p[0] = 'B'
	if order == binary.LittleEndian {
		p[0] = 'l'
	}
	order.PutUint16(p[2:], 11)
	order.PutUint16(p[6:], 18)
	order.PutUint16(p[8:], 16)
	copy(p[12:], "MIT-MAGIC-COOKIE-1")
	copy(p[32:], cookie)
	return p
}

// A bad token, protocol, version or truncated setup must never reach the local X server.
func TestX11CookieSubstitution(t *testing.T) {
	fake := bytes.Repeat([]byte{0x11}, 16)
	real := bytes.Repeat([]byte{0x22}, 16)
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		p := x11Setup(order, fake)
		got, err := readX11Setup(bytes.NewReader(p), fake, real)
		if err != nil || !bytes.Equal(got[32:], real) || !bytes.Equal(got[:32], p[:32]) {
			t.Fatalf("rewrite failed %x %v", got, err)
		}
		for _, idx := range []int{0, 2, 6, 8, 12, 32} {
			bad := append([]byte(nil), p...)
			bad[idx] ^= 1
			if _, err := readX11Setup(bytes.NewReader(bad), fake, real); err == nil {
				t.Errorf("accepted tampering at %d", idx)
			}
		}
		if _, err := readX11Setup(bytes.NewReader(p[:40]), fake, real); err == nil {
			t.Fatal("accepted truncated setup")
		}
	}
}

func TestX11XauthSelection(t *testing.T) {
	cookie, err := parseXauthCookie("local/unix:0 MIT-MAGIC-COOKIE-1 00112233445566778899aabbccddeeff\n")
	if err != nil || len(cookie) != 16 || cookie[15] != 255 {
		t.Fatalf("cookie %x %v", cookie, err)
	}
	for _, s := range []string{"", "local:0 OTHER 00112233445566778899aabbccddeeff", "local:0 MIT-MAGIC-COOKIE-1 00", "a MIT-MAGIC-COOKIE-1 00112233445566778899aabbccddeeff\nb MIT-MAGIC-COOKIE-1 ffeeddccbbaa99887766554433221100"} {
		if _, err := parseXauthCookie(s); err == nil {
			t.Fatalf("accepted invalid/ambiguous authority %q", s)
		}
	}
}
