// Package ymodem owns the YMODEM file-transfer session engine (P3-08.2,
// TERM-03.2): the 128/1024-byte block protocol used by serial transfers. It
// shares the filename/size safety authority with the zmodem package so no name
// or size can bypass validation by arriving over the other protocol.
//
// The engine is transport-neutral: it reads and writes through the ByteStream
// interface, so the serial session drives it while tests drive it with a pipe.
package ymodem

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/binaricat/netcatty/internal/terminal/zmodem"
)

// Protocol control bytes.
const (
	SOH   byte = 0x01 // 128-byte block
	STX   byte = 0x02 // 1024-byte block
	EOT   byte = 0x04
	ACK   byte = 0x06
	NAK   byte = 0x15
	CAN   byte = 0x18
	CRCCh byte = 0x43 // 'C' - receiver requests CRC mode

	BlockSizeSmall = 128
	BlockSizeLarge = 1024
	// padData fills the tail of a data block (Ctrl-Z convention).
	padData byte = 0x1a
	// padInfo fills the tail of a file-info block. A receiver parses that tail
	// as text, so it must be NUL rather than 0x1a.
	padInfo byte = 0x00
)

var (
	ErrProtocol       = errors.New("ymodem: protocol error")
	ErrTimeout        = errors.New("ymodem: response timeout")
	ErrCancelled      = errors.New("ymodem: cancelled")
	ErrNoFile         = errors.New("ymodem: sender offered no file")
	ErrTooManyRetries = errors.New("ymodem: retry limit exceeded")
)

// ByteStream is the transport the engine runs over. Serial sessions supply a
// bounded-timeout read; tests supply an in-memory pipe.
type ByteStream interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
}

// BlockReader reads exactly one byte with a deadline. It exists as a separate
// interface so the serial adapter can express timeout semantics a plain
// io.Reader cannot.
type BlockReader interface {
	ReadTimedByte(timeout time.Duration) (byte, error)
}

// packetReader adapts a ByteStream to byte-at-a-time reads. When the stream
// also implements BlockReader its deadline call is preferred.
type packetReader struct {
	stream  ByteStream
	block   BlockReader
	timeout time.Duration
}

func newPacketReader(stream ByteStream, timeout time.Duration) *packetReader {
	reader := &packetReader{stream: stream, timeout: timeout}
	if block, ok := stream.(BlockReader); ok {
		reader.block = block
	}
	return reader
}

// newReadOnlyReader wraps an io.Reader for decode-only paths. The engine never
// writes while parsing a single block, so a nil ByteStream is safe here.
func newReadOnlyReader(source io.Reader, timeout time.Duration) *packetReader {
	return &packetReader{stream: readOnlyStream{Reader: source}, timeout: timeout}
}

// readOnlyStream adapts an io.Reader to ByteStream; Write fails loudly so a
// decode path can never silently mutate the wire.
type readOnlyStream struct{ io.Reader }

func (readOnlyStream) Write([]byte) (int, error) {
	return 0, errors.New("ymodem: stream is read-only")
}

func (r *packetReader) readByte(ctx context.Context) (byte, error) {
	if err := ctx.Err(); err != nil {
		return 0, ErrCancelled
	}
	if r.block != nil {
		return r.block.ReadTimedByte(r.timeout)
	}
	var buf [1]byte
	n, err := r.stream.Read(buf[:])
	if err != nil {
		return 0, err
	}
	if n != 1 {
		return 0, fmt.Errorf("%w: short read", ErrProtocol)
	}
	return buf[0], nil
}

// skipToBlock scans forward for a block or EOT introducer, tolerating the
// stray bytes that line noise or a peer's prompt echo can inject.
func (r *packetReader) skipToBlock(ctx context.Context, limit int) (byte, error) {
	for i := 0; i < limit; i++ {
		b, err := r.readByte(ctx)
		if err != nil {
			return 0, err
		}
		switch b {
		case SOH, STX, EOT:
			return b, nil
		case CAN:
			return 0, ErrCancelled
		}
	}
	return 0, fmt.Errorf("%w: no block introducer", ErrProtocol)
}

// CRC16 mirrors zmodem's CRC so both protocols verify with one implementation.
func CRC16(data []byte) uint16 { return zmodem.CRC16(data) }

