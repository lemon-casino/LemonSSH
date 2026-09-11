package ymodem

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/binaricat/netcatty/internal/terminal/zmodem"
)

// pipeStream is a duplex in-memory transport: writes to one end are readable
// from the other. It lets the real send and receive engines drive each other.
type pipeStream struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

func (s pipeStream) Read(p []byte) (int, error)  { return s.reader.Read(p) }
func (s pipeStream) Write(p []byte) (int, error) { return s.writer.Write(p) }

func duplex() (pipeStream, pipeStream) {
	leftRead, rightWrite := io.Pipe()
	rightRead, leftWrite := io.Pipe()
	return pipeStream{reader: leftRead, writer: leftWrite}, pipeStream{reader: rightRead, writer: rightWrite}
}

func TestSendAndReceiveRoundTrip(t *testing.T) {
	sender, receiver := duplex()
	payload := bytes.Repeat([]byte("lemon-ssh-ymodem!"), 200) // spans multiple blocks
	destination := t.TempDir()

	var sendResult SendResult
	var sendErr error
	var receiveResults []ReceiveResult
	var receiveErr error
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		defer sender.writer.Close()
		sendResult, sendErr = Send(context.Background(), sender, SendOptions{
			FileName: "firmware.bin",
			Data:     payload,
			Timeout:  time.Second,
			Retries:  20,
		})
	}()
	go func() {
		defer wg.Done()
		defer receiver.writer.Close()
		receiveResults, receiveErr = Receive(context.Background(), receiver, ReceiveOptions{
			DestinationDir: destination,
			Timeout:        time.Second,
			Retries:        20,
		})
	}()
	wg.Wait()

	if sendErr != nil {
		t.Fatalf("send: %v", sendErr)
	}
	if receiveErr != nil {
		t.Fatalf("receive: %v", receiveErr)
	}
	if sendResult.FileName != "firmware.bin" || sendResult.WrittenBytes != int64(len(payload)) {
		t.Fatalf("unexpected send result: %+v", sendResult)
	}
	if len(receiveResults) != 1 {
		t.Fatalf("expected exactly one file, got %+v", receiveResults)
	}
	received := receiveResults[0]
	if received.FileName != "firmware.bin" {
		t.Fatalf("name: %q", received.FileName)
	}
	written, err := os.ReadFile(received.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(written, payload) {
		t.Fatalf("payload mismatch: %d bytes written vs %d sent", len(written), len(payload))
	}
}

func TestSendRejectsUnsafeFileName(t *testing.T) {
	sender, receiver := duplex()
	defer sender.writer.Close()
	defer receiver.writer.Close()
	for _, name := range []string{"../escape.bin", "sub/dir.bin", "NUL", ""} {
		if _, err := Send(context.Background(), sender, SendOptions{FileName: name, Data: []byte("x")}); err == nil {
			t.Fatalf("name %q must be rejected", name)
		}
	}
}

func TestSendRejectsOversizePayload(t *testing.T) {
	sender, receiver := duplex()
	defer sender.writer.Close()
	defer receiver.writer.Close()
	// A payload one byte over the shared cap must fail closed before any block.
	oversize := make([]byte, zmodem.MaxFileBytes+1)
	if _, err := Send(context.Background(), sender, SendOptions{FileName: "big.bin", Data: oversize}); !errors.Is(err, zmodem.ErrFileTooLarge) {
		t.Fatalf("expected size rejection, got %v", err)
	}
}

