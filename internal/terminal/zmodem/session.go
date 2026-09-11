// ZMODEM session engine (P3-08.4, TERM-03.4): the frame stream that drives a
// real rz/sz exchange. The primitives in zmodem.go (CRC, metadata safety, size
// caps) remain the single authority; this file adds the byte-stream state
// machine that routes a received header into validated file events, plus the
// matching sender so uploads and downloads share one framing implementation.
//
// Framing note: a bare ZMODEM binary header carries no payload length, and a
// delimiter scan is genuinely ambiguous because an escaped 0x02 encodes to
// ZDLE 'B' — the same bytes that open a header. Netcatty therefore frames each
// session unit as [binary header][uint32 big-endian payload length][escaped
// payload]. Header parsing itself is unchanged (ParseBinaryHeader is shared
// with the frozen probe fixtures); only the length prefix is Netcatty's, so
// this codec is the data-plane contract between Netcatty peers and is NOT a
// claim of raw lrzsz wire compatibility. Real-peer interop stays on the
// existing residual-risk list.
package zmodem

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// ZDLE escape classes. A ZDLE-prefixed byte is decoded as byte ^ 0x40 for the
// control characters ZMODEM must protect; a doubled ZDLE is a literal ZDLE.
var zmodemEscaped = map[byte]bool{
	0x10: true, // DLE
	0x11: true, // XON
	0x13: true, // XOFF
	0x18: true, // ZDLE
	0x0d: true, // CR
	0x0a: true, // LF
	0x00: true, // NUL
}

// EncodeEscaped applies ZMODEM ZDLE quoting to a payload for the wire.
func EncodeEscaped(payload []byte) []byte {
	out := make([]byte, 0, len(payload))
	for _, b := range payload {
		switch {
		case b == ZDLE:
			out = append(out, ZDLE, ZDLE)
		case zmodemEscaped[b]:
			out = append(out, ZDLE, b^0x40)
		default:
			out = append(out, b)
		}
	}
	return out
}

// DecodeEscaped reverses EncodeEscaped. It returns the decoded bytes and the
// number of input bytes consumed, so a caller can resume mid-stream.
func DecodeEscaped(data []byte) ([]byte, int) {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		b := data[i]
		if b != ZDLE {
			out = append(out, b)
			continue
		}
		if i+1 >= len(data) {
			// Trailing ZDLE: signal the caller to wait for more input.
			return out, i
		}
		next := data[i+1]
		if next == ZDLE {
			out = append(out, ZDLE)
			i++
			continue
		}
		out = append(out, next^0x40)
		i++
	}
	return out, len(data)
}

// encodeHeader builds the binary header bytes for a frame type.
func encodeHeader(frameType byte) []byte {
	crc := CRC16([]byte{'B', frameType})
	return []byte{ZPAD, ZDLE, 'B', frameType, byte(crc >> 8), byte(crc)}
}

// headerLength is the fixed size of the binary header this package emits.
const headerLength = 6

// zdataChunkSize bounds one ZDATA payload.
const zdataChunkSize = 1024

// Sender emits a ZMODEM file transfer from a validated in-memory payload.
type Sender struct {
	// Write receives the encoded session bytes in order. When nil the transfer
	// is validated and discarded, which is useful for dry-run checks.
	Write func([]byte) error
}

// SendFile streams one file as ZFILE + ZDATA + ZEOF. The metadata is validated
// with the same rules the receiver applies, so a bad name or oversize payload
// fails before anything reaches the wire.
func (s *Sender) SendFile(ctx context.Context, meta FileMeta, data []byte) error {
	if err := validateName(meta.Name); err != nil {
		return err
	}
	if err := ValidateSize(int64(len(data)), MaxFileBytes); err != nil {
		return err
	}
	if err := s.emit(ctx, ZFILE, []byte(fmt.Sprintf("%s %d", meta.Name, len(data)))); err != nil {
		return err
	}
	for offset := 0; offset < len(data); offset += zdataChunkSize {
		if err := ctx.Err(); err != nil {
			return ErrCancelled
		}
		end := offset + zdataChunkSize
		if end > len(data) {
			end = len(data)
		}
		if err := s.emit(ctx, ZDATA, data[offset:end]); err != nil {
			return err
		}
	}
	return s.emit(ctx, ZEOF, nil)
}

// emit writes one framed unit: header, length prefix, escaped payload.
func (s *Sender) emit(ctx context.Context, frameType byte, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return ErrCancelled
	}
	if s.Write == nil {
		return nil
	}
	escaped := EncodeEscaped(payload)
	unit := make([]byte, 0, headerLength+4+len(escaped))
	unit = append(unit, encodeHeader(frameType)...)
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(escaped)))
	unit = append(unit, length[:]...)
	unit = append(unit, escaped...)
	return s.Write(unit)
}

// frameDecoder accumulates transport bytes until a complete framed unit is
// available, then yields the parsed header plus decoded payload.
type frameDecoder struct {
	buffer []byte
}

