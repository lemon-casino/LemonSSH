package charset

import (
"strings"
"testing"
)

// gbkShuJu is the GB18030 encoding of "数据" (shùjù, "data").
var gbkShuJu = string([]byte{0xCA, 0xFD, 0xBE, 0xDD})

func TestNormalize(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", UTF8},
		{"auto", Auto},
		{"AUTO", Auto},
		{"utf-8", UTF8},
		{"utf8", UTF8},
		{"UTF-8", UTF8},
		{"gb18030", GB18030},
		{"GBK", GB18030},
		{"gb2312", GB18030},
		{"CP936", GB18030},
		{"shift_jis", UTF8}, // unsupported labels never corrupt traffic
	}
	for _, tc := range cases {
		if got := Normalize(tc.in); got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDecodeGB18030(t *testing.T) {
	if got := Decode(gbkShuJu, GB18030); got != "数据" {
		t.Fatalf("Decode(gb18030) = %q, want 数据", got)
	}
	// ASCII is a GB18030 subset and passes through byte-for-byte.
	if got := Decode("hello.txt", GB18030); got != "hello.txt" {
		t.Fatalf("Decode(gb18030 ascii) = %q", got)
	}
	// An explicit charset choice decodes every byte as that charset: UTF-8
	// multi-byte sequences are reinterpreted (this is the documented cost of
	// picking GB18030 on a UTF-8 server). Auto mode is what keeps valid
	// UTF-8 names intact.
	if got := Decode("héllo.txt", GB18030); got != "h茅llo.txt" {
		t.Fatalf("Decode(gb18030 utf8 bytes) = %q, want h茅llo.txt", got)
	}
}

func TestDecodeAuto(t *testing.T) {
	if got := Decode("ascii.txt", Auto); got != "ascii.txt" {
		t.Fatalf("Decode(auto ascii) = %q", got)
	}
	if got := Decode("中文.txt", Auto); got != "中文.txt" {
		t.Fatalf("Decode(auto utf8) = %q", got)
	}
	if got := Decode(gbkShuJu, Auto); got != "数据" {
		t.Fatalf("Decode(auto legacy) = %q, want 数据", got)
	}
}

func TestDecodeInvalidUTF8Sanitized(t *testing.T) {
	got := Decode("\xff\xfe", UTF8)
	if !strings.Contains(got, "\uFFFD") {
		t.Fatalf("Decode(invalid utf-8) = %q, want replacement chars", got)
	}
}

func TestEncodeGB18030(t *testing.T) {
	got := Encode("数据.txt", GB18030)
	if got != gbkShuJu+".txt" {
		t.Fatalf("Encode = % X, want % X", []byte(got), []byte(gbkShuJu+".txt"))
	}
	// Round trip.
	if back := Decode(got, GB18030); back != "数据.txt" {
		t.Fatalf("round trip = %q", back)
	}
}

func TestEncodeASCIISkipsCodec(t *testing.T) {
	if got := Encode("/etc/hosts", GB18030); got != "/etc/hosts" {
		t.Fatalf("Encode(ascii) = %q", got)
	}
	if got := Encode("/etc/hosts", Auto); got != "/etc/hosts" {
		t.Fatalf("Encode(auto ascii) = %q", got)
	}
}

func TestEncodeUTF8Passthrough(t *testing.T) {
	if got := Encode("数据.txt", UTF8); got != "数据.txt" {
		t.Fatalf("Encode(utf-8) = %q", got)
	}
}

func TestDetectListingEncoding(t *testing.T) {
	if encoding, detected := DetectListingEncoding([]string{"a.txt", "b.log"}); detected {
		t.Fatalf("all-UTF-8 list detected %q", encoding)
	}
	if encoding, detected := DetectListingEncoding([]string{"ok.txt", gbkShuJu}); !detected || encoding != GB18030 {
		t.Fatalf("legacy list: detected=%v encoding=%q", detected, encoding)
	}
}
