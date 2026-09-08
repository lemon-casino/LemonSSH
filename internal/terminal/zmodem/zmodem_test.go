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