// buildPacket assembles one YMODEM block: header, block number, complement,
// payload, CRC-16 big-endian.
func buildPacket(header byte, blockNumber byte, payload []byte, pad byte) []byte {
	size := BlockSizeSmall
	if header == STX {
		size = BlockSizeLarge
	}
	packet := make([]byte, 0, size+5)
	packet = append(packet, header, blockNumber, 0xFF-blockNumber)
	packet = append(packet, payload...)
	if len(payload) < size {
		padding := make([]byte, size-len(payload))
		for i := range padding {
			padding[i] = pad
		}
		packet = append(packet, padding...)
	}
	crc := CRC16(packet[3:])
	packet = append(packet, byte(crc>>8), byte(crc))
	return packet
}

// buildMetadataPayload encodes the YMODEM file-info block: "name size mode".
func buildMetadataPayload(name string, size int64) []byte {
	return []byte(fmt.Sprintf("%s %d 0 0", name, size))
}

// parseMetadataPayload returns the file name and size from a file-info block.
// Both NUL and the data-padding byte are stripped so a closing empty block
// reports ErrNoFile instead of masquerading as a file name.
func parseMetadataPayload(payload []byte) (string, int64, error) {
	text := strings.TrimRight(string(payload), "\x00\x1a ")
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", 0, ErrNoFile
	}
	name := fields[0]
	if err := zmodem.SafeFileName(name); err != nil {
		return "", 0, err
	}
	size := int64(0)
	if len(fields) > 1 {
		for _, r := range fields[1] {
			if r < '0' || r > '9' {
				return "", 0, fmt.Errorf("%w: non-numeric size", zmodem.ErrFileUnsafe)
			}
			size = size*10 + int64(r-'0')
		}
	}
	if err := zmodem.ValidateSize(size, zmodem.MaxFileBytes); err != nil {
		return "", 0, err
	}
	return name, size, nil
}

// SendOptions configures a YMODEM send.
type SendOptions struct {
	FileName string
	Data     []byte
	Timeout  time.Duration
	Retries  int
}

// SendResult reports a completed send.
type SendResult struct {
	FileName     string `json:"fileName"`
	TotalBytes   int64  `json:"totalBytes"`
	WrittenBytes int64  `json:"writtenBytes"`
}