func TestReceiveRejectsTraversalName(t *testing.T) {
	// Craft the session by hand: the sender advertises a traversal name, so the
	// receiver must refuse it rather than write outside the destination.
	destination := t.TempDir()
	reader, writer := io.Pipe()
	stream := pipeStream{reader: reader, writer: writer}

	go func() {
		buf := make([]byte, 1)
		_, _ = reader.Read(buf) // consume the receiver's 'C'
		packet := buildPacket(STX, 0, buildMetadataPayload("../../evil.bin", 4), padInfo)
		_, _ = writer.Write(packet)
		_, _ = reader.Read(buf) // swallow the NAK
		_ = writer.Close()
	}()

	_, err := Receive(context.Background(), stream, ReceiveOptions{
		DestinationDir: destination,
		Timeout:        time.Second,
		Retries:        3,
	})
	if !errors.Is(err, zmodem.ErrFileUnsafe) {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
	// Nothing may exist outside the destination directory.
	parent := filepath.Dir(destination)
	entries, _ := os.ReadDir(parent)
	for _, entry := range entries {
		if entry.Name() == "evil.bin" {
			t.Fatal("traversal file was written outside the destination")
		}
	}
}

func TestReceiveRejectsOversizeMetadata(t *testing.T) {
	destination := t.TempDir()
	reader, writer := io.Pipe()
	stream := pipeStream{reader: reader, writer: writer}

	go func() {
		buf := make([]byte, 1)
		_, _ = reader.Read(buf) // consume the receiver's 'C'
		packet := buildPacket(STX, 0, buildMetadataPayload("huge.bin", int64(zmodem.MaxFileBytes)+1), padInfo)
		_, _ = writer.Write(packet)
		_, _ = reader.Read(buf) // swallow NAK
		_ = writer.Close()
	}()

	_, err := Receive(context.Background(), stream, ReceiveOptions{
		DestinationDir: destination,
		Timeout:        time.Second,
		Retries:        3,
	})
	if !errors.Is(err, zmodem.ErrFileTooLarge) {
		t.Fatalf("expected size rejection, got %v", err)
	}
}

func TestParseMetadataPayloadStripsPadding(t *testing.T) {
	payload := append(buildMetadataPayload("note.txt", 42), bytes.Repeat([]byte{padInfo}, 100)...)
	name, size, err := parseMetadataPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if name != "note.txt" || size != 42 {
		t.Fatalf("parsed %q %d", name, size)
	}
	// A fully padded block is the closing marker, not a file.
	if _, _, err := parseMetadataPayload(bytes.Repeat([]byte{padInfo}, BlockSizeSmall)); !errors.Is(err, ErrNoFile) {
		t.Fatalf("expected ErrNoFile, got %v", err)
	}
}

func TestCRC16MatchesZmodem(t *testing.T) {
	data := []byte("123456789")
	if CRC16(data) != zmodem.CRC16(data) {
		t.Fatal("ymodem and zmodem CRC must agree")
	}
	if CRC16(data) != 0x31C3 {
		t.Fatalf("known CRC-16/XMODEM vector mismatch: %04X", CRC16(data))
	}
}

func TestBuildPacketLayout(t *testing.T) {
	packet := buildPacket(SOH, 3, []byte("abc"), padData)
	if len(packet) != BlockSizeSmall+5 {
		t.Fatalf("packet length %d", len(packet))
	}
	if packet[0] != SOH || packet[1] != 3 || packet[2] != 0xFC {
		t.Fatalf("header bytes %x", packet[:3])
	}
	if packet[3] != 'a' || packet[4] != 'b' || packet[5] != 'c' {
		t.Fatalf("payload start %q", packet[3:6])
	}
	if packet[6] != padData {
		t.Fatalf("padding byte %x", packet[6])
	}
	expected := CRC16(packet[3 : len(packet)-2])
	got := uint16(packet[len(packet)-2])<<8 | uint16(packet[len(packet)-1])
	if got != expected {
		t.Fatalf("trailing CRC %04X != %04X", got, expected)
	}
}

func TestReadBlockRejectsBadBlockNumberComplement(t *testing.T) {
	// Header says block 1 but complement is wrong: the frame must be rejected.
	frame := buildPacket(SOH, 1, []byte("data"), padData)
	frame[2] = 0x00 // corrupt the complement
	reader := newReadOnlyReader(bytes.NewReader(frame), time.Second)
	if _, _, err := readBlock(context.Background(), reader); !errors.Is(err, ErrProtocol) {
		t.Fatalf("expected protocol error, got %v", err)
	}
}

func TestReadBlockRejectsCRCTamper(t *testing.T) {
	frame := buildPacket(SOH, 1, []byte("data"), padData)
	frame[3] ^= 0xFF // flip a payload bit without fixing the CRC
	reader := newReadOnlyReader(bytes.NewReader(frame), time.Second)
	if _, _, err := readBlock(context.Background(), reader); !errors.Is(err, zmodem.ErrCRCMismatch) {
		t.Fatalf("expected CRC mismatch, got %v", err)
	}
}

func TestSkipToBlockToleratesNoiseAndHonoursCancel(t *testing.T) {
	reader := newReadOnlyReader(bytes.NewReader([]byte("noise-bytes"+string([]byte{EOT}))), time.Second)
	header, payload, err := readBlock(context.Background(), reader)
	if err != nil {
		t.Fatalf("noise should be skipped: %v", err)
	}
	if header != EOT || payload != nil {
		t.Fatalf("header %x payload %v", header, payload)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := readBlock(cancelled, newReadOnlyReader(bytes.NewReader(nil), time.Second)); !errors.Is(err, ErrCancelled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestReceiveRequiresDestination(t *testing.T) {
	stream := ByteReaderStream{Reader: strings.NewReader(""), Writer: io.Discard}
	if _, err := Receive(context.Background(), stream, ReceiveOptions{}); !errors.Is(err, ErrProtocol) {
		t.Fatalf("expected destination requirement, got %v", err)
	}
}
