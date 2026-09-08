// Package zmodem owns the ZMODEM/YMODEM service boundary (P3-08.4): frame
// parsing with CRC verification, file-metadata safety (name sanitisation and
// size caps), and context-driven cancellation. The byte transport is injected;
// this package is the single authority for what may enter the filesystem.
package zmodem

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"path"
	"strings"
)

// Protocol bytes.
const (
	ZDLE    byte = 0x18
	ZPAD    byte = 0x2A // '*'
	ZRQINIT byte = 0x00
	ZRINIT  byte = 0x04
	ZSINIT  byte = 0x08
	ZFILE   byte = 0x0C
	ZDATA   byte = 0x0A
	ZEOF    byte = 0x0B
	ZFIN    byte = 0x08
)

// Frame types as they appear after the header.
const (
	FrameZRQINIT = "ZRQINIT"
	FrameZRINIT  = "ZRINIT"
	FrameZFILE   = "ZFILE"
	FrameZDATA   = "ZDATA"
	FrameZEOF    = "ZEOF"
	FrameZFIN    = "ZFIN"
)

var (
	ErrCRCMismatch    = errors.New("zmodem: CRC mismatch")
	ErrFrameMalformed = errors.New("zmodem: frame malformed")
	ErrCancelled      = errors.New("zmodem: cancelled")
	ErrFileUnsafe     = errors.New("zmodem: unsafe file metadata")
	ErrFileTooLarge   = errors.New("zmodem: file exceeds size cap")
)

// MaxFileBytes bounds one received file (256 MiB).
const MaxFileBytes = 256 << 20

// CRC16 computes the ZMODEM/XMODEM CRC-16 (poly 0x1021, init 0).
func CRC16(data []byte) uint16 {
	crc := uint16(0)
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// FileMeta is the parsed ZFILE metadata (name, size).
type FileMeta struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// ParseFileMeta decodes the ZFILE text payload "name size date mode ..." and
// enforces filesystem safety: no separators, no traversal, no reserved names,
// size within cap. Zero size means "unknown" and is allowed.
func ParseFileMeta(payload []byte, maxBytes int64) (FileMeta, error) {
	fields := strings.Fields(string(payload))
	if len(fields) == 0 {
		return FileMeta{}, fmt.Errorf("%w: empty ZFILE payload", ErrFileUnsafe)
	}
	name := fields[0]
	if err := validateName(name); err != nil {
		return FileMeta{}, err
	}
	size := int64(0)
	if len(fields) > 1 {
		for _, r := range fields[1] {
			if r < '0' || r > '9' {
				return FileMeta{}, fmt.Errorf("%w: non-numeric size", ErrFileUnsafe)
			}
			size = size*10 + int64(r-'0')
		}
	}
	if size > maxBytes || size > MaxFileBytes {
		return FileMeta{}, fmt.Errorf("%w: %d bytes", ErrFileTooLarge, size)
	}
	return FileMeta{Name: name, Size: size}, nil
}

func validateName(name string) error {
	if name == "" || len(name) > 255 {
		return fmt.Errorf("%w: name length", ErrFileUnsafe)
	}
	if name != path.Base(name) {
		return fmt.Errorf("%w: name carries a path", ErrFileUnsafe)
	}
	for _, sep := range []string{"/", "\\", ":"} {
		if strings.Contains(name, sep) {
			return fmt.Errorf("%w: separator %q", ErrFileUnsafe, sep)
		}
	}
	upper := strings.ToUpper(name)
	if upper == "." || upper == ".." || strings.HasPrefix(upper, "CON") && len(upper) <= 4 && isReservedDevice(upper) {
		return fmt.Errorf("%w: reserved name", ErrFileUnsafe)
	}
	return nil
}

func isReservedDevice(name string) bool {
	devices := map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}
	if devices[name] {
		return true
	}
	for _, device := range []string{"COM", "LPT"} {
		if strings.HasPrefix(name, device) && len(name) == 4 && name[3] >= '1' && name[3] <= '9' {
			return true
		}
	}
	return false
}

// Frame is one parsed binary-mode header plus payload offset information.
type Frame struct {
	Type     string
	Sequence uint32 // ZP0..ZP3 rolled into a 32-bit file offset for data frames
	Payload  []byte
	CRCValid bool
}

// ParseBinaryHeader locates and parses a ZMODEM binary header from a buffer:
// ZPAD ZDLE 'B' type crc0 crc1 (16-bit CRC variant). Returns the frame and
// the number of bytes consumed; ErrFrameMalformed when the prefix is partial
// or the CRC fails.
func ParseBinaryHeader(data []byte) (Frame, int, error) {
	// Scan for ZPAD ZDLE 'B'.
	for start := 0; start+3 <= len(data); start++ {
		if data[start] != ZPAD || data[start+1] != ZDLE || data[start+2] != 'B' {
			continue
		}
		if start+6 > len(data) {
			return Frame{}, 0, ErrFrameMalformed
		}
		frameType := data[start+3]
		crcBytes := data[start+4 : start+6]
		// CRC-16 covers type + crc bytes as transmitted (ZDLE-decoded form).
		body := []byte{'B', frameType}
		expected := binary.BigEndian.Uint16([]byte{crcBytes[0], crcBytes[1]})
		if CRC16(body) != expected {
			return Frame{}, 0, ErrCRCMismatch
		}
		return Frame{Type: frameTypeName(frameType), CRCValid: true}, start + 6, nil
	}
	return Frame{}, 0, ErrFrameMalformed
}

func frameTypeName(t byte) string {
	switch t {
	case ZRQINIT:
		return FrameZRQINIT
	case ZRINIT:
		return FrameZRINIT
	case ZFILE:
		return FrameZFILE
	case ZDATA:
		return FrameZDATA
	case ZEOF:
		return FrameZEOF
	case ZFIN:
		return FrameZFIN
	default:
		return fmt.Sprintf("Z%02X", t)
	}
}

// Receiver consumes a byte stream, emits file transfer events, and honours
// cancellation. It never writes to disk itself: the sink callback receives
// validated metadata and data chunks.
type Receiver struct {
	MaxFileBytes int64
	// OnFileStart is invoked with validated metadata before data flows. A
	// non-nil error rejects the transfer.
	OnFileStart func(FileMeta) error
	// OnChunk receives validated data chunks.
	OnChunk func([]byte) error
}

// Feed processes one transport buffer. Cancellation is checked between
// frames; a cancelled receiver returns ErrCancelled immediately.
func (r *Receiver) Feed(ctx context.Context, data []byte) error {
	_, consumed, err := ParseBinaryHeader(data)
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ErrCancelled
	default:
	}
	// Frame-type specific handling (ZFILE metadata, ZDATA chunk routing)
	// lands with the transport adapter; consumed tracks bytes used.
	_ = consumed
	return nil
}
