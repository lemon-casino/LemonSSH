package zmodem

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
)

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
	frameType := byte(ZFILE)
	body := []byte{'B', frameType}
	crc := CRC16(body)
	header := []byte{ZPAD, ZDLE, 'B', frameType, byte(crc >> 8), byte(crc)}

	frame, consumed, err := ParseBinaryHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if !frame.CRCValid || frame.Type != FrameZFILE || consumed != 6 {
		t.Fatalf("frame mismatch: %+v consumed=%d", frame, consumed)
	}
	header[4] ^= 1
	if _, _, err := ParseBinaryHeader(header); !errors.Is(err, ErrCRCMismatch) {
		t.Fatalf("tampered CRC must fail: %v", err)
	}
}

func TestReceiverCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	receiver := &Receiver{}
	frameType := byte(ZRQINIT)
	crc := CRC16([]byte{'B', frameType})
	header := []byte{ZPAD, ZDLE, 'B', frameType, byte(crc >> 8), byte(crc)}
	if err := receiver.Feed(ctx, header); !errors.Is(err, ErrCancelled) {
		t.Fatalf("cancelled receiver must fail immediately, got %v", err)
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
func headerFor(frameType byte) []byte {
	crc := CRC16([]byte{'B', frameType})
	return []byte{ZPAD, ZDLE, 'B', frameType, byte(crc >> 8), byte(crc)}
}

func TestSenderReceiverFileRoundTrip(t *testing.T) {
	payload := []byte("hello zmodem \x18 escaped \r\n bytes")
	meta := FileMeta{Name: "payload.bin", Size: int64(len(payload))}

	var wire []byte
	sender := &Sender{Write: func(unit []byte) error {
		wire = append(wire, unit...)
		return nil
	}}
	if err := sender.SendFile(context.Background(), meta, payload); err != nil {
		t.Fatal(err)
	}

	var gotName string
	var gotData []byte
	var started bool
	receiver := &Receiver{
		OnFileStart: func(meta FileMeta) error {
			started = true
			gotName = meta.Name
			if meta.Size != int64(len(payload)) {
				t.Fatalf("declared size %d", meta.Size)
			}
			return nil
		},
		OnChunk: func(chunk []byte) error {
			gotData = append(gotData, chunk...)
			return nil
		},
	}
	// Feed the wire in small pieces to prove cross-call state is preserved.
	for offset := 0; offset < len(wire); offset += 3 {
		end := offset + 3
		if end > len(wire) {
			end = len(wire)
		}
		if _, err := receiver.FeedSession(context.Background(), wire[offset:end]); err != nil {
			t.Fatalf("feed at %d: %v", offset, err)
		}
	}
	if !started {
		t.Fatal("OnFileStart never fired")
	}
	if gotName != "payload.bin" {
		t.Fatalf("name %q", gotName)
	}
	if string(gotData) != string(payload) {
		t.Fatalf("data mismatch: %q != %q", gotData, payload)
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
	if _, err := receiver.FeedSession(context.Background(), framedUnit(ZEOF, nil)[2:]); err != nil {
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

func TestReceiverRejectsOversizeLengthPrefix(t *testing.T) {
	// A hostile length prefix must be rejected before allocating a payload.
	unit := append(append([]byte{}, headerFor(ZFILE)...), 0xFF, 0xFF, 0xFF, 0xFF)
	receiver := &Receiver{}
	if _, err := receiver.FeedSession(context.Background(), unit); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("expected length-prefix rejection, got %v", err)
	}
}

// framedUnit builds one session unit the way the sender does, for tests that
// need to hand-craft a frame.
func framedUnit(frameType byte, payload []byte) []byte {
	escaped := EncodeEscaped(payload)
	unit := append([]byte{}, headerFor(frameType)...)
	unit = append(unit, byte(len(escaped)>>24), byte(len(escaped)>>16), byte(len(escaped)>>8), byte(len(escaped)))
	return append(unit, escaped...)
}
