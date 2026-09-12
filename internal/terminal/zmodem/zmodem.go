// Package zmodem owns the ZMODEM/YMODEM service boundary (P3-08.4): frame
// parsing with CRC verification, file-metadata safety (name sanitisation and
// size caps), and context-driven cancellation. The byte transport is injected;
// this package is the single authority for what may enter the filesystem.
package zmodem

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
)

// Protocol bytes.
const (
	ZDLE    byte = 0x18
	ZPAD    byte = 0x2A // '*'
	ZRQINIT byte = 0x00
	ZRINIT  byte = 0x01
	ZSINIT  byte = 0x02
	ZACK    byte = 0x03
	ZFILE   byte = 0x04
	ZSKIP   byte = 0x05
	ZNAK    byte = 0x06
	ZABORT  byte = 0x07
	ZRPOS   byte = 0x09
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
	name, attributes, raw := strings.Cut(string(payload), "\x00")
	if !raw {
		fields := strings.Fields(name)
		if len(fields) == 0 {
			return FileMeta{}, fmt.Errorf("%w: empty ZFILE payload", ErrFileUnsafe)
		}
		name = fields[0]
		attributes = strings.Join(fields[1:], " ")
	}
	if err := validateName(name); err != nil {
		return FileMeta{}, err
	}
	size := int64(0)
	fields := strings.Fields(strings.TrimRight(attributes, "\x00"))
	if len(fields) > 0 {
		var err error
		size, err = strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			return FileMeta{}, fmt.Errorf("%w: invalid size", ErrFileUnsafe)
		}
	}
	if err := ValidateSize(size, maxBytes); err != nil {
		return FileMeta{}, err
	}
	return FileMeta{Name: name, Size: size}, nil
}

func validateName(name string) error {
	if name == "" || len(name) > 255 {
		return fmt.Errorf("%w: name length", ErrFileUnsafe)
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return fmt.Errorf("%w: control character in name", ErrFileUnsafe)
		}
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
	if upper == "." || upper == ".." {
		return fmt.Errorf("%w: reserved name", ErrFileUnsafe)
	}
	// Windows resolves reserved device names with or without an extension, so
	// "NUL.txt" is still the NUL device. Check the stem, not the whole name.
	stem := upper
	if dot := strings.IndexByte(stem, '.'); dot >= 0 {
		stem = stem[:dot]
	}
	if isReservedDevice(strings.TrimRight(stem, " ")) {
		return fmt.Errorf("%w: reserved name", ErrFileUnsafe)
	}
	return nil
}

// SafeFileName is the exported entry point to the package's filename safety
// contract. YMODEM shares this single authority instead of re-deriving the
// rules, so a name rejected for ZFILE can never slip in through a YMODEM block.
func SafeFileName(name string) error { return validateName(name) }

// ValidateSize enforces the per-file byte cap shared by ZMODEM and YMODEM.
func ValidateSize(size, maxBytes int64) error {
	if size < 0 {
		return fmt.Errorf("%w: negative size", ErrFileUnsafe)
	}
	if size > maxBytes || size > MaxFileBytes {
		return fmt.Errorf("%w: %d bytes", ErrFileTooLarge, size)
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

// ParseBinaryHeader parses standard hex, binary CRC16, and binary CRC32 headers.
func ParseBinaryHeader(data []byte) (Frame, int, error) {
	h, used, err := parseHeader(data)
	if err != nil {
		return Frame{}, 0, err
	}
	return Frame{Type: frameTypeName(h.kind), Sequence: h.position, CRCValid: true}, used, nil
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
	// state carries per-file routing across streaming Feed calls (session.go).
	state *feedState
	// decoder buffers partial framed units across FeedSession calls.
	decoder   *wireDecoder
	Write     func([]byte) error
	OnFileEnd func(FileMeta) error
	Done      bool
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