// next returns one decoded unit. ok is false when more bytes are required.
// A returned error is terminal: the receiver must abort the session.
func (d *frameDecoder) next() (Frame, []byte, bool, error) {
	if len(d.buffer) < headerLength {
		return Frame{}, nil, false, nil
	}
	// Locate the header at the buffer start; tolerate leading noise because the
	// transport may surface a peer's prompt echo before the first frame.
	if !isHeaderPrefix(d.buffer) {
		index := indexHeaderPrefix(d.buffer)
		if index < 0 {
			// Keep only a tail that could still be a split prefix.
			if len(d.buffer) > headerLength {
				d.buffer = d.buffer[len(d.buffer)-(headerLength-1):]
			}
			return Frame{}, nil, false, nil
		}
		d.buffer = d.buffer[index:]
		if len(d.buffer) < headerLength {
			return Frame{}, nil, false, nil
		}
	}
	frame, consumed, err := ParseBinaryHeader(d.buffer)
	if err != nil {
		if errors.Is(err, ErrFrameMalformed) {
			return Frame{}, nil, false, nil
		}
		return Frame{}, nil, false, err
	}
	if len(d.buffer) < consumed+4 {
		return Frame{}, nil, false, nil
	}
	payloadLength := binary.BigEndian.Uint32(d.buffer[consumed : consumed+4])
	if payloadLength > MaxFileBytes {
		return Frame{}, nil, false, fmt.Errorf("%w: declared payload %d", ErrFileTooLarge, payloadLength)
	}
	if uint64(len(d.buffer)) < uint64(consumed)+4+uint64(payloadLength) {
		return Frame{}, nil, false, nil
	}
	start := consumed + 4
	end := start + int(payloadLength)
	decoded, _ := DecodeEscaped(d.buffer[start:end])
	d.buffer = d.buffer[end:]
	frame.Payload = decoded
	return frame, decoded, true, nil
}

func isHeaderPrefix(data []byte) bool {
	return len(data) >= 3 && data[0] == ZPAD && data[1] == ZDLE && data[2] == 'B'
}

func indexHeaderPrefix(data []byte) int {
	for i := 0; i+3 <= len(data); i++ {
		if data[i] == ZPAD && data[i+1] == ZDLE && data[i+2] == 'B' {
			return i
		}
	}
	return -1
}

// feedState carries the per-file routing state across Feed calls.
type feedState struct {
	pendingName  string
	declaredSize int64
	written      int64
	inFile       bool
}

// FeedSession consumes transport bytes, routing decoded frames into the
// receiver's sink callbacks. Partial units are buffered for the next call.
func (r *Receiver) FeedSession(ctx context.Context, data []byte) (int, error) {
	if r.state == nil {
		r.state = &feedState{}
	}
	if r.decoder == nil {
		r.decoder = &frameDecoder{}
	}
	r.decoder.buffer = append(r.decoder.buffer, data...)
	consumed := 0
	for {
		if err := ctx.Err(); err != nil {
			return consumed, ErrCancelled
		}
		frame, payload, ok, err := r.decoder.next()
		if err != nil {
			return consumed, err
		}
		if !ok {
			return consumed, nil
		}
		consumed += len(payload)
		if err := r.routeFrame(frame, payload); err != nil {
			return consumed, err
		}
	}
}

// routeFrame applies one decoded frame to the session state.
func (r *Receiver) routeFrame(frame Frame, payload []byte) error {
	switch frame.Type {
	case FrameZFILE:
		meta, err := ParseFileMeta(payload, r.maxBytes())
		if err != nil {
			return err
		}
		if r.OnFileStart != nil {
			if err := r.OnFileStart(meta); err != nil {
				return err
			}
		}
		r.state.pendingName = meta.Name
		r.state.declaredSize = meta.Size
		r.state.written = 0
		r.state.inFile = true
	case FrameZDATA:
		if !r.state.inFile {
			return fmt.Errorf("%w: ZDATA before ZFILE", ErrFrameMalformed)
		}
		chunk := payload
		if r.state.declaredSize > 0 && r.state.written+int64(len(chunk)) > r.state.declaredSize {
			chunk = chunk[:r.state.declaredSize-r.state.written]
		}
		if r.OnChunk != nil && len(chunk) > 0 {
			if err := r.OnChunk(chunk); err != nil {
				return err
			}
		}
		r.state.written += int64(len(chunk))
	case FrameZEOF, FrameZFIN:
		r.state.inFile = false
	}
	return nil
}

func (r *Receiver) maxBytes() int64 {
	if r.MaxFileBytes > 0 {
		return r.MaxFileBytes
	}
	return MaxFileBytes
}

// SessionReader drives a whole transfer from an io.Reader until EOF.
func (r *Receiver) SessionReader(ctx context.Context, source io.Reader) error {
	if r.state == nil {
		r.state = &feedState{}
	}
	buffer := make([]byte, 4096)
	for {
		if err := ctx.Err(); err != nil {
			return ErrCancelled
		}
		n, err := source.Read(buffer)
		if n > 0 {
			if _, feedErr := r.FeedSession(ctx, buffer[:n]); feedErr != nil {
				return feedErr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}
