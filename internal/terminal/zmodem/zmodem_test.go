package zmodem

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"
)

func TestRawHeaderFixtures(t *testing.T) {
	// Independent zmodem.js headers, also the standard lrzsz hex handshake.
	for _, tc := range []struct{ wire, kind string }{
		{"**\x18B00000000000000\r\n\x11", FrameZRQINIT},
		{"**\x18B0100000023be50\r\n\x11", FrameZRINIT},
	} {
		frame, _, err := ParseBinaryHeader([]byte(tc.wire))
		if err != nil || frame.Type != tc.kind {
			t.Fatalf("raw header %q: %+v %v", tc.wire, frame, err)
		}
	}
}

func TestRawNullSeparatedMetadata(t *testing.T) {
	meta, err := ParseFileMeta([]byte("hello world.txt\x005 0 100644 0\x00"), 1024)
	if err != nil || meta.Name != "hello world.txt" || meta.Size != 5 {
		t.Fatalf("raw metadata: %+v %v", meta, err)
	}
}

func TestParseFileMetaSafeNames(t *testing.T) {
	cases := []struct {
		payload  string
		name     string
		size     int64
		hasError bool
	}{
		{"report.pdf 12345 644 0 0", "report.pdf", 12345, false},
		{"archive.tar.gz 0", "archive.tar.gz", 0, false},
		{"../../etc/passwd 100", "", 0, true},
		{"C:\\evil.exe 100", "", 0, true},
		{"/abs/path 100", "", 0, true},
		{"CON 100", "", 0, true},
		{"NUL 100", "", 0, true},
		{"PRN 100", "", 0, true},
		{"AUX 100", "", 0, true},
		{"COM1 100", "", 0, true},
		{"LPT9 100", "", 0, true},
		{"NUL.txt 100", "", 0, true},
		{"com1.log 100", "", 0, true},
		{"console.txt 100", "console.txt", 100, false},
		{"COM0 100", "COM0", 100, false},
		{".. 100", "", 0, true},
		{"sizexyz 12ab", "", 0, true},
	}
	for index, testCase := range cases {
		meta, err := ParseFileMeta([]byte(testCase.payload), MaxFileBytes)
		if testCase.hasError {
			if err == nil {
				t.Fatalf("case %d: unsafe metadata accepted: %+v", index, meta)
			}
			continue
		}
		if err != nil {
			t.Fatalf("case %d: %v", index, err)
		}
		if meta.Name != testCase.name || meta.Size != testCase.size {
			t.Fatalf("case %d: %+v != %s %d", index, meta, testCase.name, testCase.size)
		}
	}
}

func TestParseFileMetaSizeCap(t *testing.T) {
	if _, err := ParseFileMeta([]byte("huge.bin 999999999999"), 1024); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("size cap must reject, got %v", err)
	}
}

func TestSafeFileNameSharedAuthority(t *testing.T) {
	// YMODEM consumes these so both protocols enforce one rule set.
	for _, name := range []string{"", "../x", "a/b", "a\\b", "C:x", "NUL", "PRN.log", "COM3", "LPT1.txt", ".", ".."} {
		if err := SafeFileName(name); err == nil {
			t.Fatalf("SafeFileName(%q) must reject", name)
		}
	}
	for _, name := range []string{"report.pdf", "firmware-v1.bin", "console.txt", "COM0"} {
		if err := SafeFileName(name); err != nil {
			t.Fatalf("SafeFileName(%q) must accept: %v", name, err)
		}
	}
}

func TestValidateSizeSharedCap(t *testing.T) {
	if err := ValidateSize(10, 100); err != nil {
		t.Fatalf("size within cap must pass: %v", err)
	}
	if err := ValidateSize(-1, 100); !errors.Is(err, ErrFileUnsafe) {
		t.Fatalf("negative size must reject, got %v", err)
	}
	if err := ValidateSize(200, 100); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("size over configured cap must reject, got %v", err)
	}
	if err := ValidateSize(MaxFileBytes+1, MaxFileBytes*2); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("size over the hard cap must reject even when the caller cap is larger, got %v", err)
	}
}

func TestCRC16KnownVector(t *testing.T) {
	// CRC-16/XMODEM of "123456789" is 0x31C3.
	if got := CRC16([]byte("123456789")); got != 0x31C3 {
		t.Fatalf("CRC16 mismatch: %04X", got)
	}
}

func TestParseBinaryHeaderValidAndTampered(t *testing.T) {
	header := binaryHeader(ZFILE, 123)
	frame, _, err := ParseBinaryHeader(header)
	if err != nil || frame.Type != FrameZFILE || frame.Sequence != 123 {
		t.Fatalf("%+v %v", frame, err)
	}
	header[len(header)-1] ^= 1
	if _, _, err = ParseBinaryHeader(header); !errors.Is(err, ErrCRCMismatch) {
		t.Fatal(err)
	}
}

func TestReceiverCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &Receiver{}
	if _, err := r.FeedSession(ctx, nil); !errors.Is(err, ErrCancelled) {
		t.Fatal(err)
	}
}

func TestBinaryHeaderBigEndianCRCAssembly(t *testing.T) {
	// Sanity: the CRC is transmitted big-endian, matching ParseBinaryHeader.
	body := []byte{'B', ZRINIT}
	crc := CRC16(body)
	assembled := binary.BigEndian.Uint16([]byte{byte(crc >> 8), byte(crc)})
	if assembled != crc {
		t.Fatal("big-endian assembly mismatch")
	}
}