// Send transmits one file to a waiting YMODEM receiver. The receiver must have
// announced CRC mode with 'C'; the sender offers the file-info block, streams
// the data blocks, closes the file with EOT, then sends the empty info block
// that terminates the session.
func Send(ctx context.Context, stream ByteStream, options SendOptions) (SendResult, error) {
	if err := zmodem.SafeFileName(options.FileName); err != nil {
		return SendResult{}, err
	}
	if err := zmodem.ValidateSize(int64(len(options.Data)), zmodem.MaxFileBytes); err != nil {
		return SendResult{}, err
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	retries := options.Retries
	if retries <= 0 {
		retries = 10
	}
	reader := newPacketReader(stream, timeout)

	// Wait for the receiver's CRC-mode request.
	if err := awaitReady(ctx, reader, retries); err != nil {
		return SendResult{}, err
	}

	metadata := buildMetadataPayload(filepath.Base(options.FileName), int64(len(options.Data)))
	infoHeader := byte(SOH)
	if len(metadata) > BlockSizeSmall {
		infoHeader = STX
	}
	if err := sendPacket(ctx, stream, reader, infoHeader, 0, metadata, padInfo, retries); err != nil {
		return SendResult{}, err
	}

	blockNumber := byte(1)
	for offset := 0; offset < len(options.Data); offset += BlockSizeLarge {
		end := offset + BlockSizeLarge
		if end > len(options.Data) {
			end = len(options.Data)
		}
		if err := sendPacket(ctx, stream, reader, STX, blockNumber, options.Data[offset:end], padData, retries); err != nil {
			return SendResult{}, err
		}
		blockNumber++
	}

	// Close the file, then terminate the session with the empty info block.
	if err := awaitAck(ctx, stream, reader, EOT, retries); err != nil {
		return SendResult{}, err
	}
	if err := awaitReady(ctx, reader, retries); err != nil {
		return SendResult{}, err
	}
	if err := sendPacket(ctx, stream, reader, SOH, 0, nil, padInfo, retries); err != nil {
		return SendResult{}, err
	}

	total := int64(len(options.Data))
	return SendResult{FileName: filepath.Base(options.FileName), TotalBytes: total, WrittenBytes: total}, nil
}

// awaitReady waits for a byte that signals the peer is ready ('C' or NAK).
func awaitReady(ctx context.Context, reader *packetReader, retries int) error {
	for attempt := 0; attempt <= retries; attempt++ {
		b, err := reader.readByte(ctx)
		if err != nil {
			if errors.Is(err, ErrCancelled) {
				return ErrCancelled
			}
			continue
		}
		switch b {
		case CRCCh, NAK:
			return nil
		case CAN:
			return ErrCancelled
		}
	}
	return ErrTimeout
}

// sendPacket writes one packet and waits for its ACK, retrying on NAK.
func sendPacket(ctx context.Context, stream ByteStream, reader *packetReader, header, blockNumber byte, payload []byte, pad byte, retries int) error {
	packet := buildPacket(header, blockNumber, payload, pad)
	for attempt := 0; attempt <= retries; attempt++ {
		if _, err := stream.Write(packet); err != nil {
			return err
		}
		response, err := reader.readByte(ctx)
		if err != nil {
			if errors.Is(err, ErrCancelled) {
				return ErrCancelled
			}
			continue
		}
		switch response {
		case ACK:
			return nil
		case CAN:
			return ErrCancelled
		case NAK:
			continue
		}
	}
	return ErrTooManyRetries
}

// awaitAck writes a bare control byte (EOT) and waits for its ACK.
func awaitAck(ctx context.Context, stream ByteStream, reader *packetReader, control byte, retries int) error {
	for attempt := 0; attempt <= retries; attempt++ {
		if _, err := stream.Write([]byte{control}); err != nil {
			return err
		}
		response, err := reader.readByte(ctx)
		if err != nil {
			if errors.Is(err, ErrCancelled) {
				return ErrCancelled
			}
			continue
		}
		switch response {
		case ACK:
			return nil
		case CAN:
			return ErrCancelled
		case NAK:
			continue
		}
	}
	return ErrTooManyRetries
}

// ReceiveResult reports one received file.
type ReceiveResult struct {
	FileName     string `json:"fileName"`
	FilePath     string `json:"filePath"`
	TotalBytes   int64  `json:"totalBytes"`
	WrittenBytes int64  `json:"writtenBytes"`
}

// ReceiveOptions configures a YMODEM receive.
type ReceiveOptions struct {
	// DestinationDir is where accepted files are written. The engine refuses to
	// escape it: the offered name is validated as a base name first.
	DestinationDir string
	Timeout        time.Duration
	Retries        int
	// MaxBytes caps the accepted payload per file; zero uses the shared cap.
	MaxBytes int64
}

// Receive reads an incoming YMODEM session and writes accepted files under the
// destination directory. It returns every completed file.
func Receive(ctx context.Context, stream ByteStream, options ReceiveOptions) ([]ReceiveResult, error) {
	if options.DestinationDir == "" {
		return nil, fmt.Errorf("%w: destination directory required", ErrProtocol)
	}
	maxBytes := options.MaxBytes
	if maxBytes <= 0 {
		maxBytes = zmodem.MaxFileBytes
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	retries := options.Retries
	if retries <= 0 {
		retries = 10
	}
	reader := newPacketReader(stream, timeout)

	// Ask the sender to start in CRC mode.
	if _, err := stream.Write([]byte{CRCCh}); err != nil {
		return nil, err
	}

	var results []ReceiveResult
	for {
		header, payload, err := readBlockWithRetry(ctx, stream, reader, retries)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return results, nil
			}
			return results, err
		}
		if header == EOT {
			_, _ = stream.Write([]byte{ACK})
			return results, nil
		}
		name, size, metaErr := parseMetadataPayload(payload)
		if metaErr != nil {
			if errors.Is(metaErr, ErrNoFile) {
				// Empty info block closes the session.
				_, _ = stream.Write([]byte{ACK})
				return results, nil
			}
			_, _ = stream.Write([]byte{NAK})
			return results, metaErr
		}
		if err := zmodem.ValidateSize(size, maxBytes); err != nil {
			_, _ = stream.Write([]byte{NAK})
			return results, err
		}
		// ACK the info block before the sender streams data.
		_, _ = stream.Write([]byte{ACK})
		result, writeErr := receiveFile(ctx, stream, reader, options.DestinationDir, name, size, maxBytes, retries)
		if writeErr != nil {
			return results, writeErr
		}
		results = append(results, result)
		// Re-arm CRC mode so the sender offers the closing (empty) info block
		// that terminates the session.
		if _, err := stream.Write([]byte{CRCCh}); err != nil {
			return results, err
		}
	}
}