func TestEncodeDecodeEscapedRoundTrip(t *testing.T) {
	// Every escaped class plus ordinary bytes and delimiter collisions.
	payload := []byte{0x00, 0x10, 0x11, 0x13, 0x18, 0x0d, 0x0a, 'a', 'b', 0xff, 0x18, 0x00}
	encoded := EncodeEscaped(payload)
	// A literal ZDLE must have been doubled, never left bare.
	for i := 0; i < len(encoded); i++ {
		if encoded[i] == ZDLE {
			if i+1 >= len(encoded) {
				t.Fatal("trailing bare ZDLE in encoded output")
			}
			i++
		}
	}
	decoded, used := DecodeEscaped(encoded)
	if used != len(encoded) {
		t.Fatalf("decode consumed %d of %d", used, len(encoded))
	}
	if string(decoded) != string(payload) {
		t.Fatalf("round trip mismatch: %x != %x", decoded, payload)
	}
}

func TestDecodeEscapedHandlesTrailingZDLE(t *testing.T) {
	// A buffer that ends mid-escape must report partial consumption, not panic
	// or silently drop the pair.
	decoded, used := DecodeEscaped([]byte{'a', ZDLE})
	if string(decoded) != "a" || used != 1 {
		t.Fatalf("decoded=%q used=%d", decoded, used)
	}
}

// headerFor builds a valid binary header for a frame type, mirroring what the
// sender emits on the wire.
func headerFor(kind byte) []byte { return binaryHeader(kind, 0) }

func TestSenderReceiverFileRoundTrip(t *testing.T) {
	payload := append([]byte("hello zmodem"), 24, 13, 10, 255)
	var got []byte
	var responses []byte
	receiver := &Receiver{OnChunk: func(b []byte) error { got = append(got, b...); return nil }, Write: func(b []byte) error { responses = append(responses, b...); return nil }}
	sender := &Sender{Write: func(b []byte) error {
		for _, v := range b {
			if _, err := receiver.FeedSession(context.Background(), []byte{v}); err != nil {
				return err
			}
		}
		return nil
	}, Read: func(_ context.Context, b []byte) (int, error) {
		n := copy(b, responses)
		responses = responses[n:]
		return n, nil
	}}
	if err := sender.SendFile(context.Background(), FileMeta{Name: "payload.bin"}, payload); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) || !receiver.Done {
		t.Fatalf("%q done=%v", got, receiver.Done)
	}
}

func TestSenderRejectsUnsafeMetadataBeforeWire(t *testing.T) {
	wrote := false
	sender := &Sender{Write: func([]byte) error { wrote = true; return nil }}
	if err := sender.SendFile(context.Background(), FileMeta{Name: "../evil"}, []byte("x")); !errors.Is(err, ErrFileUnsafe) {
		t.Fatalf("expected unsafe-name rejection, got %v", err)
	}
	if wrote {
		t.Fatal("nothing may reach the wire after a rejected name")
	}
}

func TestSenderRejectsOversizePayload(t *testing.T) {
	sender := &Sender{Write: func([]byte) error { return nil }}
	oversize := make([]byte, MaxFileBytes+1)
	if err := sender.SendFile(context.Background(), FileMeta{Name: "big.bin"}, oversize); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("expected size rejection, got %v", err)
	}
}

func TestReceiverRejectsZDATABeforeZFILE(t *testing.T) {
	receiver := &Receiver{}
	if _, err := receiver.FeedSession(context.Background(), framedUnit(ZDATA, []byte("orphan"))); !errors.Is(err, ErrFrameMalformed) {
		t.Fatalf("ZDATA before ZFILE must fail, got %v", err)
	}
}

func TestReceiverHonoursDeclaredSizeCap(t *testing.T) {
	receiver := &Receiver{MaxFileBytes: 4}
	if _, err := receiver.FeedSession(context.Background(), framedUnit(ZFILE, []byte("big.bin 100"))); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("expected size cap rejection, got %v", err)
	}
}

func TestFeedSessionPartialHeaderWaits(t *testing.T) {
	// A truncated header must be buffered, not treated as a permanent error.
	receiver := &Receiver{}
	consumed, err := receiver.FeedSession(context.Background(), []byte{ZPAD, ZDLE})
	if err != nil {
		t.Fatalf("partial header must wait: %v", err)
	}
	if consumed != 0 {
		t.Fatalf("nothing consumed, got %d", consumed)
	}
	// Completing the unit afterwards must still succeed.
	if _, err := receiver.FeedSession(context.Background(), framedUnit(ZRQINIT, nil)[2:]); err != nil {
		t.Fatalf("completed unit must parse: %v", err)
	}
}

func TestReceiverToleratesLeadingNoise(t *testing.T) {
	var got []byte
	receiver := &Receiver{
		OnFileStart: func(FileMeta) error { return nil },
		OnChunk:     func(c []byte) error { got = append(got, c...); return nil },
	}
	wire := append([]byte("login: "), framedUnit(ZFILE, []byte("a.txt 5"))...)
	if _, err := receiver.FeedSession(context.Background(), wire); err != nil {
		t.Fatalf("leading noise must be skipped: %v", err)
	}
	if _, err := receiver.FeedSession(context.Background(), framedUnit(ZDATA, []byte("hello"))); err != nil {
		t.Fatalf("data after noise: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("payload %q", got)
	}
}

func TestReceiverRejectsOversizeUnterminatedPacket(t *testing.T) {
	r := &Receiver{}
	wire := append(binaryHeader(ZFILE, 0), bytes.Repeat([]byte{97}, maxSubpacket+1)...)
	if _, err := r.FeedSession(context.Background(), wire); !errors.Is(err, ErrFileTooLarge) {
		t.Fatal(err)
	}
}

func framedUnit(kind byte, payload []byte) []byte {
	out := binaryHeader(kind, 0)
	if kind == ZFILE || kind == ZDATA {
		out = append(out, dataPacket(payload, zcrcw)...)
	}
	return out
}