// receiveFile accepts the data blocks for one advertised file and writes them.
// It ACKs each block and stops at the sender's EOT.
func receiveFile(ctx context.Context, stream ByteStream, reader *packetReader, dir, name string, size, maxBytes int64, retries int) (ReceiveResult, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ReceiveResult{}, err
	}
	target := filepath.Join(dir, name)
	// Defence in depth: the validated base name must still resolve inside dir.
	if rel, err := filepath.Rel(dir, target); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return ReceiveResult{}, fmt.Errorf("%w: escapes destination", zmodem.ErrFileUnsafe)
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return ReceiveResult{}, err
	}
	defer file.Close()

	written := int64(0)
	for {
		header, payload, err := readBlockWithRetry(ctx, stream, reader, retries)
		if err != nil {
			return ReceiveResult{}, err
		}
		if header == EOT {
			_, _ = stream.Write([]byte{ACK})
			break
		}
		if written+int64(len(payload)) > maxBytes {
			_, _ = stream.Write([]byte{NAK})
			return ReceiveResult{}, fmt.Errorf("%w: exceeds cap", zmodem.ErrFileTooLarge)
		}
		if size > 0 && written+int64(len(payload)) > size {
			payload = payload[:size-written]
		}
		if _, err := file.Write(payload); err != nil {
			return ReceiveResult{}, err
		}
		written += int64(len(payload))
		_, _ = stream.Write([]byte{ACK})
	}
	if err := file.Sync(); err != nil {
		return ReceiveResult{}, err
	}
	return ReceiveResult{FileName: name, FilePath: target, TotalBytes: size, WrittenBytes: written}, nil
}

// readBlockWithRetry reads one verified block, NAKing and retrying on a
// malformed frame or CRC failure.
func readBlockWithRetry(ctx context.Context, stream ByteStream, reader *packetReader, retries int) (byte, []byte, error) {
	var lastErr error = ErrProtocol
	for attempt := 0; attempt <= retries; attempt++ {
		header, payload, err := readBlock(ctx, reader)
		if err == nil {
			return header, payload, nil
		}
		lastErr = err
		if errors.Is(err, ErrCancelled) {
			return 0, nil, err
		}
		if errors.Is(err, io.EOF) {
			return 0, nil, err
		}
		if _, writeErr := stream.Write([]byte{NAK}); writeErr != nil {
			return 0, nil, writeErr
		}
	}
	return 0, nil, lastErr
}

// readBlock reads and CRC-verifies exactly one block or an EOT.
func readBlock(ctx context.Context, reader *packetReader) (byte, []byte, error) {
	header, err := reader.skipToBlock(ctx, BlockSizeLarge+8)
	if err != nil {
		return 0, nil, err
	}
	if header == EOT {
		return EOT, nil, nil
	}
	size := BlockSizeSmall
	if header == STX {
		size = BlockSizeLarge
	}
	var blockNumber, complement byte
	if blockNumber, err = reader.readByte(ctx); err != nil {
		return 0, nil, err
	}
	if complement, err = reader.readByte(ctx); err != nil {
		return 0, nil, err
	}
	if complement != 0xFF-blockNumber {
		return 0, nil, fmt.Errorf("%w: block number complement", ErrProtocol)
	}
	payload := make([]byte, size)
	for i := range payload {
		b, err := reader.readByte(ctx)
		if err != nil {
			return 0, nil, err
		}
		payload[i] = b
	}
	var crcBytes [2]byte
	for i := range crcBytes {
		b, err := reader.readByte(ctx)
		if err != nil {
			return 0, nil, err
		}
		crcBytes[i] = b
	}
	expected := uint16(crcBytes[0])<<8 | uint16(crcBytes[1])
	if CRC16(payload) != expected {
		return 0, nil, zmodem.ErrCRCMismatch
	}
	return header, payload, nil
}

// ByteReaderStream adapts an io.Reader/io.Writer pair (used by tests and by
// serial adapters that expose a raw handle rather than a BlockReader).
type ByteReaderStream struct {
	Reader io.Reader
	Writer io.Writer
}

func (s ByteReaderStream) Read(p []byte) (int, error)  { return s.Reader.Read(p) }
func (s ByteReaderStream) Write(p []byte) (int, error) { return s.Writer.Write(p) }
